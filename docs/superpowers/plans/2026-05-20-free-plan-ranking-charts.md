# 免费套餐排行榜图表化实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将免费套餐排行榜改为图表优先展示，提供横向条形图和按免费订阅开通后 0–23 小时对齐的折线趋势，并保持匿名隐私边界。

**架构：** 后端继续用 `user_subscriptions.token_used` 生成免费总榜，同时新增基于 `logs.other.subscription_id` 与 `subscription_tokens_consumed` 的 `free_user_history`，只在 Go 层解析 JSON 并补齐 24 小时点。前端新增免费榜条形图和折线图组件，`FreeUsersSection` 负责视图切换和简化榜单，所有文案走 i18n。

**技术栈：** Go 1.22+、Gin、GORM、SQLite/MySQL/PostgreSQL 兼容查询、React 19、TypeScript、@visactor/react-vchart、i18next、Bun。

---

## 执行约束

- 本计划按用户要求直接在当前主分支/当前工作区开发，不创建 worktree。
- 计划审查通过后，先提交规格与计划文件，再开始实现。提交命令必须精确暂存：
  - `C:/Users/34404/source/repos/new-api/docs/superpowers/specs/2026-05-20-free-plan-ranking-charts-design.md`
  - `C:/Users/34404/source/repos/new-api/docs/superpowers/plans/2026-05-20-free-plan-ranking-charts.md`
- 每个新启动的实现、修复、审查子代理提示词必须不少于 2000 字，并包含完整绝对路径：spec、plan、`AGENTS.md`、`web/default/AGENTS.md`、目标文件清单、完整任务文本、验证方式和禁止事项。
- 只读 review 任务每轮至少 3 个子代理并发运行；若任一 review 返回必须修复项，修复后重新并发复审，直到全部 PASS。
- 并发写入边界：任务 1（后端）与任务 2（前端图表）可并发；任务 3（i18n 与最终测试）必须等任务 2 完成后串行执行，并独占 `web/default/src/features/rankings/rankings-free-users.test.ts` 与六个 locale JSON。
- 不得并发启动会修改同一文件的子代理。暂存和提交必须使用精确文件路径，避免把用户或其他子代理的无关改动一起提交。

## 规格与规则

- 规格文件：`docs/superpowers/specs/2026-05-20-free-plan-ranking-charts-design.md`
- 项目规则：`AGENTS.md`
- 前端规则：`web/default/AGENTS.md`

关键约束：

- 不使用 `quota_data` 做免费榜 24 小时趋势主数据源。
- 趋势只统计 `logs.other.subscription_id` 指向免费/试用订阅，且 `subscription_tokens_consumed > 0` 的消费日志。
- JSON 解析使用项目封装，例如 `common.UnmarshalJsonStr`，不得直接调用标准库 marshal/unmarshal。
- 数据库查询保持 SQLite/MySQL/PostgreSQL 兼容，不使用 JSON 操作符、窗口函数、`DISTINCT ON` 或数据库专属时间函数。
- 外部 JSON 响应不得输出 `user_id`、`subscription_id`、`username`、`email` 或普通账号 `DisplayName`。
- 不恢复 `MarketShareSection`、`PulseSection`、`VendorRanking`、`RankingMover`、`vendor_share_history`、`top_movers`、`top_droppers`。

## 文件结构

### 后端

- 修改：`model/usedata_rankings.go`
  - 增加免费订阅窗口与消费日志候选查询结构/函数。
- 修改：`service/rankings.go`
  - 扩展 `RankingsResponse`，新增 `free_user_history`。
  - 构建内部 free user 映射，保留 `UserID` 但不导出。
  - 从日志候选构建 24 小时趋势，补齐 24 个点。
- 修改：`service/rankings_test.go`
  - 增加趋势归属、边界、累计、补点、隐私和排除规则测试。

### 前端

- 修改：`web/default/src/features/rankings/types.ts`
  - 增加 `FreeUserHistoryPoint`、`FreeUserHistorySeries`、`RankingsSnapshot.free_user_history`。
- 修改：`web/default/src/features/rankings/index.tsx`
  - 向 `FreeUsersSection` 传入 `snapshot.free_user_history`。
- 修改：`web/default/src/features/rankings/components/free-users-section.tsx`
  - 改成容器组件，提供条形图/趋势切换和折线模式切换。
- 创建：`web/default/src/features/rankings/components/free-users-bar-chart.tsx`
  - 横向条形图，前端派生 `series_label`。
- 创建：`web/default/src/features/rankings/components/free-users-line-chart.tsx`
  - 折线图，支持每小时和累计两种模式。
- 如 `free-users-section.tsx` 接近 200 行，创建：`web/default/src/features/rankings/components/free-users-list.tsx`
  - 简化榜单列表。
- 修改：`web/default/src/features/rankings/components/index.ts`
  - 导出新增组件。
- 修改：`web/default/src/features/rankings/rankings-free-users.test.ts`
  - 增加类型、图表、调用链、旧 UI 防回归、i18n smoke 静态测试。
- 修改：`web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`
  - 新增图表相关翻译。

---

## 任务 1：后端免费榜 24 小时趋势 API

**文件：**
- 修改：`model/usedata_rankings.go`
- 修改：`service/rankings.go`
- 修改：`service/rankings_test.go`

- [ ] **步骤 1：编写失败的后端测试**

在 `service/rankings_test.go` 追加测试。测试需要创建用户、免费/付费/奖励计划、免费订阅、消费日志，并断言 `free_user_history`。使用项目现有测试 setup 与 `truncate(t)`。新增测试至少包含以下形态：

```go
func TestGetRankingsSnapshotBuildsFreeUserHistoryFromSubscriptionLogs(t *testing.T) {
	truncate(t)
	FlushRankingsCacheForTest()
	require.NoError(t, model.DB.AutoMigrate(&model.QuotaData{}))

	start := time.Date(2026, 5, 20, 10, 30, 0, 0, time.UTC).Unix()
	freeCode := "history-free"
	paidCode := "history-paid"
	freePlan := &model.SubscriptionPlan{Id: 9971, Title: "History Free", Enabled: true, PriceAmount: 0, ConcurrencyLimit: 1, IsTrial: true, BusinessCode: &freeCode}
	paidPlan := &model.SubscriptionPlan{Id: 9972, Title: "History Paid", Enabled: true, PriceAmount: 10, ConcurrencyLimit: 1, BusinessCode: &paidCode}
	require.NoError(t, model.DB.Create(freePlan).Error)
	require.NoError(t, model.DB.Create(paidPlan).Error)

	user := model.User{Id: 9973, Username: "history-user", DisplayName: "Private Name", Status: common.UserStatusEnabled, AffCode: "aff9973"}
	user.SetSetting(dto.UserSetting{RankingsDisplayName: "Token Sprinter"})
	require.NoError(t, model.DB.Create(&user).Error)

	freeSub := model.UserSubscription{Id: 9974, UserId: user.Id, PlanId: freePlan.Id, Status: "active", TokenUsed: 300, StartTime: start, EndTime: start + 48*3600, GrantReason: "trial_code"}
	paidSub := model.UserSubscription{Id: 9975, UserId: user.Id, PlanId: paidPlan.Id, Status: "active", TokenUsed: 999, StartTime: start, EndTime: start + 48*3600, GrantReason: "order"}
	require.NoError(t, model.DB.Create(&freeSub).Error)
	require.NoError(t, model.DB.Create(&paidSub).Error)

	seedRankingConsumeLog(t, 9976, user.Id, start+30*60, freeSub.Id, 100)
	seedRankingConsumeLog(t, 9977, user.Id, start+90*60, freeSub.Id, 200)
	seedRankingConsumeLog(t, 9978, user.Id, start+24*3600, freeSub.Id, 300)
	seedRankingConsumeLog(t, 9979, user.Id, start+45*60, paidSub.Id, 999)
	seedRankingConsumeLog(t, 9980, user.Id, start+60*60, 0, 777)

	result, err := GetRankingsSnapshot("all")
	require.NoError(t, err)
	require.Len(t, result.FreeUsers, 1)
	require.Equal(t, 24, result.FreeUserHistory.Hours)
	require.Len(t, result.FreeUserHistory.Points, 24)

	assert.Equal(t, int64(100), result.FreeUserHistory.Points[0].Tokens)
	assert.Equal(t, int64(100), result.FreeUserHistory.Points[0].CumulativeTokens)
	assert.Equal(t, int64(200), result.FreeUserHistory.Points[1].Tokens)
	assert.Equal(t, int64(300), result.FreeUserHistory.Points[1].CumulativeTokens)
	assert.Equal(t, int64(0), result.FreeUserHistory.Points[2].Tokens)
	assert.Equal(t, int64(300), result.FreeUserHistory.Points[2].CumulativeTokens)
	assert.Equal(t, int64(300), result.FreeUserHistory.Points[23].CumulativeTokens)
	assert.Equal(t, "#1 · Token Sprinter", result.FreeUserHistory.Points[0].SeriesLabel)
}
```

同时追加重叠免费订阅、非正数消费、软删除用户与隐私测试：

```go
func TestGetRankingsSnapshotFreeUserHistoryDoesNotDuplicateOverlappingSubscriptions(t *testing.T) {
	truncate(t)
	FlushRankingsCacheForTest()
	require.NoError(t, model.DB.AutoMigrate(&model.QuotaData{}))

	start := time.Date(2026, 5, 20, 8, 15, 0, 0, time.UTC).Unix()
	freeCode := "overlap-free"
	plan := &model.SubscriptionPlan{Id: 9981, Title: "Overlap Free", Enabled: true, PriceAmount: 0, ConcurrencyLimit: 1, IsTrial: true, BusinessCode: &freeCode}
	require.NoError(t, model.DB.Create(plan).Error)
	user := model.User{Id: 9982, Username: "overlap-user", DisplayName: "Hidden Overlap", Status: common.UserStatusEnabled, AffCode: "aff9982"}
	require.NoError(t, model.DB.Create(&user).Error)

	first := model.UserSubscription{Id: 9983, UserId: user.Id, PlanId: plan.Id, Status: "active", TokenUsed: 100, StartTime: start, EndTime: start + 48*3600, GrantReason: "trial_code"}
	second := model.UserSubscription{Id: 9984, UserId: user.Id, PlanId: plan.Id, Status: "active", TokenUsed: 500, StartTime: start + 30*60, EndTime: start + 48*3600, GrantReason: "trial_code"}
	require.NoError(t, model.DB.Create(&first).Error)
	require.NoError(t, model.DB.Create(&second).Error)
	seedRankingConsumeLog(t, 9985, user.Id, start+45*60, second.Id, 500)

	result, err := GetRankingsSnapshot("all")
	require.NoError(t, err)
	require.Len(t, result.FreeUserHistory.Points, 24)
	assert.Equal(t, int64(500), result.FreeUserHistory.Points[0].Tokens)
	assert.Equal(t, int64(500), result.FreeUserHistory.Points[0].CumulativeTokens)
	assert.Equal(t, int64(500), result.FreeUserHistory.Points[23].CumulativeTokens)
}

func TestGetRankingsSnapshotFreeUserHistoryIgnoresNonPositiveAndDeletedUsers(t *testing.T) {
	truncate(t)
	FlushRankingsCacheForTest()
	require.NoError(t, model.DB.AutoMigrate(&model.QuotaData{}))

	start := time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC).Unix()
	freeCode := "ignore-free"
	plan := &model.SubscriptionPlan{Id: 9986, Title: "Ignore Free", Enabled: true, PriceAmount: 0, ConcurrencyLimit: 1, IsTrial: true, BusinessCode: &freeCode}
	require.NoError(t, model.DB.Create(plan).Error)
	active := model.User{Id: 9987, Username: "active-history", Status: common.UserStatusEnabled, AffCode: "aff9987"}
	deleted := model.User{Id: 9988, Username: "deleted-history", Status: common.UserStatusEnabled, AffCode: "aff9988"}
	require.NoError(t, model.DB.Create(&active).Error)
	require.NoError(t, model.DB.Create(&deleted).Error)
	activeSub := model.UserSubscription{Id: 9989, UserId: active.Id, PlanId: plan.Id, Status: "active", TokenUsed: 50, StartTime: start, EndTime: start + 48*3600, GrantReason: "trial_code"}
	deletedSub := model.UserSubscription{Id: 9990, UserId: deleted.Id, PlanId: plan.Id, Status: "active", TokenUsed: 500, StartTime: start, EndTime: start + 48*3600, GrantReason: "trial_code"}
	require.NoError(t, model.DB.Create(&activeSub).Error)
	require.NoError(t, model.DB.Create(&deletedSub).Error)
	require.NoError(t, deleted.Delete())
	seedRankingConsumeLogWithMetered(t, 9991, active.Id, start+60, activeSub.Id, 1, 0, true)
	seedRankingConsumeLogWithMetered(t, 9992, active.Id, start+120, activeSub.Id, 1, -10, true)
	seedRankingConsumeLog(t, 9993, active.Id, start+180, activeSub.Id, 50)
	seedRankingConsumeLog(t, 9994, deleted.Id, start+180, deletedSub.Id, 500)

	result, err := GetRankingsSnapshot("all")
	require.NoError(t, err)
	require.Len(t, result.FreeUsers, 1)
	require.Len(t, result.FreeUserHistory.Points, 24)
	assert.Equal(t, int64(50), result.FreeUserHistory.Points[0].Tokens)
	assert.Equal(t, int64(50), result.FreeUserHistory.Points[23].CumulativeTokens)
}

func TestGetRankingsSnapshotFreeUserHistoryResponseHidesAccountIdentifiers(t *testing.T) {
	truncate(t)
	FlushRankingsCacheForTest()
	require.NoError(t, model.DB.AutoMigrate(&model.QuotaData{}))

	start := time.Date(2026, 5, 20, 7, 0, 0, 0, time.UTC).Unix()
	freeCode := "privacy-free"
	plan := &model.SubscriptionPlan{Id: 9995, Title: "Privacy Free", Enabled: true, PriceAmount: 0, ConcurrencyLimit: 1, IsTrial: true, BusinessCode: &freeCode}
	require.NoError(t, model.DB.Create(plan).Error)
	user := model.User{Id: 9996, Username: "private-user", DisplayName: "Private Display", Email: "private@example.com", Status: common.UserStatusEnabled, AffCode: "aff9996"}
	require.NoError(t, model.DB.Create(&user).Error)
	sub := model.UserSubscription{Id: 9997, UserId: user.Id, PlanId: plan.Id, Status: "active", TokenUsed: 100, StartTime: start, EndTime: start + 48*3600, GrantReason: "trial_code"}
	require.NoError(t, model.DB.Create(&sub).Error)
	seedRankingConsumeLog(t, 9998, user.Id, start+60, sub.Id, 100)

	result, err := GetRankingsSnapshot("all")
	require.NoError(t, err)
	encoded, err := common.Marshal(result)
	require.NoError(t, err)
	body := string(encoded)
	assert.NotContains(t, body, "user_id")
	assert.NotContains(t, body, "subscription_id")
	assert.NotContains(t, body, "private-user")
	assert.NotContains(t, body, "private@example.com")
	assert.NotContains(t, body, "Private Display")
	assert.Contains(t, body, "Explorer #1")
	assert.Contains(t, body, "#1 · Explorer #1")
}
```

Add helper in the same test file:

```go
func seedRankingConsumeLog(t *testing.T, id int, userID int, createdAt int64, subscriptionID int, subscriptionTokens int) {
	t.Helper()
	seedRankingConsumeLogWithMetered(t, id, userID, createdAt, subscriptionID, subscriptionTokens, subscriptionTokens, subscriptionTokens != 0)
}

func seedRankingConsumeLogWithMetered(t *testing.T, id int, userID int, createdAt int64, subscriptionID int, meteredTokens int, subscriptionTokens int, includeSubscriptionTokens bool) {
	t.Helper()
	other := map[string]interface{}{}
	if subscriptionID > 0 {
		other["subscription_id"] = subscriptionID
	}
	if includeSubscriptionTokens {
		other["subscription_tokens_consumed"] = subscriptionTokens
	}
	otherStr := common.MapToJsonStr(other)
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		Id:            id,
		UserId:        userID,
		CreatedAt:     createdAt,
		Type:          model.LogTypeConsume,
		ModelName:     "gpt-5.5",
		MeteredTokens: &meteredTokens,
		Other:         otherStr,
	}).Error)
}
```

- [ ] **步骤 2：运行后端测试验证失败**

运行：

```bash
go test ./service -run 'TestGetRankingsSnapshot.*FreeUserHistory' -count=1
```

预期：编译失败或测试失败，原因是 `FreeUserHistory`、`FreeUserHistorySeries`、`GetRankingFreeUserSubscriptions`、`GetRankingFreeUserLogCandidates` 等尚未实现。

- [ ] **步骤 3：实现 model 层候选查询**

在 `model/usedata_rankings.go` 增加结构和函数：

```go
type RankingFreeUserSubscription struct {
	ID        int   `json:"id"`
	UserID    int   `json:"user_id"`
	StartTime int64 `json:"start_time"`
	EndTime   int64 `json:"end_time"`
}

type RankingFreeUserLogCandidate struct {
	ID            int    `json:"id"`
	UserID        int    `json:"user_id"`
	CreatedAt     int64  `json:"created_at"`
	MeteredTokens *int   `json:"metered_tokens"`
	Other         string `json:"other"`
}

func GetRankingFreeUserSubscriptions(userIDs []int) ([]RankingFreeUserSubscription, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	var rows []RankingFreeUserSubscription
	err := DB.Table("user_subscriptions").
		Select("user_subscriptions.id, user_subscriptions.user_id, user_subscriptions.start_time, user_subscriptions.end_time").
		Joins("JOIN subscription_plans ON subscription_plans.id = user_subscriptions.plan_id").
		Joins("JOIN users ON users.id = user_subscriptions.user_id").
		Where("users.deleted_at IS NULL").
		Where("user_subscriptions.user_id IN ?", userIDs).
		Where("subscription_plans.price_amount = ?", 0).
		Where("(subscription_plans.is_trial = ? OR user_subscriptions.grant_reason IN ?)", true, []string{"trial_code", "invite_trial"}).
		Find(&rows).Error
	return rows, err
}

func GetRankingFreeUserLogCandidates(userIDs []int, startTime int64, endTime int64) ([]RankingFreeUserLogCandidate, error) {
	if len(userIDs) == 0 || endTime <= startTime {
		return nil, nil
	}
	var rows []RankingFreeUserLogCandidate
	err := LOG_DB.Table("logs").
		Select("id, user_id, created_at, metered_tokens, other").
		Where("user_id IN ?", userIDs).
		Where("type = ?", LogTypeConsume).
		Where("created_at >= ?", startTime).
		Where("created_at < ?", endTime).
		Where("metered_tokens > ?", 0).
		Find(&rows).Error
	return rows, err
}
```

- [ ] **步骤 4：实现 service 层响应类型和构建逻辑**

在 `service/rankings.go`：

1. 扩展 `RankingsResponse`：

```go
FreeUserHistory FreeUserHistorySeries `json:"free_user_history"`
```

2. 增加类型：

```go
const rankingFreeUserHistoryHours = 24

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
```

3. 将 `buildRankedFreeUsers` 改为内部构建，再导出响应。可增加 helper：

```go
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
		rows = append(rows, RankedFreeUser{Rank: item.Rank, DisplayName: item.DisplayName, TotalTokens: item.TotalTokens, Named: item.Named})
	}
	return rows
}
```

4. 增加 JSON number parsing helper，不使用标准库 marshal/unmarshal：

```go
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
```

5. 实现 `buildFreeUserHistory`，核心逻辑必须遵循：

```go
func buildFreeUserHistory(users []rankedFreeUserInternal) (FreeUserHistorySeries, error) {
	series := FreeUserHistorySeries{Hours: rankingFreeUserHistoryHours}
	if len(users) == 0 {
		return series, nil
	}
	userIDs := make([]int, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.UserID)
	}
	subs, err := model.GetRankingFreeUserSubscriptions(userIDs)
	if err != nil { return series, err }
	if len(subs) == 0 { return buildFreeUserHistoryPoints(users, map[int][]int64{}), nil }

	subsByID := make(map[int]model.RankingFreeUserSubscription, len(subs))
	minStart := subs[0].StartTime
	maxEnd := subs[0].StartTime + int64(rankingFreeUserHistoryHours*3600)
	for _, sub := range subs {
		subsByID[sub.ID] = sub
		if sub.StartTime < minStart { minStart = sub.StartTime }
		end := sub.StartTime + int64(rankingFreeUserHistoryHours*3600)
		if end > maxEnd { maxEnd = end }
	}

	logs, err := model.GetRankingFreeUserLogCandidates(userIDs, minStart, maxEnd)
	if err != nil { return series, err }
	tokensByUserHour := make(map[int][]int64, len(users))
	for _, user := range users { tokensByUserHour[user.UserID] = make([]int64, rankingFreeUserHistoryHours) }
	for _, candidate := range logs {
		var other map[string]interface{}
		if err := common.UnmarshalJsonStr(candidate.Other, &other); err != nil { continue }
		subID, ok := intFromOtherMapValue(other["subscription_id"]); if !ok { continue }
		consumed, ok := intFromOtherMapValue(other["subscription_tokens_consumed"]); if !ok || consumed <= 0 { continue }
		sub, ok := subsByID[subID]; if !ok || sub.UserID != candidate.UserID { continue }
		if candidate.CreatedAt < sub.StartTime || candidate.CreatedAt >= sub.StartTime+int64(rankingFreeUserHistoryHours*3600) { continue }
		hour := int((candidate.CreatedAt - sub.StartTime) / 3600)
		if hour < 0 || hour >= rankingFreeUserHistoryHours { continue }
		tokensByUserHour[candidate.UserID][hour] += int64(consumed)
	}
	return buildFreeUserHistoryPoints(users, tokensByUserHour), nil
}
```

实现时写成可编译的 Go 代码，不保留说明性草稿注释。因为 map 中的 slice 需要初始化，使用 `map[int][]int64` 存储每个用户 24 个小时桶。

同时增加 helper：

```go
func buildFreeUserHistoryPoints(users []rankedFreeUserInternal, tokensByUserHour map[int][]int64) FreeUserHistorySeries {
	series := FreeUserHistorySeries{Hours: rankingFreeUserHistoryHours}
	for _, user := range users {
		buckets := tokensByUserHour[user.UserID]
		if len(buckets) != rankingFreeUserHistoryHours { buckets = make([]int64, rankingFreeUserHistoryHours) }
		cumulative := int64(0)
		for hour := 0; hour < rankingFreeUserHistoryHours; hour++ {
			tokens := buckets[hour]
			cumulative += tokens
			series.Points = append(series.Points, FreeUserHistoryPoint{Rank: user.Rank, DisplayName: user.DisplayName, SeriesLabel: user.SeriesLabel, Hour: hour, HourLabel: fmt.Sprintf("%dh", hour), Tokens: tokens, CumulativeTokens: cumulative})
		}
	}
	return series
}
```

新增/更新 imports 时确保包含 `strconv`，并在使用 `common.UnmarshalJsonStr` 后保留 `common` import。

6. In `buildRankingsSnapshot`, call:

```go
freeUserInternals, freeUserTotalTokens := buildRankedFreeUserInternals(freeUserTotals)
freeUserHistory, err := buildFreeUserHistory(freeUserInternals)
if err != nil { return nil, err }
```

Return `FreeUsers: publicRankedFreeUsers(freeUserInternals), FreeUserHistory: freeUserHistory`.

- [ ] **步骤 5：运行后端测试验证通过**

运行：

```bash
go test ./service -run 'TestGetRankingsSnapshot.*FreeUserHistory|TestGetRankingsSnapshotRanksFreeSubscriptionTokenUsage|TestGetRankingsSnapshotFreeSubscriptionLeaderboardIsLifetime|TestGetRankingsSnapshotExcludesDeletedFreeUsers' -count=1
```

预期：PASS。

- [ ] **步骤 6：运行后端包级回归**

运行：

```bash
go test ./service ./controller -count=1
```

预期：PASS。

- [ ] **步骤 7：提交后端任务**

```bash
git add model/usedata_rankings.go service/rankings.go service/rankings_test.go
git commit -m "feat(rankings): 添加免费榜24小时趋势数据"
```

---

## 任务 2：前端免费榜图表组件

**文件：**
- 修改：`web/default/src/features/rankings/types.ts`
- 修改：`web/default/src/features/rankings/index.tsx`
- 修改：`web/default/src/features/rankings/components/free-users-section.tsx`
- 修改：`web/default/src/features/rankings/components/index.ts`
- 创建：`web/default/src/features/rankings/components/free-users-bar-chart.tsx`
- 创建：`web/default/src/features/rankings/components/free-users-line-chart.tsx`
- 可选创建：`web/default/src/features/rankings/components/free-users-list.tsx`

- [ ] **步骤 1：编写失败的前端静态测试**

先修改 `web/default/src/features/rankings/rankings-free-users.test.ts`，加入新断言。不要修改生产前端代码。测试应读取以下文件：

```ts
function readFreeUsersSectionSource(): string {
  return readFileSync('src/features/rankings/components/free-users-section.tsx', 'utf8')
}

function readFreeUsersBarChartSource(): string {
  return readFileSync('src/features/rankings/components/free-users-bar-chart.tsx', 'utf8')
}

function readFreeUsersLineChartSource(): string {
  return readFileSync('src/features/rankings/components/free-users-line-chart.tsx', 'utf8')
}
```

新增测试形态：

```ts
test('rankings snapshot exposes free-user history for chart views', () => {
  const types = readRankingsTypesSource()
  assert.match(types, /FreeUserHistoryPoint/)
  assert.match(types, /FreeUserHistorySeries/)
  assert.match(types, /free_user_history: FreeUserHistorySeries/)
  assert.match(types, /series_label: string/)
  assert.match(types, /hour_label: string/)
  assert.match(types, /cumulative_tokens: number/)

  const page = readRankingsIndexSource()
  assert.match(page, /history=\{snapshot\.free_user_history\}/)
})

test('free-user section wires bar and line chart views', () => {
  const section = readFreeUsersSectionSource()
  assert.match(section, /history: FreeUserHistorySeries/)
  assert.match(section, /FreeUsersBarChart/)
  assert.match(section, /FreeUsersLineChart/)
  assert.match(section, /Bar chart/)
  assert.match(section, /24-hour trend/)
  assert.match(section, /Hourly usage/)
  assert.match(section, /Cumulative usage/)
})

test('free-user bar chart is horizontal with user labels on the y axis', () => {
  const source = readFreeUsersBarChartSource()
  assert.match(source, /direction:\s*'horizontal'/)
  assert.match(source, /series_label/)
  assert.match(source, /rank/)
  assert.match(source, /display_name/)
  assert.match(source, /xField:\s*'total_tokens'/)
  assert.match(source, /yField:\s*'series_label'/)
  assert.match(source, /#\$\{row\.rank\} · \$\{row\.display_name\}/)
})

test('free-user line chart supports hourly and cumulative modes', () => {
  const source = readFreeUsersLineChartSource()
  assert.match(source, /FreeUserTrendMode/)
  assert.match(source, /tokens/)
  assert.match(source, /cumulative_tokens/)
  assert.match(source, /hour_label/)
  assert.match(source, /series_label/)
  assert.match(source, /No free-plan trend data available/)
})
```

继续保留并扩展旧 UI 负向断言：`MarketShareSection`、`PulseSection`、`VendorRanking`、`RankingMover`、`vendor_share_history`、`top_movers`、`top_droppers` 都不得出现在排行榜入口、组件导出或类型定义中。

- [ ] **步骤 2：运行前端测试验证失败**

运行：

```bash
cd web/default && bun test src/features/rankings/rankings-free-users.test.ts
```

预期：FAIL，因为新增组件、类型和 props 尚未实现。

- [ ] **步骤 3：更新前端类型与页面调用**

修改 `web/default/src/features/rankings/types.ts`：

```ts
export type FreeUserHistoryPoint = {
  rank: number
  display_name: string
  series_label: string
  hour: number
  hour_label: string
  tokens: number
  cumulative_tokens: number
}

export type FreeUserHistorySeries = {
  points: FreeUserHistoryPoint[]
  hours: number
}

export type RankingsSnapshot = {
  models: ModelRanking[]
  models_history: ModelHistorySeries
  free_users: FreeUserRanking[]
  free_user_total_tokens: number
  free_user_history: FreeUserHistorySeries
}
```

修改 `web/default/src/features/rankings/index.tsx`：

```tsx
<FreeUsersSection
  rows={snapshot.free_users}
  totalTokens={snapshot.free_user_total_tokens}
  history={snapshot.free_user_history}
/>
```

- [ ] **步骤 4：创建横向条形图组件**

创建 `web/default/src/features/rankings/components/free-users-bar-chart.tsx`。实现要点：

- Props 不解构。
- 使用 `useTranslation()`。
- 使用 `useChartTheme()`、`VCHART_OPTION`、`formatTokens`。
- 前端派生 data：

```ts
type FreeUserBarDatum = FreeUserRanking & { series_label: string }

function buildBarData(rows: FreeUserRanking[]): FreeUserBarDatum[] {
  return rows.map((row) => ({
    ...row,
    series_label: `#${row.rank} · ${row.display_name}`,
  }))
}
```

- VChart spec 必须包含：

```ts
{
  type: 'bar' as const,
  direction: 'horizontal' as const,
  data: [{ id: 'free-users-bar', values: data }],
  xField: 'total_tokens',
  yField: 'series_label',
  axes: [...],
  tooltip: {...},
}
```

- 空 rows 时显示 `No free-plan ranking data available`。

- [ ] **步骤 5：创建折线图组件**

创建 `web/default/src/features/rankings/components/free-users-line-chart.tsx`。实现要点：

```ts
export type FreeUserTrendMode = 'hourly' | 'cumulative'

type FreeUsersLineChartProps = {
  history: FreeUserHistorySeries
  mode: FreeUserTrendMode
}
```

- Props 不解构。
- 使用 `useTranslation()`。
- 只绘制前 10 名：过滤 `point.rank <= 10`。
- mode 为 `hourly` 时 yField 为 `tokens`；mode 为 `cumulative` 时 yField 为 `cumulative_tokens`。
- 静态测试需要断言存在 `mode === 'hourly' ? 'tokens' : 'cumulative_tokens'` 或等效明确分支，不能只让两个字段名同时出现在文件中。
- spec 必须使用 `xField: 'hour_label'`、`seriesField: 'series_label'`。
- 无 points 时显示 `No free-plan trend data available`。

- [ ] **步骤 6：改造 FreeUsersSection 容器**

修改 `web/default/src/features/rankings/components/free-users-section.tsx`：

```ts
type FreeUsersSectionProps = {
  rows: FreeUserRanking[]
  totalTokens: number
  history: FreeUserHistorySeries
}
```

新增状态：

```ts
type FreeUsersView = 'bar' | 'trend'
const [view, setView] = useState<FreeUsersView>('bar')
const [trendMode, setTrendMode] = useState<FreeUserTrendMode>('hourly')
```

UI：

- header 保持总 token。
- 主切换按钮文案：`Bar chart`、`24-hour trend`。
- 当 view 为 `bar`：渲染 `<FreeUsersBarChart rows={props.rows} />`。
- 当 view 为 `trend`：显示二级切换 `Hourly usage`、`Cumulative usage`，并渲染 `<FreeUsersLineChart history={props.history} mode={trendMode} />`。
- 图表下方保留简化榜单列表。若文件过长，提取 `free-users-list.tsx`。
- 如果拆出 `free-users-list.tsx`，该子组件也必须自行调用 `useTranslation()`，不要从父组件传入 `t`。

- [ ] **步骤 7：导出新组件并运行前端测试**

修改 `web/default/src/features/rankings/components/index.ts`：

```ts
export * from './free-users-bar-chart'
export * from './free-users-line-chart'
```

运行：

```bash
cd web/default && bun test src/features/rankings/rankings-free-users.test.ts
```

预期：PASS。

- [ ] **步骤 8：运行前端类型检查**

运行：

```bash
cd web/default && bun run typecheck
```

预期：PASS。

- [ ] **步骤 9：提交前端图表任务**

```bash
git add web/default/src/features/rankings/types.ts web/default/src/features/rankings/index.tsx web/default/src/features/rankings/components web/default/src/features/rankings/rankings-free-users.test.ts
git commit -m "feat(rankings): 添加免费榜图表视图"
```

---

## 任务 3：i18n 与最终验证

**文件：**
- 修改：`web/default/src/i18n/locales/en.json`
- 修改：`web/default/src/i18n/locales/zh.json`
- 修改：`web/default/src/i18n/locales/fr.json`
- 修改：`web/default/src/i18n/locales/ja.json`
- 修改：`web/default/src/i18n/locales/ru.json`
- 修改：`web/default/src/i18n/locales/vi.json`
- 修改：`web/default/src/features/rankings/rankings-free-users.test.ts`

- [ ] **步骤 1：扩展 i18n smoke test 并验证失败**

在 `web/default/src/features/rankings/rankings-free-users.test.ts` 增加 locale 读取 helper：

```ts
const chartI18nKeys = [
  'Bar chart',
  '24-hour trend',
  'Hourly usage',
  'Cumulative usage',
  'Usage after free-plan activation',
  'Compare each ranked user within their first 24 hours of free-plan access',
  'No free-plan trend data available',
  'Rank #{{rank}}',
] as const

function readLocale(locale: string): Record<string, string> {
  const content = readFileSync(`src/i18n/locales/${locale}.json`, 'utf8')
  return JSON.parse(content).translation as Record<string, string>
}

test('free-user chart i18n keys exist in all supported locales', () => {
  for (const locale of ['en', 'zh', 'fr', 'ja', 'ru', 'vi']) {
    const translation = readLocale(locale)
    for (const key of chartI18nKeys) {
      assert.equal(typeof translation[key], 'string', `${locale} missing ${key}`)
      assert.notEqual(translation[key].trim(), '', `${locale} empty ${key}`)
      if (locale !== 'en') {
        assert.ok(translation[key].includes('{{rank}}') === key.includes('{{rank}}'), `${locale} placeholder mismatch ${key}`)
        assert.notEqual(translation[key], key, `${locale} untranslated ${key}`)
      }
    }
  }
})
```

运行：

```bash
cd web/default && bun test src/features/rankings/rankings-free-users.test.ts
```

预期：FAIL，原因是 locale key 尚未全部补齐。

- [ ] **步骤 2：补齐六种语言翻译**

向六个 locale JSON 增加：

English:

```json
{
  "Bar chart": "Bar chart",
  "24-hour trend": "24-hour trend",
  "Hourly usage": "Hourly usage",
  "Cumulative usage": "Cumulative usage",
  "Usage after free-plan activation": "Usage after free-plan activation",
  "Compare each ranked user within their first 24 hours of free-plan access": "Compare each ranked user within their first 24 hours of free-plan access",
  "No free-plan trend data available": "No free-plan trend data available",
  "Rank #{{rank}}": "Rank #{{rank}}"
}
```

Chinese:

```json
{
  "Bar chart": "条形图",
  "24-hour trend": "24 小时趋势",
  "Hourly usage": "每小时用量",
  "Cumulative usage": "累计用量",
  "Usage after free-plan activation": "免费套餐开通后的用量",
  "Compare each ranked user within their first 24 hours of free-plan access": "比较每位上榜用户免费套餐开通后前 24 小时的用量",
  "No free-plan trend data available": "暂无免费套餐趋势数据",
  "Rank #{{rank}}": "第 {{rank}} 名"
}
```

French:

```json
{
  "Bar chart": "Graphique en barres",
  "24-hour trend": "Tendance sur 24 heures",
  "Hourly usage": "Utilisation horaire",
  "Cumulative usage": "Utilisation cumulée",
  "Usage after free-plan activation": "Utilisation après l’activation du forfait gratuit",
  "Compare each ranked user within their first 24 hours of free-plan access": "Comparez chaque utilisateur classé pendant ses 24 premières heures d’accès gratuit",
  "No free-plan trend data available": "Aucune donnée de tendance du forfait gratuit disponible",
  "Rank #{{rank}}": "Rang n° {{rank}}"
}
```

Japanese:

```json
{
  "Bar chart": "棒グラフ",
  "24-hour trend": "24 時間トレンド",
  "Hourly usage": "1 時間ごとの使用量",
  "Cumulative usage": "累計使用量",
  "Usage after free-plan activation": "無料プラン有効化後の使用量",
  "Compare each ranked user within their first 24 hours of free-plan access": "ランキング対象ユーザーの無料プラン開始後 24 時間の使用量を比較します",
  "No free-plan trend data available": "無料プランのトレンドデータはありません",
  "Rank #{{rank}}": "第 {{rank}} 位"
}
```

Russian:

```json
{
  "Bar chart": "Столбчатая диаграмма",
  "24-hour trend": "Тренд за 24 часа",
  "Hourly usage": "Почасовое использование",
  "Cumulative usage": "Накопленное использование",
  "Usage after free-plan activation": "Использование после активации бесплатного плана",
  "Compare each ranked user within their first 24 hours of free-plan access": "Сравните пользователей рейтинга за первые 24 часа бесплатного доступа",
  "No free-plan trend data available": "Нет данных тренда бесплатного плана",
  "Rank #{{rank}}": "Место № {{rank}}"
}
```

Vietnamese:

```json
{
  "Bar chart": "Biểu đồ thanh",
  "24-hour trend": "Xu hướng 24 giờ",
  "Hourly usage": "Mức dùng theo giờ",
  "Cumulative usage": "Mức dùng tích lũy",
  "Usage after free-plan activation": "Mức dùng sau khi kích hoạt gói miễn phí",
  "Compare each ranked user within their first 24 hours of free-plan access": "So sánh từng người dùng trong bảng xếp hạng trong 24 giờ đầu truy cập gói miễn phí",
  "No free-plan trend data available": "Chưa có dữ liệu xu hướng gói miễn phí",
  "Rank #{{rank}}": "Hạng #{{rank}}"
}
```

Use edit/write tools or a small Node script; do not use shell redirection.

- [ ] **步骤 3：运行 i18n 同步并检查报告**

运行：

```bash
cd web/default && bun run i18n:sync
```

然后读取 `web/default/src/i18n/locales/_reports/_sync-report.json`，确认每个 locale：

- `missingCount = 0`
- `extrasCount = 0`

Existing unrelated `untranslatedCount` may remain; do not expand scope to translate old unrelated keys.

- [ ] **步骤 4：运行所有目标验证**

运行：

```bash
go test ./service ./controller -count=1
cd web/default && bun test src/features/rankings/rankings-free-users.test.ts
cd web/default && bun run typecheck
git diff --check
```

预期：全部通过，无 diff whitespace error。

- [ ] **步骤 5：提交 i18n 与验证任务**

```bash
git add web/default/src/features/rankings/rankings-free-users.test.ts web/default/src/i18n/locales/en.json web/default/src/i18n/locales/zh.json web/default/src/i18n/locales/fr.json web/default/src/i18n/locales/ja.json web/default/src/i18n/locales/ru.json web/default/src/i18n/locales/vi.json
git commit -m "test(rankings): 补齐免费榜图表翻译验证"
```

---

## 最终审查与收尾

- [ ] **步骤 1：请求至少 3 个只读 review 子代理并发审查最终实现**

审查方向：

1. 后端统计与隐私。
2. 前端图表与类型。
3. 测试/i18n/回归覆盖。

若任一 review 返回必须修复项，按反馈修复并重新审查，直到全部 PASS。

- [ ] **步骤 2：运行最终验证**

```bash
go test ./service ./controller -count=1
cd web/default && bun test src/features/rankings/rankings-free-users.test.ts
cd web/default && bun run i18n:sync
cd web/default && bun run typecheck
git diff --check
```

读取 i18n report 并确认 `missingCount = 0`、`extrasCount = 0`。

- [ ] **步骤 3：提交最终修正**

如最终审查产生修正，按实际变更文件精确暂存并提交，例如：

```bash
git add model/usedata_rankings.go service/rankings.go service/rankings_test.go web/default/src/features/rankings/types.ts web/default/src/features/rankings/index.tsx web/default/src/features/rankings/components/free-users-section.tsx web/default/src/features/rankings/components/free-users-bar-chart.tsx web/default/src/features/rankings/components/free-users-line-chart.tsx web/default/src/features/rankings/components/free-users-list.tsx web/default/src/features/rankings/components/index.ts web/default/src/features/rankings/rankings-free-users.test.ts web/default/src/i18n/locales/en.json web/default/src/i18n/locales/zh.json web/default/src/i18n/locales/fr.json web/default/src/i18n/locales/ja.json web/default/src/i18n/locales/ru.json web/default/src/i18n/locales/vi.json
git commit -m "fix(rankings): 完善免费榜图表实现"
```

如果没有新增修正，不需要创建空提交。
