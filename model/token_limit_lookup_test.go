package model

import (
	"errors"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func loadTokenLimitEnabledProjected(db *gorm.DB, id, userID int) (bool, error) {
	return tokenLimitEnabledTx(db, id, userID)
}

func setupTokenLimitLookupBenchmarkDB(b *testing.B) *gorm.DB {
	b.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", b.Name())), &gorm.Config{})
	require.NoError(b, err)
	require.NoError(b, db.AutoMigrate(&Token{}))
	require.NoError(b, db.Create(&Token{Id: 1, UserId: 2, Key: "bench-token-limit", Status: common.TokenStatusEnabled, ExpiredTime: -1, TokenLimitEnabled: true, TokenLimit: 100}).Error)
	return db
}

func TestTokenLimitEnabledProjectedSemantics(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Token{}))
	for _, token := range []Token{
		{Id: 1, UserId: 10, Key: "enabled", TokenLimitEnabled: true, TokenLimit: 100},
		{Id: 2, UserId: 10, Key: "disabled", TokenLimitEnabled: false, TokenLimit: 100},
		{Id: 3, UserId: 10, Key: "zero", TokenLimitEnabled: true, TokenLimit: 0},
		{Id: 4, UserId: 10, Key: "deleted", TokenLimitEnabled: true, TokenLimit: 100},
	} {
		require.NoError(t, db.Create(&token).Error)
	}
	require.NoError(t, db.Delete(&Token{}, 4).Error)

	for _, tc := range []struct {
		name    string
		id      int
		userID  int
		want    bool
		wantErr error
	}{
		{name: "enabled", id: 1, userID: 10, want: true},
		{name: "disabled", id: 2, userID: 10, want: false},
		{name: "zero_limit", id: 3, userID: 10, want: false},
		{name: "deleted", id: 4, userID: 10, wantErr: gorm.ErrRecordNotFound},
		{name: "wrong_user", id: 2, userID: 11, wantErr: gorm.ErrRecordNotFound},
		{name: "missing", id: 99, userID: 10, wantErr: gorm.ErrRecordNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tokenLimitEnabledTx(db, tc.id, tc.userID)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func BenchmarkTokenLimitEnabledLookup(b *testing.B) {
	db := setupTokenLimitLookupBenchmarkDB(b)
	for _, tc := range []struct {
		name   string
		id     int
		userID int
	}{
		{name: "hit", id: 1, userID: 2},
		{name: "miss", id: 99, userID: 2},
	} {
		b.Run(tc.name+"/gorm_full", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var token Token
				err := db.Where("id = ? AND user_id = ?", tc.id, tc.userID).First(&token).Error
				if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					b.Fatal(err)
				}
			}
		})
		b.Run(tc.name+"/projected", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := loadTokenLimitEnabledProjected(db, tc.id, tc.userID)
				if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					b.Fatal(err)
				}
			}
		})
	}
}
