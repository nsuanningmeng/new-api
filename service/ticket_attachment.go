package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
	"gorm.io/gorm"
)

// AttachmentLimits 控制单个/单工单/单回复附件硬上限。
// 任何边界突破都属于安全事件，必须落 SysLog 用于审计。
type AttachmentLimits struct {
	MaxUploadBytes       int   // multipart 上传体硬上限（包括压缩前的原始图片）
	MaxStoredBytes       int   // 入库前服务端压缩后的硬上限
	MaxTicketTotalBytes  int64 // 单工单全部附件总字节数上限
	MaxTicketAttachments int   // 单工单附件数量上限
	MaxReplyAttachments  int   // 单回复附件数量上限
	MaxFilenameLen       int   // 文件名长度上限
	MaxImagePixels       int64 // 防解压炸弹：宽×高像素上限
	MaxJpegLongEdge      int   // 长边缩放目标
}

// DefaultAttachmentLimits 是默认安全策略；如需调整需经过安全评审。
var DefaultAttachmentLimits = AttachmentLimits{
	MaxUploadBytes:       2 * 1024 * 1024,
	MaxStoredBytes:       256 * 1024,
	MaxTicketTotalBytes:  3 * 1024 * 1024,
	MaxTicketAttachments: 5,
	MaxReplyAttachments:  5,
	MaxFilenameLen:       200,
	MaxImagePixels:       25_000_000,
	MaxJpegLongEdge:      1920,
}

var (
	ErrAttachmentTooLarge          = errors.New("attachment file too large")
	ErrAttachmentMimeNotAllowed    = errors.New("attachment mime type not allowed")
	ErrAttachmentDecodeFailed      = errors.New("attachment image decode failed")
	ErrAttachmentTooComplex        = errors.New("attachment too complex to compress under limit")
	ErrAttachmentLimitReached      = errors.New("attachment limit reached for this ticket or reply")
	ErrAttachmentTotalSizeExceeded = errors.New("attachment total size exceeds ticket limit")
	ErrAttachmentClosed            = errors.New("ticket is closed and cannot accept attachments")
	ErrAttachmentForbidden         = errors.New("forbidden to access this attachment")
	ErrAttachmentDuplicate         = errors.New("attachment with same content already exists in this ticket")
	ErrAttachmentTooManyPixels     = errors.New("attachment image too many pixels")
)

type UploadAttachmentInput struct {
	TicketId int
	ReplyId  int
	Scope    DataScope
	Filename string
	Header   *multipart.FileHeader
	File     io.Reader
}

// allowedAttachmentMimes 是 MIME 嗅探后允许的白名单（image-only v1）。
// SVG 显式被拒（防 stored XSS）。PDF / text 留待 v2。
var allowedAttachmentMimes = map[string]struct{}{
	"image/png":  {},
	"image/jpeg": {},
	"image/gif":  {},
	"image/webp": {},
}

// allowedAttachmentFormats 是真实 image.Decode 后允许的格式（与 sniffed mime 配合）。
var allowedAttachmentFormats = map[string]struct{}{
	"png":  {},
	"jpeg": {},
	"gif":  {},
	"webp": {},
}

// UploadTicketAttachment 串联 10 层防御：
//
//	L1 Header.Size 预检 → L2 LimitReader 二次拦截 → L3 magic-byte 嗅探 →
//	L4 真实解码（含像素总数防炸弹）→ L5 SVG 黑名单 → L6 强制重编码（剥离 EXIF/polyglot 载荷）→
//	L7 sha256 去重 → L8 文件名 sanitize → L9 单工单/单回复/总量限额 → L10 SysLog 审计。
func UploadTicketAttachment(input UploadAttachmentInput) (*model.TicketAttachment, error) {
	limits := DefaultAttachmentLimits
	if input.TicketId <= 0 || input.File == nil {
		return nil, ErrAttachmentDecodeFailed
	}

	// L1: caller-supplied size pre-check
	if input.Header != nil && input.Header.Size > int64(limits.MaxUploadBytes) {
		return nil, ErrAttachmentTooLarge
	}

	// L2: defense-in-depth — read body with hard cap
	data, err := io.ReadAll(io.LimitReader(input.File, int64(limits.MaxUploadBytes)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limits.MaxUploadBytes {
		return nil, ErrAttachmentTooLarge
	}
	if len(data) == 0 {
		common.SysError(fmt.Sprintf("ticket attachment empty upload: ticket=%d user=%d filename=%s", input.TicketId, input.Scope.ActorUserId, input.Filename))
		return nil, ErrAttachmentDecodeFailed
	}

	// L3: MIME magic-byte sniffing (do NOT trust client-declared Content-Type)
	sniffed := sniffAttachmentMime(data)
	if sniffed == "image/svg+xml" {
		// L5: SVG 黑名单（DetectContentType 通常会把 SVG 识别为 text/xml；这里再加一层防御）
		common.SysError(fmt.Sprintf("ticket attachment rejected svg: ticket=%d user=%d filename=%s", input.TicketId, input.Scope.ActorUserId, input.Filename))
		return nil, ErrAttachmentMimeNotAllowed
	}
	if _, ok := allowedAttachmentMimes[sniffed]; !ok {
		common.SysError(fmt.Sprintf("ticket attachment mime rejected: ticket=%d user=%d filename=%s sniffed=%s", input.TicketId, input.Scope.ActorUserId, input.Filename, sniffed))
		return nil, ErrAttachmentMimeNotAllowed
	}

	// L4: real-decode validation — DecodeConfig 先取尺寸（廉价）
	cfg, format, err := decodeAttachmentConfig(data)
	if err != nil {
		common.SysError(fmt.Sprintf("ticket attachment config decode failed: ticket=%d user=%d filename=%s mime=%s err=%s", input.TicketId, input.Scope.ActorUserId, input.Filename, sniffed, err.Error()))
		return nil, ErrAttachmentDecodeFailed
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		common.SysError(fmt.Sprintf("ticket attachment invalid image size: ticket=%d user=%d filename=%s width=%d height=%d", input.TicketId, input.Scope.ActorUserId, input.Filename, cfg.Width, cfg.Height))
		return nil, ErrAttachmentDecodeFailed
	}
	// 防解压炸弹：宽×高超过像素上限直接拒绝
	if int64(cfg.Width)*int64(cfg.Height) > limits.MaxImagePixels {
		return nil, ErrAttachmentTooManyPixels
	}
	if _, ok := allowedAttachmentFormats[format]; !ok {
		common.SysError(fmt.Sprintf("ticket attachment format rejected: ticket=%d user=%d filename=%s format=%s", input.TicketId, input.Scope.ActorUserId, input.Filename, format))
		return nil, ErrAttachmentMimeNotAllowed
	}

	img, err := decodeAttachmentImage(data)
	if err != nil {
		common.SysError(fmt.Sprintf("ticket attachment decode failed: ticket=%d user=%d filename=%s mime=%s format=%s err=%s", input.TicketId, input.Scope.ActorUserId, input.Filename, sniffed, format, err.Error()))
		return nil, ErrAttachmentDecodeFailed
	}

	// L6: forced re-encode + compression (kills polyglot / EXIF / embedded payloads)
	finalBytes, finalMime, finalWidth, finalHeight, err := compressAttachmentImage(img, format, data, limits)
	if err != nil {
		return nil, err
	}
	if len(finalBytes) > limits.MaxStoredBytes {
		return nil, ErrAttachmentTooComplex
	}

	// L7: sha256 dedup
	sum := sha256.Sum256(finalBytes)
	hexSha := hex.EncodeToString(sum[:])

	// 权限闸门 + L9 上限校验：先放在 sha 之后，确保不浪费上传压缩计算
	ticket, err := getTicketForActor(input.TicketId, input.Scope)
	if err != nil {
		return nil, err
	}
	if ticket.Status == model.TicketStatusClosed {
		return nil, ErrAttachmentClosed
	}
	if input.ReplyId > 0 {
		if err := ensureReplyBelongsToTicket(input.ReplyId, input.TicketId); err != nil {
			return nil, err
		}
	}

	count, err := model.CountAttachmentsForTicket(input.TicketId)
	if err != nil {
		return nil, err
	}
	if count >= int64(limits.MaxTicketAttachments) {
		return nil, ErrAttachmentLimitReached
	}
	if input.ReplyId > 0 {
		replyCount, err := model.CountAttachmentsForReply(input.ReplyId)
		if err != nil {
			return nil, err
		}
		if replyCount >= int64(limits.MaxReplyAttachments) {
			return nil, ErrAttachmentLimitReached
		}
	}
	totalSize, err := model.SumAttachmentSizeForTicket(input.TicketId)
	if err != nil {
		return nil, err
	}
	if totalSize+int64(len(finalBytes)) > limits.MaxTicketTotalBytes {
		return nil, ErrAttachmentTotalSizeExceeded
	}

	if existing, err := model.FindAttachmentBySha(input.TicketId, hexSha); err == nil && existing != nil {
		return nil, ErrAttachmentDuplicate
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// L8: filename sanitize
	filename := sanitizeAttachmentFilename(input.Filename, finalMime, hexSha, limits.MaxFilenameLen)
	attachment := &model.TicketAttachment{
		TicketId:   input.TicketId,
		ReplyId:    input.ReplyId,
		TenantId:   ticket.TenantId,
		ResellerId: ticket.ResellerId,
		UserId:     input.Scope.ActorUserId,
		Filename:   filename,
		Mime:       finalMime,
		Size:       len(finalBytes),
		Width:      finalWidth,
		Height:     finalHeight,
		Sha256:     hexSha,
		Data:       finalBytes,
	}

	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := model.CreateAttachmentTx(tx, attachment); err != nil {
			return err
		}
		return model.IncTicketAttachmentCount(tx, input.TicketId, 1)
	}); err != nil {
		return nil, err
	}

	// L10: audit log
	common.SysLog(fmt.Sprintf("ticket attachment uploaded: ticket=%d user=%d sha=%s size_in=%d size_out=%d mime=%s", input.TicketId, input.Scope.ActorUserId, hexSha, len(data), len(finalBytes), finalMime))
	return attachment, nil
}

// DownloadTicketAttachment 仅在 scope 校验通过后返回完整 BLOB。
func DownloadTicketAttachment(attachmentId int64, scope DataScope) (*model.TicketAttachment, error) {
	meta, err := getAttachmentForActor(attachmentId, scope)
	if err != nil {
		return nil, err
	}

	attachment, err := model.GetAttachmentForDownload(meta.Id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAttachmentForbidden
		}
		return nil, err
	}
	return attachment, nil
}

// DeleteTicketAttachment 仅原始上传者或 tenant_admin / platform_admin 可删除；
// 工单关闭后禁止任何删除（保留审计）。
func DeleteTicketAttachment(attachmentId int64, scope DataScope) error {
	meta, err := getAttachmentForActor(attachmentId, scope)
	if err != nil {
		return err
	}

	ticket, err := getTicketForActor(meta.TicketId, scope)
	if err != nil {
		return ErrAttachmentForbidden
	}
	if ticket.Status == model.TicketStatusClosed {
		return ErrAttachmentClosed
	}
	if meta.UserId != scope.ActorUserId && scope.ActorRole != ActorRoleTenantAdmin && scope.ActorRole != ActorRolePlatformAdmin {
		return ErrAttachmentForbidden
	}

	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := model.SoftDeleteAttachment(tx, meta.Id); err != nil {
			return err
		}
		return model.IncTicketAttachmentCount(tx, meta.TicketId, -1)
	})
}

// ListTicketAttachmentDTOs 仅加载附件元数据（不含 BLOB），用于详情/列表展示。
func ListTicketAttachmentDTOs(ticketId int) ([]dto.TicketAttachmentMetaDTO, error) {
	attachments, err := model.ListAttachmentMetaByTicket(ticketId)
	if err != nil {
		return nil, err
	}

	out := make([]dto.TicketAttachmentMetaDTO, 0, len(attachments))
	for i := range attachments {
		out = append(out, attachmentMetaDTO(&attachments[i]))
	}
	return out, nil
}

func getAttachmentForActor(attachmentId int64, scope DataScope) (*model.TicketAttachment, error) {
	meta, err := model.GetAttachmentMeta(attachmentId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAttachmentForbidden
		}
		return nil, err
	}
	if _, err := getTicketForActor(meta.TicketId, scope); err != nil {
		return nil, ErrAttachmentForbidden
	}
	return meta, nil
}

func ensureReplyBelongsToTicket(replyId, ticketId int) error {
	var count int64
	if err := model.DB.Model(&model.TicketReply{}).
		Where("id = ? AND ticket_id = ?", replyId, ticketId).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrTicketNotFound
	}
	return nil
}

func attachmentMetaDTO(a *model.TicketAttachment) dto.TicketAttachmentMetaDTO {
	if a == nil {
		return dto.TicketAttachmentMetaDTO{}
	}
	return dto.TicketAttachmentMetaDTO{
		Id:        a.Id,
		TicketId:  a.TicketId,
		ReplyId:   a.ReplyId,
		Filename:  a.Filename,
		Mime:      a.Mime,
		Size:      a.Size,
		Width:     a.Width,
		Height:    a.Height,
		Sha256:    a.Sha256,
		CreatedAt: a.CreatedAt,
		Url:       fmt.Sprintf("/api/ticket/%d/attachment/%d", a.TicketId, a.Id),
	}
}

func sniffAttachmentMime(data []byte) string {
	n := len(data)
	if n > 512 {
		n = 512
	}
	sniffed := http.DetectContentType(data[:n])
	if isWebP(data) {
		return "image/webp"
	}
	return strings.TrimSpace(strings.Split(sniffed, ";")[0])
}

func isWebP(data []byte) bool {
	return len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP"
}

func decodeAttachmentConfig(data []byte) (image.Config, string, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err == nil {
		return cfg, format, nil
	}
	cfg, err = webp.DecodeConfig(bytes.NewReader(data))
	if err == nil {
		return cfg, "webp", nil
	}
	return image.Config{}, "", err
}

func decodeAttachmentImage(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err == nil {
		return img, nil
	}
	return webp.Decode(bytes.NewReader(data))
}

func compressAttachmentImage(img image.Image, format string, data []byte, limits AttachmentLimits) ([]byte, string, int, int, error) {
	forcePNG := false
	if format == "gif" {
		// 多帧 GIF 仅取首帧（防御 GIF 渲染漏洞 + 大幅减小体积）
		g, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil {
			return nil, "", 0, 0, ErrAttachmentDecodeFailed
		}
		if len(g.Image) > 1 {
			img = g.Image[0]
			forcePNG = true
		}
	}

	if forcePNG || imageHasAlpha(img) {
		return compressAttachmentPNG(img, limits)
	}
	return compressAttachmentJPEG(img, limits)
}

func compressAttachmentJPEG(img image.Image, limits AttachmentLimits) ([]byte, string, int, int, error) {
	current := resizeToLongEdge(img, limits.MaxJpegLongEdge)
	qualities := []int{85, 75, 65, 55}

	for round := 0; round <= 3; round++ {
		for _, quality := range qualities {
			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, current, &jpeg.Options{Quality: quality}); err != nil {
				return nil, "", 0, 0, err
			}
			if buf.Len() <= limits.MaxStoredBytes {
				w, h := imageSize(current)
				return buf.Bytes(), "image/jpeg", w, h, nil
			}
		}
		if round == 3 {
			break
		}
		current = resizeByScale(current, 0.8)
	}
	return nil, "", 0, 0, ErrAttachmentTooComplex
}

func compressAttachmentPNG(img image.Image, limits AttachmentLimits) ([]byte, string, int, int, error) {
	current := resizeToLongEdge(img, limits.MaxJpegLongEdge)
	var buf bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(&buf, current); err != nil {
		return nil, "", 0, 0, err
	}
	if buf.Len() > limits.MaxStoredBytes {
		return nil, "", 0, 0, ErrAttachmentTooComplex
	}
	w, h := imageSize(current)
	return buf.Bytes(), "image/png", w, h, nil
}

func resizeToLongEdge(src image.Image, maxLongEdge int) image.Image {
	w, h := imageSize(src)
	if maxLongEdge <= 0 || (w <= maxLongEdge && h <= maxLongEdge) {
		return src
	}
	longest := w
	if h > longest {
		longest = h
	}
	scale := float64(maxLongEdge) / float64(longest)
	return resizeByScale(src, scale)
}

func resizeByScale(src image.Image, scale float64) image.Image {
	w, h := imageSize(src)
	newW := int(float64(w) * scale)
	newH := int(float64(h) * scale)
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

func imageSize(img image.Image) (int, int) {
	b := img.Bounds()
	return b.Dx(), b.Dy()
}

// imageHasAlpha 全图扫描 alpha 通道；最坏情况 25 megapixel × 4 字节 = 100MB 内存，
// 但已被 L4 像素上限保护（cfg 阶段拒绝超大图）。
func imageHasAlpha(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a != 0xffff {
				return true
			}
		}
	}
	return false
}

// sanitizeAttachmentFilename 严格白名单：[A-Za-z0-9._-] + 中文 CJK 范围；其它替换为 "_"。
// path.Base + ".." 替换确保无路径遍历。
func sanitizeAttachmentFilename(filename, mimeType, sha string, maxLen int) string {
	filename = strings.ReplaceAll(filename, "\\", "/")
	filename = path.Base(filename)
	filename = strings.ReplaceAll(filename, "..", "")

	var builder strings.Builder
	for _, r := range filename {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '.' || r == '_' || r == '-':
			builder.WriteRune(r)
		case r >= 0x4e00 && r <= 0x9fff: // CJK Unified Ideographs
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}

	name := strings.Trim(builder.String(), "._-")
	if maxLen > 0 {
		runes := []rune(name)
		if len(runes) > maxLen {
			name = string(runes[:maxLen])
		}
	}
	if strings.Trim(name, "._-") == "" {
		ext := ".jpg"
		if mimeType == "image/png" {
			ext = ".png"
		}
		prefix := sha
		if len(prefix) > 8 {
			prefix = prefix[:8]
		}
		return "attachment_" + prefix + ext
	}
	return name
}
