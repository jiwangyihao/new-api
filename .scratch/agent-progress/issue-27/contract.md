# Issue #27 并行实现合同

## 基线与目标

- 工作树：`C:/Users/34404/source/repos/new-api/.workspaces/new-api/issue-27-migration-final`
- 基线：`b45bc8694e7e7a2b15be9e2447b46e140090191f`
- 规格：`issue://jiwangyihao/new-api/27`、`docs/superpowers/specs/2026-08-02-credit-operational-remaining-value-spec.md`
- 本合同只固定并行实现接缝；产品语义仍以上述规格为准。

## 公共迁移接口

`model/credit_valuation_migration.go` 由 Marker/Engine Worker 独占，并提供：

```go
type CreditValuationMigrationMode string

const (
    CreditValuationMigrationModeDryRun CreditValuationMigrationMode = "dry_run"
    CreditValuationMigrationModeApply CreditValuationMigrationMode = "apply"
    CreditValuationMigrationModeVerify CreditValuationMigrationMode = "verify"
    CreditValuationMigrationModeRepairMissingAsUnknown CreditValuationMigrationMode = "repair_missing_as_unknown"
    CreditValuationMigrationModeSuspend CreditValuationMigrationMode = "suspend"
)

type CreditValuationMigrationRequest struct {
    Mode CreditValuationMigrationMode
    Version int
    BatchSize int
    Reason string
}

type CreditValuationMigrationFXSnapshot struct {
    SourceCurrency string `json:"source_currency"`
    ValuationCurrency string `json:"valuation_currency"`
    Numerator int64 `json:"numerator,string"`
    Denominator int64 `json:"denominator,string"`
    CapturedAt int64 `json:"captured_at"`
}

type CreditValuationMigrationReasonCount struct {
    Reason string `json:"reason"`
    Count int64 `json:"count"`
}

type CreditValuationMigrationBlocker struct {
    Code string `json:"code"`
    Count int64 `json:"count"`
}

type CreditValuationMigrationBatchBoundary struct {
    Entity string `json:"entity"`
    StartID int64 `json:"start_id"`
    EndID int64 `json:"end_id"`
    Rows int64 `json:"rows"`
}

func ValidateCreditValuationMigrationRequest(request CreditValuationMigrationRequest) error
func RunCreditValuationMigration(db *gorm.DB, request CreditValuationMigrationRequest) (CreditValuationMigrationReport, error)
```

最终 `CreditValuationMigrationReport` 必须使用结构体字段与稳定排序切片，不使用 map 作为 checksum 业务载荷；checksum 不包含运行时间、路径、耗时或进程信息。JSON 通过 `common.Marshal` 生成。

## 精确价格接口

`model/credit_valuation_price_backfill.go` 由 Price Worker 独占，并提供：

```go
type CreditValuationPlanPriceMigrationRequest struct {
    Apply bool
    BatchSize int
}

type CreditValuationPlanPriceDiagnostic struct {
    PlanID int `json:"plan_id"`
    RawValue string `json:"raw_value"`
    Reason string `json:"reason"`
}

type CreditValuationPlanPriceMigrationReport struct {
    RowsTotal int64 `json:"rows_total"`
    RowsAlreadyExact int64 `json:"rows_already_exact"`
    RowsBackfilled int64 `json:"rows_backfilled"`
    RowsInvalid int64 `json:"rows_invalid"`
    Diagnostics []CreditValuationPlanPriceDiagnostic `json:"diagnostics"`
    Batches []CreditValuationMigrationBatchBoundary `json:"batches"`
}

func RunCreditValuationPlanPriceMigration(db *gorm.DB, request CreditValuationPlanPriceMigrationRequest) (CreditValuationPlanPriceMigrationReport, error)
```

必须从数据库原始 DECIMAL/NUMERIC/SQLite 数值文本读取，不能先扫描到 `float64`。`Apply=false` 完全只读。非法行只诊断、不写入，并阻止 Engine 进入 ready。

## Credit 与 timed 历史重建接口

`model/credit_valuation_backfill.go` 与 `model/timed_subscription_valuation_backfill.go` 由 Historical Worker 独占，并提供：

```go
type CreditValuationHistoricalBackfillRequest struct {
    Apply bool
    RepairMissingAsUnknown bool
    MigrationVersion int
    BatchSize int
    ValuationCurrency string
    FX CreditValuationMigrationFXSnapshot
}

func RunCreditValuationHistoricalBackfill(db *gorm.DB, request CreditValuationHistoricalBackfillRequest) (CreditValuationHistoricalBackfillReport, error)
func RunTimedSubscriptionValuationHistoricalBackfill(db *gorm.DB, request CreditValuationHistoricalBackfillRequest) (TimedSubscriptionValuationHistoricalBackfillReport, error)
```

报告必须使用稳定切片并包含 estimated/unknown 数量、金额、原因和批次。Credit 公式固定为 `A/K/U/T/C/R`；历史结果不得标 exact。已有 `state_version>0` 状态与完整前向 exact timed grant 不得覆盖。

## 命令接缝

根命令 Worker 独占 `credit_valuation_command.go`、`credit_valuation_command_test.go`、`main.go` 和 `model/main.go` 中维护连接抽取。根 `main()` 在 `InitResources()` 之前识别 `credit-valuation-migrate`，完成后退出，因此不会启动 HTTP、Redis、定时器或后台 worker。命令只调用上面的 `ValidateCreditValuationMigrationRequest`、`RunCreditValuationMigration` 与维护数据库连接，不复制迁移逻辑。

## Marker/Engine 所有权

Marker/Engine Worker 独占 `model/credit_valuation_migration.go`、迁移引擎测试、稳定迁移错误码，以及 ready/suspended fail-closed 的最小生产接缝。它负责：

- `pending/running/ready/failed/suspended` 版本 CAS；
- deterministic report/checksum；
- 读取数据库中的 `USDExchangeRate` 原始字符串并冻结严格有理数；
- blocker（非终态预扣、异步任务、旧写会话）；
- apply/verify/repair/suspend 编排；
- 空库 auto-ready；
- ready 后状态缺失/不一致原子拒绝；
- 同版本 ready 重放 no-op；
- repair 只允许显式更高版本且只写 unknown；
- suspend 只允许 ready→suspended 且必须有原因。

## 文件所有权与并行纪律

- Command Worker：只改根命令文件、`main.go`、`model/main.go` 和自己的测试。
- Price Worker：只改 `model/credit_valuation_price_backfill*.go`。
- Historical Worker：只改 `model/credit_valuation_backfill*.go`、`model/timed_subscription_valuation_backfill*.go`。
- Marker/Engine Worker：只改 `model/credit_valuation_migration*.go`、必要的 `model/errors.go` 和最小 fail-closed 调用点。
- 不得修改其他 Worker 独占文件；遇到共享接口问题通过 Orca ask/escalation，不自行改名。
- Worker 不运行 formatter、lint、build、测试或项目命令；统一由协调器在合流后运行。
- 所有 Worker 将进度写入 `.scratch/agent-progress/issue-27/worker-<name>-status.md`，不得覆盖本合同。
