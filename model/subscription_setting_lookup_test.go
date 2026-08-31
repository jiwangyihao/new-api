package model

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func loadUserSettingGORMForBenchmark(db *gorm.DB, userID int) (string, bool, error) {
	var user User
	result := lockForUpdate(db.Select("setting").Where("id = ?", userID)).First(&user)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return "", false, nil
		}
		return "", false, result.Error
	}
	return user.Setting, true, nil
}

func loadUserSettingRowForBenchmark(db *gorm.DB, userID int) (string, bool, error) {
	setting, err := loadUserSettingTx(db, userID, true)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	return setting, true, nil
}

func TestLoadUserSettingTxMatchesGORM(t *testing.T) {
	db := setupCreditValuationTracerTestDB(t)
	require.NoError(t, db.Create(&User{Id: 98_101, Username: "setting-row", AffCode: "setting-row-aff", Setting: `{"active_subscription_id":42}`}).Error)
	require.NoError(t, db.Create(&User{Id: 98_102, Username: "setting-null", AffCode: "setting-null-aff"}).Update("setting", nil).Error)
	require.NoError(t, db.Create(&User{Id: 98_103, Username: "setting-deleted", AffCode: "setting-deleted-aff", Setting: `{"active_subscription_id":43}`}).Error)
	require.NoError(t, db.Delete(&User{}, 98_103).Error)
	for _, userID := range []int{98_101, 98_102, 98_103, 98_199} {
		gormSetting, gormFound, gormErr := loadUserSettingGORMForBenchmark(db, userID)
		rowSetting, rowFound, rowErr := loadUserSettingRowForBenchmark(db, userID)
		require.Equal(t, gormFound, rowFound)
		require.Equal(t, gormSetting, rowSetting)
		if gormErr == nil {
			require.NoError(t, rowErr)
		} else {
			require.ErrorIs(t, rowErr, gorm.ErrRecordNotFound)
		}
	}
}

func BenchmarkLoadUserSettingSingleColumn(b *testing.B) {
	name := strings.NewReplacer("/", "_", " ", "_").Replace(b.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{PrepareStmt: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		b.Fatal(err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(b, db.Create(&User{Id: 98_201, Username: "setting-bench", Setting: `{"subscription_billing_strategy":"single_active","active_subscription_id":42}`}).Error)
	for _, userID := range []int{98_201, 98_299} {
		name := "hit"
		if userID == 98_299 {
			name = "miss"
		}
		b.Run(name+"/gorm", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _, err := loadUserSettingGORMForBenchmark(db, userID)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(name+"/row", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _, err := loadUserSettingRowForBenchmark(db, userID)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
