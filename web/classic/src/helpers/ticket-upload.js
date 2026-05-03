/*
Copyright (C) 2025 QuantumNous

工单附件客户端工具：
  - 客户端图片预压缩（双保险，主要为带宽优化；服务端仍会强制重编码）
  - MIME / 大小预校验（仅 UX 提示，服务端始终为最终防线）
  - 上传 / 下载 / 删除 API 包装
*/

import { API } from './api';

const ALLOWED_MIME = ['image/png', 'image/jpeg', 'image/gif', 'image/webp'];
const MAX_UPLOAD_BYTES = 2 * 1024 * 1024;
const MAX_STORED_BYTES = 256 * 1024;
const MAX_DIM = 1920;

export async function compressImage(file, maxBytes = MAX_STORED_BYTES, maxDim = MAX_DIM) {
  if (!file || !file.type || !file.type.startsWith('image/')) return file;
  if (file.size <= maxBytes && file.type !== 'image/gif') return file;

  try {
    const bitmap = await createImageBitmap(file);
    const ratio = Math.min(1, maxDim / Math.max(bitmap.width, bitmap.height));
    const w = Math.round(bitmap.width * ratio);
    const h = Math.round(bitmap.height * ratio);

    const canvas = document.createElement('canvas');
    canvas.width = w;
    canvas.height = h;
    const ctx = canvas.getContext('2d');
    ctx.drawImage(bitmap, 0, 0, w, h);

    // 嗅探透明度，决定输出 JPEG 还是 PNG
    const imgData = ctx.getImageData(0, 0, w, h).data;
    let hasAlpha = false;
    for (let i = 3; i < imgData.length; i += 4) {
      if (imgData[i] < 255) {
        hasAlpha = true;
        break;
      }
    }

    const targetMime = hasAlpha ? 'image/png' : 'image/jpeg';
    const qualities = targetMime === 'image/jpeg' ? [0.85, 0.75, 0.65] : [1.0];

    for (const q of qualities) {
      const blob = await new Promise((res) => canvas.toBlob(res, targetMime, q));
      if (blob && blob.size <= maxBytes) {
        return new File([blob], file.name, { type: targetMime });
      }
    }

    // 三轮质量都不达标：返回最低质量结果，让服务端兜底（仍会被服务端拒绝时再提示用户）
    const finalBlob = await new Promise((res) =>
      canvas.toBlob(res, targetMime, qualities[qualities.length - 1])
    );
    return finalBlob ? new File([finalBlob], file.name, { type: targetMime }) : file;
  } catch (e) {
    console.error('Image compression failed', e);
    return file;
  }
}

export function validateImageFile(file, allowedMime = ALLOWED_MIME, maxBytes = MAX_UPLOAD_BYTES) {
  if (!file) return { ok: false, error: '未选择文件' };
  if (!allowedMime.includes(file.type)) {
    return { ok: false, error: '不支持的文件类型' };
  }
  if (file.size > maxBytes) {
    return { ok: false, error: '文件过大' };
  }
  return { ok: true };
}

export async function uploadAttachment(ticketId, file, replyId = 0) {
  const formData = new FormData();
  formData.append('file', file);
  if (replyId > 0) {
    formData.append('reply_id', String(replyId));
  }

  // I02 修复：不显式设置 Content-Type — 让 axios/浏览器自动加 multipart boundary。
  // 显式设置会丢失 boundary，部分浏览器会按字面 multipart/form-data 发，服务端解析失败。
  const res = await API.post(`/api/ticket/${ticketId}/attachment`, formData);

  if (res.data.success) {
    return res.data.data;
  }
  throw new Error(res.data.message || '上传失败');
}

export async function deleteAttachment(ticketId, attachmentId) {
  const res = await API.delete(`/api/ticket/${ticketId}/attachment/${attachmentId}`);
  if (!res.data.success) {
    throw new Error(res.data.message || '删除失败');
  }
}

export function getAttachmentUrl(ticketId, attachmentId) {
  const baseURL = API.defaults.baseURL || '';
  return `${baseURL}/api/ticket/${ticketId}/attachment/${attachmentId}`;
}

// fetchAttachmentBlob 用于浏览器内联预览（图片缩略图）。
// 不能直接用 <img src="/api/ticket/.../attachment/..."/>，因为后端强制 Content-Disposition: attachment。
// 必须先用 fetch + responseType:blob 取回，再 createObjectURL。
export async function fetchAttachmentBlob(ticketId, attachmentId) {
  const res = await API.get(`/api/ticket/${ticketId}/attachment/${attachmentId}`, {
    responseType: 'blob',
  });
  return res.data;
}
