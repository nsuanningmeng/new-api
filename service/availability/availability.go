package availability

import (
	"context"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

const (
	shardCount      = 32
	defaultFlushSec = 30
	defaultWindow   = 3600
	cacheTTL        = 5 * time.Second
	cacheMaxEntries = 1024
)

type counterKey struct {
	BucketStart int64
	ModelName   string
	GroupName   string
}

type counterValue struct {
	Success int64
	Total   int64
}

type counterShard struct {
	mu sync.Mutex
	m  map[counterKey]counterValue
}

type thresholds struct {
	Green float64 `json:"green"`
	Red   float64 `json:"red"`
}

type GroupItem struct {
	Group               string   `json:"group"`
	Availability        *float64 `json:"availability"`
	Status              string   `json:"status"`
	SuccessCount        int64    `json:"success_count"`
	TotalCount          int64    `json:"total_count"`
	ExcludedFromOverall bool     `json:"excluded_from_overall"`
}

type ModelItem struct {
	Model        string   `json:"model"`
	Availability *float64 `json:"availability"`
	Status       string   `json:"status"`
	SuccessCount int64    `json:"success_count"`
	TotalCount   int64    `json:"total_count"`
}

type OverviewResult struct {
	WindowSeconds int         `json:"window_seconds"`
	Thresholds    thresholds  `json:"thresholds"`
	Items         []ModelItem `json:"items"`
}

type GroupsResult struct {
	Model         string      `json:"model"`
	WindowSeconds int         `json:"window_seconds"`
	Thresholds    thresholds  `json:"thresholds"`
	Items         []GroupItem `json:"items"`
}

type cacheEntry struct {
	expiresAt time.Time
	value     any
}

var (
	shards [shardCount]counterShard

	lifecycleMu   sync.Mutex
	startedAtomic atomic.Bool
	stopping      bool
	stopCh        chan struct{}
	doneCh        chan struct{}

	cacheMu sync.Mutex
	cache   = make(map[string]cacheEntry)
)

func init() {
	for i := range shards {
		shards[i].m = make(map[counterKey]counterValue)
	}
}

func RecordFinal(c *gin.Context, info *relaycommon.RelayInfo) {
	if c == nil || info == nil {
		return
	}
	if len(c.GetStringSlice("use_channel")) == 0 {
		return
	}
	ensureStarted()

	modelName := strings.TrimSpace(info.OriginModelName)
	groupName := strings.TrimSpace(info.UsingGroup)
	if groupName == "" {
		groupName = strings.TrimSpace(info.TokenGroup)
	}
	if modelName == "" || groupName == "" {
		return
	}

	success, total := int64(0), int64(0)
	if info.LastError == nil {
		success = 1
		total = 1
	} else {
		statusCode := info.LastError.StatusCode
		if statusCode == 0 || !countStatusCode(statusCode) {
			return
		}
		total = 1
	}

	key := counterKey{
		BucketStart: bucketStart(time.Now()),
		ModelName:   modelName,
		GroupName:   groupName,
	}
	shard := &shards[shardIndex(key)]
	shard.mu.Lock()
	current := shard.m[key]
	current.Success += success
	current.Total += total
	shard.m[key] = current
	shard.mu.Unlock()
}

func Start(ctx context.Context) {
	lifecycleMu.Lock()
	if startedAtomic.Load() || stopping {
		lifecycleMu.Unlock()
		return
	}
	startedAtomic.Store(true)
	stopCh = make(chan struct{})
	doneCh = make(chan struct{})
	localStop := stopCh
	localDone := doneCh
	lifecycleMu.Unlock()

	go runFlushLoop(ctx, localStop, localDone)
}

func Stop() {
	lifecycleMu.Lock()
	if !startedAtomic.Load() {
		lifecycleMu.Unlock()
		return
	}
	stopping = true
	localStop := stopCh
	localDone := doneCh
	startedAtomic.Store(false)
	close(localStop)
	lifecycleMu.Unlock()

	<-localDone

	lifecycleMu.Lock()
	stopCh = nil
	doneCh = nil
	stopping = false
	lifecycleMu.Unlock()
}

func GetOverview() (*OverviewResult, error) {
	th := getThresholds()
	window := getWindowSeconds()
	excludeRaw := getOption("availability.exclude_keywords", "")
	cacheKey := "overview|" + strconv.Itoa(window) + "|" + thresholdsCachePart(th) + "|" + excludeRaw
	if cached, ok := getCache(cacheKey); ok {
		if result, ok := cached.(*OverviewResult); ok {
			return result, nil
		}
	}

	rows, err := model.QueryAvailabilityRows(context.Background(), cutoffBucket(window), "")
	if err != nil {
		return nil, err
	}
	exclude := parseExcludeKeywords(excludeRaw)
	byModel := make(map[string]counterValue)
	for _, row := range rows {
		if isExcludedGroup(row.GroupName, exclude) {
			continue
		}
		current := byModel[row.ModelName]
		current.Success += row.SuccessCount
		current.Total += row.TotalCount
		byModel[row.ModelName] = current
	}

	items := make([]ModelItem, 0, len(byModel))
	for modelName, counts := range byModel {
		availability := availabilityPercent(counts.Success, counts.Total)
		items = append(items, ModelItem{
			Model:        modelName,
			Availability: availability,
			Status:       statusFor(availability, th),
			SuccessCount: counts.Success,
			TotalCount:   counts.Total,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Model < items[j].Model
	})

	result := &OverviewResult{
		WindowSeconds: window,
		Thresholds:    th,
		Items:         items,
	}
	setCache(cacheKey, result)
	return result, nil
}

func GetGroups(modelName string) (*GroupsResult, error) {
	modelName = strings.TrimSpace(modelName)
	th := getThresholds()
	window := getWindowSeconds()
	excludeRaw := getOption("availability.exclude_keywords", "")
	cacheKey := "groups|" + modelName + "|" + strconv.Itoa(window) + "|" + thresholdsCachePart(th) + "|" + excludeRaw
	if cached, ok := getCache(cacheKey); ok {
		if result, ok := cached.(*GroupsResult); ok {
			return result, nil
		}
	}

	rows, err := model.QueryAvailabilityRows(context.Background(), cutoffBucket(window), modelName)
	if err != nil {
		return nil, err
	}
	exclude := parseExcludeKeywords(excludeRaw)
	items := make([]GroupItem, 0, len(rows))
	for _, row := range rows {
		availability := availabilityPercent(row.SuccessCount, row.TotalCount)
		items = append(items, GroupItem{
			Group:               row.GroupName,
			Availability:        availability,
			Status:              statusFor(availability, th),
			SuccessCount:        row.SuccessCount,
			TotalCount:          row.TotalCount,
			ExcludedFromOverall: isExcludedGroup(row.GroupName, exclude),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Group < items[j].Group
	})

	result := &GroupsResult{
		Model:         modelName,
		WindowSeconds: window,
		Thresholds:    th,
		Items:         items,
	}
	setCache(cacheKey, result)
	return result, nil
}

func ensureStarted() {
	if startedAtomic.Load() {
		return
	}
	lifecycleMu.Lock()
	if stopping || startedAtomic.Load() {
		lifecycleMu.Unlock()
		return
	}
	startedAtomic.Store(true)
	stopCh = make(chan struct{})
	doneCh = make(chan struct{})
	localStop := stopCh
	localDone := doneCh
	lifecycleMu.Unlock()

	go runFlushLoop(context.Background(), localStop, localDone)
}

func runFlushLoop(ctx context.Context, localStop <-chan struct{}, localDone chan<- struct{}) {
	defer close(localDone)
	defer flush()
	for {
		timer := time.NewTimer(time.Duration(getFlushSeconds()) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-localStop:
			timer.Stop()
			return
		case <-timer.C:
			flush()
		}
	}
}

func flush() {
	rows := drainRows()
	if len(rows) == 0 {
		return
	}
	if err := model.UpsertAvailabilityBuckets(rows); err != nil {
		common.SysError("failed to flush availability buckets: " + err.Error())
		mergeRows(rows)
	}
}

func drainRows() []model.ModelAvailabilityBucket {
	var rows []model.ModelAvailabilityBucket
	for i := range shards {
		shard := &shards[i]
		shard.mu.Lock()
		snapshot := shard.m
		shard.m = make(map[counterKey]counterValue)
		shard.mu.Unlock()
		for key, value := range snapshot {
			rows = append(rows, model.ModelAvailabilityBucket{
				BucketStart:  key.BucketStart,
				ModelName:    key.ModelName,
				GroupName:    key.GroupName,
				SuccessCount: value.Success,
				TotalCount:   value.Total,
			})
		}
	}
	return rows
}

func mergeRows(rows []model.ModelAvailabilityBucket) {
	for _, row := range rows {
		key := counterKey{
			BucketStart: row.BucketStart,
			ModelName:   row.ModelName,
			GroupName:   row.GroupName,
		}
		shard := &shards[shardIndex(key)]
		shard.mu.Lock()
		current := shard.m[key]
		current.Success += row.SuccessCount
		current.Total += row.TotalCount
		shard.m[key] = current
		shard.mu.Unlock()
	}
}

func bucketStart(t time.Time) int64 {
	return t.Unix() / 60 * 60
}

func cutoffBucket(windowSeconds int) int64 {
	return (time.Now().Unix() - int64(windowSeconds)) / 60 * 60
}

func shardIndex(key counterKey) uint32 {
	hash := fnv32String(strconv.FormatInt(key.BucketStart, 10), 2166136261)
	hash = fnv32Byte(0, hash)
	hash = fnv32String(key.ModelName, hash)
	hash = fnv32Byte(0, hash)
	hash = fnv32String(key.GroupName, hash)
	return hash % shardCount
}

func fnv32String(value string, hash uint32) uint32 {
	for i := 0; i < len(value); i++ {
		hash = fnv32Byte(value[i], hash)
	}
	return hash
}

func fnv32Byte(value byte, hash uint32) uint32 {
	hash ^= uint32(value)
	hash *= 16777619
	return hash
}

func countStatusCode(code int) bool {
	ranges, err := operation_setting.ParseHTTPStatusCodeRanges(getOption("availability.count_status", "500-599,429"))
	if err != nil {
		ranges, _ = operation_setting.ParseHTTPStatusCodeRanges("500-599,429")
	}
	for _, r := range ranges {
		if code >= r.Start && code <= r.End {
			return true
		}
	}
	return false
}

func getThresholds() thresholds {
	raw := getOption("availability.thresholds", `{"green":99,"red":95}`)
	th := thresholds{Green: 99, Red: 95}
	if err := common.UnmarshalJsonStr(raw, &th); err != nil {
		return thresholds{Green: 99, Red: 95}
	}
	if th.Red < 0 || th.Green < 0 || th.Green > 100 || th.Red > 100 || th.Red > th.Green {
		return thresholds{Green: 99, Red: 95}
	}
	return th
}

func getFlushSeconds() int {
	return clampIntOption("availability.flush_seconds", defaultFlushSec, 5, 3600)
}

func getWindowSeconds() int {
	return clampIntOption("availability.window_seconds", defaultWindow, 60, 30*24*3600)
}

func clampIntOption(key string, defaultValue, minValue, maxValue int) int {
	value, err := strconv.Atoi(strings.TrimSpace(getOption(key, strconv.Itoa(defaultValue))))
	if err != nil {
		return defaultValue
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func getOption(key, defaultValue string) string {
	common.OptionMapRWMutex.RLock()
	value, ok := common.OptionMap[key]
	common.OptionMapRWMutex.RUnlock()
	if !ok || strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func parseExcludeKeywords(raw string) []string {
	parts := strings.Split(raw, ",")
	keywords := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		if part != "" {
			keywords = append(keywords, part)
		}
	}
	return keywords
}

func isExcludedGroup(groupName string, keywords []string) bool {
	groupName = strings.ToLower(groupName)
	for _, keyword := range keywords {
		if strings.Contains(groupName, keyword) {
			return true
		}
	}
	return false
}

func availabilityPercent(success, total int64) *float64 {
	if total <= 0 {
		return nil
	}
	value := float64(success) / float64(total) * 100
	value = math.Round(value*100) / 100
	return &value
}

func statusFor(availability *float64, th thresholds) string {
	if availability == nil {
		return "unknown"
	}
	if *availability >= th.Green {
		return "green"
	}
	if *availability < th.Red {
		return "red"
	}
	return "yellow"
}

func thresholdsCachePart(th thresholds) string {
	return strconv.FormatFloat(th.Green, 'f', -1, 64) + "," + strconv.FormatFloat(th.Red, 'f', -1, 64)
}

func getCache(key string) (any, bool) {
	now := time.Now()
	cacheMu.Lock()
	defer cacheMu.Unlock()
	entry, ok := cache[key]
	if !ok || now.After(entry.expiresAt) {
		if ok {
			delete(cache, key)
		}
		return nil, false
	}
	return entry.value, true
}

func setCache(key string, value any) {
	now := time.Now()
	cacheMu.Lock()
	if len(cache) >= cacheMaxEntries {
		for cacheKey, entry := range cache {
			if now.After(entry.expiresAt) {
				delete(cache, cacheKey)
			}
		}
		if len(cache) >= cacheMaxEntries {
			clear(cache)
		}
	}
	cache[key] = cacheEntry{
		expiresAt: now.Add(cacheTTL),
		value:     value,
	}
	cacheMu.Unlock()
}
