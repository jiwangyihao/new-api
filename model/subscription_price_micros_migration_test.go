package model

import (
	"database/sql"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSubscriptionPlanPriceMicrosMigrationLeavesLegacyRowPending(t *testing.T) {
	oldDB := DB
	oldSQLite := common.UsingSQLite
	oldMySQL := common.UsingMySQL
	oldPostgreSQL := common.UsingPostgreSQL
	db, err := gorm.Open(sqlite.Open("file:price-micros-legacy?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	t.Cleanup(func() {
		DB = oldDB
		common.UsingSQLite = oldSQLite
		common.UsingMySQL = oldMySQL
		common.UsingPostgreSQL = oldPostgreSQL
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.Exec(`CREATE TABLE subscription_plans (id integer PRIMARY KEY, title varchar(128) NOT NULL, price_amount decimal(10,6) NOT NULL)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO subscription_plans (id, title, price_amount) VALUES (1, 'legacy paid', 40.000000)`).Error)

	require.NoError(t, ensureSubscriptionPlanTableSQLite())
	var stored sql.NullInt64
	require.NoError(t, db.Raw(`SELECT price_amount_micros FROM subscription_plans WHERE id = 1`).Scan(&stored).Error)
	require.False(t, stored.Valid, "legacy prices must remain pending until #27 backfill")
}
