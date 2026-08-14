package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func lockForUpdate(tx *gorm.DB) *gorm.DB {
	if tx == nil || common.UsingSQLite {
		return tx
	}
	return tx.Clauses(clause.Locking{Strength: "UPDATE"})
}

// LockForUpdate applies the project-wide row-lock policy to a transaction.
// SQLite intentionally skips FOR UPDATE because the dialect does not support it.
func LockForUpdate(tx *gorm.DB) *gorm.DB {
	return lockForUpdate(tx)
}
