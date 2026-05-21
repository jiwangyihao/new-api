package monitor

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	"gorm.io/gorm"
)

var defaultPostgresTables = []string{"consume_logs", "subscription_pre_consume_records", "user_subscriptions", "tokens"}

func LoadPostgresSnapshot(db *gorm.DB, tableNames []string) artifact.PostgresSnapshot {
	if db == nil {
		return artifact.PostgresSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "postgres database is not configured"}}
	}
	if len(tableNames) == 0 {
		tableNames = defaultPostgresTables
	}
	snapshot := artifact.PostgresSnapshot{Statused: artifact.Statused{Status: "ok"}, Rows: make(map[string]int64, len(tableNames))}
	var active int64
	if err := db.Raw("SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND state = 'active'").Scan(&active).Error; err != nil {
		return postgresUnavailable("pg_stat_activity active: %v", err)
	}
	snapshot.ActiveConnections = int(active)
	var idle int64
	if err := db.Raw("SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND state = 'idle'").Scan(&idle).Error; err != nil {
		return postgresUnavailable("pg_stat_activity idle: %v", err)
	}
	snapshot.IdleConnections = int(idle)
	var size uint64
	if err := db.Raw("SELECT pg_database_size(current_database())").Scan(&size).Error; err != nil {
		return postgresUnavailable("pg_database_size: %v", err)
	}
	snapshot.DatabaseSizeBytes = size
	var waitingLocks int64
	if err := db.Raw("SELECT count(*) FROM pg_locks WHERE NOT granted").Scan(&waitingLocks).Error; err != nil {
		return postgresUnavailable("pg_locks waiting: %v", err)
	}
	snapshot.WaitingLocks = int(waitingLocks)
	for _, table := range tableNames {
		table = strings.TrimSpace(table)
		if !allowedPostgresSnapshotTable(table) {
			return postgresUnavailable("unsupported table %q", table)
		}
		count, err := countPostgresSnapshotRows(db, table)
		if err != nil {
			return postgresUnavailable("%s rows: %v", table, err)
		}
		snapshot.Rows[table] = count
	}
	return snapshot
}

func allowedPostgresSnapshotTable(table string) bool {
	for _, allowed := range defaultPostgresTables {
		if table == allowed {
			return true
		}
	}
	return false
}

func countPostgresSnapshotRows(db *gorm.DB, table string) (int64, error) {
	var count int64
	switch table {
	case "consume_logs":
		return count, db.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume).Count(&count).Error
	case "subscription_pre_consume_records":
		return count, db.Model(&model.SubscriptionPreConsumeRecord{}).Count(&count).Error
	case "user_subscriptions":
		return count, db.Model(&model.UserSubscription{}).Count(&count).Error
	case "tokens":
		return count, db.Model(&model.Token{}).Count(&count).Error
	default:
		return 0, fmt.Errorf("unsupported table %q", table)
	}
}

func postgresUnavailable(reason string, args ...any) artifact.PostgresSnapshot {
	return artifact.PostgresSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: fmt.Sprintf(reason, args...)}}
}
