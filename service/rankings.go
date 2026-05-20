package service

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	rankingCacheTTL             = 5 * time.Minute
	rankingLeaderboardLimit     = 20
	rankingHistoryLimit         = 10
	rankingFreeUserLimit        = 20
	rankingFreeUserHistoryHours = 24
	rankingOthersLabel          = "Others"
	rankingUnknownVendor        = "Unknown"
)

type RankingsResponse struct {
	Models              []RankedModel         `json:"models"`
	ModelsHistory       ModelHistorySeries    `json:"models_history"`
	FreeUsers           []RankedFreeUser      `json:"free_users"`
	FreeUserTotalTokens int64                 `json:"free_user_total_tokens"`
	FreeUserHistory     FreeUserHistorySeries `json:"free_user_history"`
}

type RankedModel struct {
	Rank        int    `json:"rank"`
	ModelName   string `json:"model_name"`
	Vendor      string `json:"vendor"`
	VendorIcon  string `json:"vendor_icon,omitempty"`
	Category    string `json:"category"`
	TotalTokens int64  `json:"total_tokens"`
}

type RankedFreeUser struct {
	Rank        int    `json:"rank"`
	DisplayName string `json:"display_name"`
	TotalTokens int64  `json:"total_tokens"`
	Named       bool   `json:"named"`
}

type FreeUserHistoryPoint struct {
	Rank             int    `json:"rank"`
	DisplayName      string `json:"display_name"`
	SeriesLabel      string `json:"series_label"`
	Hour             int    `json:"hour"`
	HourLabel        string `json:"hour_label"`
	Tokens           int64  `json:"tokens"`
	CumulativeTokens int64  `json:"cumulative_tokens"`
}

type FreeUserHistorySeries struct {
	Points []FreeUserHistoryPoint `json:"points"`
	Hours  int                    `json:"hours"`
}

type rankedFreeUserInternal struct {
	UserID      int
	Rank        int
	DisplayName string
	SeriesLabel string
	TotalTokens int64
	Named       bool
}

type ModelHistoryPoint struct {
	Ts     string `json:"ts"`
	Label  string `json:"label"`
	Model  string `json:"model"`
	Vendor string `json:"vendor"`
	Tokens int64  `json:"tokens"`
}

type ModelHistoryModel struct {
	Name   string `json:"name"`
	Vendor string `json:"vendor"`
	Total  int64  `json:"total"`
}

type ModelHistorySeries struct {
	Points  []ModelHistoryPoint `json:"points"`
	Models  []ModelHistoryModel `json:"models"`
	Buckets int                 `json:"buckets"`
}

type rankingPeriodConfig struct {
	id          string
	duration    time.Duration
	bucketSize  int64
	labelLayout string
}

type rankingCacheItem struct {
	expiresAt time.Time
	data      *RankingsResponse
}

type rankingModelMeta struct {
	vendor     string
	vendorIcon string
}

var (
	rankingCacheMu sync.Mutex
	rankingCache   = map[string]rankingCacheItem{}
)

func GetRankingsSnapshot(period string) (*RankingsResponse, error) {
	config, err := rankingConfig(period)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	rankingCacheMu.Lock()
	if item, ok := rankingCache[config.id]; ok && now.Before(item.expiresAt) {
		rankingCacheMu.Unlock()
		return item.data, nil
	}
	rankingCacheMu.Unlock()

	data, err := buildRankingsSnapshot(config, now)
	if err != nil {
		return nil, err
	}

	rankingCacheMu.Lock()
	rankingCache[config.id] = rankingCacheItem{
		expiresAt: now.Add(rankingCacheTTL),
		data:      data,
	}
	rankingCacheMu.Unlock()

	return data, nil
}

func FlushRankingsCache() {
	rankingCacheMu.Lock()
	rankingCache = map[string]rankingCacheItem{}
	rankingCacheMu.Unlock()
}

func FlushRankingsCacheForTest() {
	FlushRankingsCache()
}

func rankingConfig(period string) (rankingPeriodConfig, error) {
	switch period {
	case "", "week":
		return rankingPeriodConfig{id: "week", duration: 7 * 24 * time.Hour, bucketSize: 24 * 3600, labelLayout: "Jan 2"}, nil
	case "today":
		return rankingPeriodConfig{id: "today", duration: 24 * time.Hour, bucketSize: 3600, labelLayout: "15:04"}, nil
	case "month":
		return rankingPeriodConfig{id: "month", duration: 30 * 24 * time.Hour, bucketSize: 24 * 3600, labelLayout: "Jan 2"}, nil
	case "year":
		return rankingPeriodConfig{id: "year", duration: 365 * 24 * time.Hour, bucketSize: 7 * 24 * 3600, labelLayout: "Jan 2"}, nil
	case "all":
		return rankingPeriodConfig{id: "all", bucketSize: 30 * 24 * 3600, labelLayout: "Jan 2006"}, nil
	default:
		return rankingPeriodConfig{}, fmt.Errorf("invalid ranking period: %s", period)
	}
}

func buildRankingsSnapshot(config rankingPeriodConfig, now time.Time) (*RankingsResponse, error) {
	startTime, endTime := rankingTimeRange(config, now)
	currentTotals, err := model.GetRankingQuotaTotals(startTime, endTime)
	if err != nil {
		return nil, err
	}
	currentBuckets, err := model.GetRankingQuotaBuckets(startTime, endTime, config.bucketSize)
	if err != nil {
		return nil, err
	}
	freeUserTotals, err := model.GetRankingFreeUserTotals(rankingFreeUserLimit)
	if err != nil {
		return nil, err
	}

	meta := buildRankingModelMeta()
	rankedModels := buildRankedModels(currentTotals, meta)
	freeUserInternals, freeUserTotalTokens := buildRankedFreeUserInternals(freeUserTotals)
	freeUserHistory, err := buildFreeUserHistory(freeUserInternals)
	if err != nil {
		return nil, err
	}

	return &RankingsResponse{
		Models:              limitRankedModels(rankedModels, rankingLeaderboardLimit),
		ModelsHistory:       buildModelHistory(currentBuckets, currentTotals, meta, config),
		FreeUsers:           publicRankedFreeUsers(freeUserInternals),
		FreeUserTotalTokens: freeUserTotalTokens,
		FreeUserHistory:     freeUserHistory,
	}, nil
}

func rankingTimeRange(config rankingPeriodConfig, now time.Time) (int64, int64) {
	endTime := now.Unix()
	if config.duration <= 0 {
		return 0, endTime
	}
	return now.Add(-config.duration).Unix(), endTime
}

func buildRankingModelMeta() map[string]rankingModelMeta {
	vendorByID := make(map[int]model.PricingVendor)
	for _, vendor := range model.GetVendors() {
		vendorByID[vendor.ID] = vendor
	}

	meta := make(map[string]rankingModelMeta)
	for _, pricing := range model.GetPricing() {
		item := rankingModelMeta{vendor: rankingUnknownVendor}
		if vendor, ok := vendorByID[pricing.VendorID]; ok {
			item.vendor = vendor.Name
			item.vendorIcon = vendor.Icon
		} else if pricing.OwnerBy != "" {
			item.vendor = pricing.OwnerBy
		}
		meta[pricing.ModelName] = item
	}
	return meta
}

func modelMeta(modelName string, meta map[string]rankingModelMeta) rankingModelMeta {
	if item, ok := meta[modelName]; ok && item.vendor != "" {
		return item
	}
	return rankingModelMeta{vendor: rankingUnknownVendor}
}

func buildRankedModels(totals []model.RankingQuotaTotal, meta map[string]rankingModelMeta) []RankedModel {
	rows := make([]RankedModel, 0, len(totals))
	for idx, item := range totals {
		modelMeta := modelMeta(item.ModelName, meta)
		rows = append(rows, RankedModel{
			Rank:        idx + 1,
			ModelName:   item.ModelName,
			Vendor:      modelMeta.vendor,
			VendorIcon:  modelMeta.vendorIcon,
			Category:    "all",
			TotalTokens: item.TotalTokens,
		})
	}
	return rows
}

func buildModelHistory(buckets []model.RankingQuotaBucket, totals []model.RankingQuotaTotal, meta map[string]rankingModelMeta, config rankingPeriodConfig) ModelHistorySeries {
	topModels := make(map[string]struct{})
	models := make([]ModelHistoryModel, 0, minInt(len(totals), rankingHistoryLimit)+1)
	otherTotal := int64(0)
	for idx, item := range totals {
		if idx < rankingHistoryLimit {
			topModels[item.ModelName] = struct{}{}
			modelMeta := modelMeta(item.ModelName, meta)
			models = append(models, ModelHistoryModel{Name: item.ModelName, Vendor: modelMeta.vendor, Total: item.TotalTokens})
			continue
		}
		otherTotal += item.TotalTokens
	}
	if otherTotal > 0 {
		models = append(models, ModelHistoryModel{Name: rankingOthersLabel, Vendor: "Various", Total: otherTotal})
	}

	bucketSet := make(map[int64]struct{})
	tokensByBucketAndModel := make(map[int64]map[string]int64)
	for _, item := range buckets {
		modelName := item.ModelName
		if _, ok := topModels[modelName]; !ok {
			modelName = rankingOthersLabel
		}
		bucketSet[item.Bucket] = struct{}{}
		if _, ok := tokensByBucketAndModel[item.Bucket]; !ok {
			tokensByBucketAndModel[item.Bucket] = make(map[string]int64)
		}
		tokensByBucketAndModel[item.Bucket][modelName] += item.Tokens
	}

	sortedBuckets := sortedRankingBuckets(bucketSet)
	points := make([]ModelHistoryPoint, 0, len(sortedBuckets)*len(models))
	for _, bucket := range sortedBuckets {
		for _, historyModel := range models {
			tokens := tokensByBucketAndModel[bucket][historyModel.Name]
			if tokens <= 0 {
				continue
			}
			points = append(points, ModelHistoryPoint{
				Ts:     rankingBucketTs(bucket),
				Label:  rankingBucketLabel(bucket, config),
				Model:  historyModel.Name,
				Vendor: historyModel.Vendor,
				Tokens: tokens,
			})
		}
	}

	return ModelHistorySeries{
		Points:  points,
		Models:  models,
		Buckets: len(sortedBuckets),
	}
}

func sortedRankingBuckets(bucketSet map[int64]struct{}) []int64 {
	buckets := make([]int64, 0, len(bucketSet))
	for bucket := range bucketSet {
		buckets = append(buckets, bucket)
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i] < buckets[j]
	})
	return buckets
}

func rankingBucketTs(bucket int64) string {
	return time.Unix(bucket, 0).UTC().Format(time.RFC3339)
}

func rankingBucketLabel(bucket int64, config rankingPeriodConfig) string {
	return time.Unix(bucket, 0).Format(config.labelLayout)
}

func buildRankedFreeUserInternals(totals []model.RankingFreeUserTotal) ([]rankedFreeUserInternal, int64) {
	rows := make([]rankedFreeUserInternal, 0, len(totals))
	totalTokens := int64(0)
	for idx, item := range totals {
		rank := idx + 1
		totalTokens += item.TotalTokens
		displayName, named := rankingDisplayNameFromSetting(item.Setting, rank)
		rows = append(rows, rankedFreeUserInternal{
			UserID:      item.UserID,
			Rank:        rank,
			DisplayName: displayName,
			SeriesLabel: fmt.Sprintf("#%d · %s", rank, displayName),
			TotalTokens: item.TotalTokens,
			Named:       named,
		})
	}
	return rows, totalTokens
}

func publicRankedFreeUsers(internal []rankedFreeUserInternal) []RankedFreeUser {
	rows := make([]RankedFreeUser, 0, len(internal))
	for _, item := range internal {
		rows = append(rows, RankedFreeUser{
			Rank:        item.Rank,
			DisplayName: item.DisplayName,
			TotalTokens: item.TotalTokens,
			Named:       item.Named,
		})
	}
	return rows
}

func buildFreeUserHistory(users []rankedFreeUserInternal) (FreeUserHistorySeries, error) {
	series := FreeUserHistorySeries{Points: []FreeUserHistoryPoint{}, Hours: rankingFreeUserHistoryHours}
	if len(users) == 0 {
		return series, nil
	}

	userIDs := make([]int, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.UserID)
	}

	subs, err := model.GetRankingFreeUserSubscriptions(userIDs)
	if err != nil {
		return series, err
	}
	if len(subs) == 0 {
		return buildFreeUserHistoryPoints(users, map[int][]int64{}), nil
	}

	subsByID := make(map[int]model.RankingFreeUserSubscription, len(subs))
	minStart := subs[0].StartTime
	maxEnd := subs[0].StartTime + int64(rankingFreeUserHistoryHours*3600)
	for _, sub := range subs {
		subsByID[sub.ID] = sub
		if sub.StartTime < minStart {
			minStart = sub.StartTime
		}
		end := sub.StartTime + int64(rankingFreeUserHistoryHours*3600)
		if end > maxEnd {
			maxEnd = end
		}
	}

	logs, err := model.GetRankingFreeUserLogCandidates(userIDs, minStart, maxEnd)
	if err != nil {
		return series, err
	}

	tokensByUserHour := make(map[int][]int64, len(users))
	for _, user := range users {
		tokensByUserHour[user.UserID] = make([]int64, rankingFreeUserHistoryHours)
	}
	for _, candidate := range logs {
		var other map[string]interface{}
		if err := common.UnmarshalJsonStr(candidate.Other, &other); err != nil {
			continue
		}
		subID, ok := intFromOtherMapValue(other["subscription_id"])
		if !ok {
			continue
		}
		consumed, ok := intFromOtherMapValue(other["subscription_tokens_consumed"])
		if !ok || consumed <= 0 {
			continue
		}
		sub, ok := subsByID[subID]
		if !ok || sub.UserID != candidate.UserID {
			continue
		}
		if candidate.CreatedAt < sub.StartTime || candidate.CreatedAt >= sub.StartTime+int64(rankingFreeUserHistoryHours*3600) {
			continue
		}
		hour := int((candidate.CreatedAt - sub.StartTime) / 3600)
		if hour < 0 || hour >= rankingFreeUserHistoryHours {
			continue
		}
		tokensByUserHour[candidate.UserID][hour] += int64(consumed)
	}

	return buildFreeUserHistoryPoints(users, tokensByUserHour), nil
}

func buildFreeUserHistoryPoints(users []rankedFreeUserInternal, tokensByUserHour map[int][]int64) FreeUserHistorySeries {
	series := FreeUserHistorySeries{Points: []FreeUserHistoryPoint{}, Hours: rankingFreeUserHistoryHours}
	for _, user := range users {
		buckets := tokensByUserHour[user.UserID]
		if len(buckets) != rankingFreeUserHistoryHours {
			buckets = make([]int64, rankingFreeUserHistoryHours)
		}
		cumulative := int64(0)
		for hour := 0; hour < rankingFreeUserHistoryHours; hour++ {
			tokens := buckets[hour]
			cumulative += tokens
			series.Points = append(series.Points, FreeUserHistoryPoint{
				Rank:             user.Rank,
				DisplayName:      user.DisplayName,
				SeriesLabel:      user.SeriesLabel,
				Hour:             hour,
				HourLabel:        fmt.Sprintf("%dh", hour),
				Tokens:           tokens,
				CumulativeTokens: cumulative,
			})
		}
	}
	return series
}

func intFromOtherMapValue(value interface{}) (int, bool) {
	switch v := value.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		return parsed, err == nil
	default:
		return 0, false
	}
}

func rankingDisplayNameFromSetting(setting string, rank int) (string, bool) {
	name := ""
	if setting != "" {
		if parsed, err := model.ParseUserSettingString(setting); err == nil {
			name = strings.TrimSpace(parsed.RankingsDisplayName)
		}
	}
	if name == "" {
		return anonymousRankingsDisplayName(rank), false
	}
	return name, true
}

func anonymousRankingsDisplayName(rank int) string {
	if rank <= 0 {
		rank = 1
	}
	return fmt.Sprintf("Explorer #%d", rank)
}

func limitRankedModels(rows []RankedModel, limit int) []RankedModel {
	if limit <= 0 || len(rows) <= limit {
		return rows
	}
	return rows[:limit]
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
