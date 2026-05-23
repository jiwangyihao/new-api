package model

import (
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestUpsertPerfMetricAccumulatesCounters(t *testing.T) {
	truncateTables(t)
	requireNoErrorPerfMetricTest(t, DB.AutoMigrate(&PerfMetric{}))
	requireNoErrorPerfMetricTest(t, DB.Exec("DELETE FROM perf_metrics").Error)

	first := &PerfMetric{
		ModelName:      "gpt-5.5",
		Group:          "default",
		BucketTs:       1779190000,
		RequestCount:   2,
		SuccessCount:   1,
		TotalLatencyMs: 300,
		TtftSumMs:      50,
		TtftCount:      1,
		OutputTokens:   20,
		GenerationMs:   250,
	}
	second := &PerfMetric{
		ModelName:      "gpt-5.5",
		Group:          "default",
		BucketTs:       1779190000,
		RequestCount:   3,
		SuccessCount:   2,
		TotalLatencyMs: 700,
		TtftSumMs:      90,
		TtftCount:      2,
		OutputTokens:   80,
		GenerationMs:   600,
	}

	requireNoErrorPerfMetricTest(t, UpsertPerfMetric(first))
	requireNoErrorPerfMetricTest(t, UpsertPerfMetric(second))

	var got PerfMetric
	requireNoErrorPerfMetricTest(t, DB.Where("model_name = ? AND bucket_ts = ?", "gpt-5.5", int64(1779190000)).First(&got).Error)

	if got.RequestCount != 5 || got.SuccessCount != 3 || got.TotalLatencyMs != 1000 || got.TtftSumMs != 140 || got.TtftCount != 3 || got.OutputTokens != 100 || got.GenerationMs != 850 {
		t.Fatalf("unexpected accumulated metric: %+v", got)
	}
}

func TestUpsertPerfMetricIgnoresLegacyGroupDimension(t *testing.T) {
	truncateTables(t)
	requireNoErrorPerfMetricTest(t, DB.AutoMigrate(&PerfMetric{}))
	requireNoErrorPerfMetricTest(t, DB.Exec("DELETE FROM perf_metrics").Error)

	first := &PerfMetric{ModelName: "gpt-db-perf", Group: "vip", BucketTs: 1779190100, RequestCount: 1, SuccessCount: 1, TotalLatencyMs: 100, OutputTokens: 10, GenerationMs: 1000}
	second := &PerfMetric{ModelName: "gpt-db-perf", Group: "default", BucketTs: 1779190100, RequestCount: 2, SuccessCount: 1, TotalLatencyMs: 500, OutputTokens: 20, GenerationMs: 1000}
	requireNoErrorPerfMetricTest(t, UpsertPerfMetric(first))
	requireNoErrorPerfMetricTest(t, UpsertPerfMetric(second))

	rows, err := GetPerfMetrics("gpt-db-perf", "missing", 1779190090, 1779190110)
	requireNoErrorPerfMetricTest(t, err)
	if len(rows) != 1 {
		t.Fatalf("expected one merged metric row, got %d: %+v", len(rows), rows)
	}
	if rows[0].Group != "" || rows[0].RequestCount != 3 || rows[0].TotalLatencyMs != 600 {
		t.Fatalf("unexpected merged metric row: %+v", rows[0])
	}
}

func TestUpsertPerfMetricPostgresSQLQualifiesConflictColumns(t *testing.T) {
	stmt := DB.Session(&gorm.Session{DryRun: true}).Clauses(perfMetricUpsertClause(&PerfMetric{
		RequestCount:   1,
		SuccessCount:   1,
		TotalLatencyMs: 10,
		TtftSumMs:      2,
		TtftCount:      1,
		OutputTokens:   5,
		GenerationMs:   8,
	})).Create(&PerfMetric{
		ModelName:    "probe-model",
		Group:        "default",
		BucketTs:     1779190000,
		RequestCount: 1,
	}).Statement
	if stmt.Error != nil {
		t.Fatal(stmt.Error)
	}

	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  "host=localhost user=test dbname=test sslmode=disable",
		PreferSimpleProtocol: true,
		Conn:                 stmt.ConnPool,
	}), &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	pgStmt := db.Clauses(perfMetricUpsertClause(&PerfMetric{
		RequestCount:   1,
		SuccessCount:   1,
		TotalLatencyMs: 10,
		TtftSumMs:      2,
		TtftCount:      1,
		OutputTokens:   5,
		GenerationMs:   8,
	})).Create(&PerfMetric{
		ModelName:    "probe-model",
		Group:        "default",
		BucketTs:     1779190000,
		RequestCount: 1,
	}).Statement
	if pgStmt.Error != nil {
		t.Fatal(pgStmt.Error)
	}
	sql := pgStmt.SQL.String()
	if strings.Contains(sql, "generation_ms=\"generation_ms\"") || strings.Contains(sql, "generation_ms +") {
		t.Fatalf("PostgreSQL upsert must qualify existing columns, got SQL: %s", sql)
	}
	if !strings.Contains(sql, "\"perf_metrics\".\"generation_ms\"") {
		t.Fatalf("PostgreSQL upsert should reference the perf_metrics generation_ms column, got SQL: %s", sql)
	}
}

func requireNoErrorPerfMetricTest(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
