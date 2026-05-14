package model

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOAuthProviderLockRejectsDuplicateProviderUser(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&OAuthProviderLock{}))
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		_, err := CreateOAuthProviderLockTx(tx, "linuxdo", "same-user")
		return err
	}))
	require.Error(t, DB.Transaction(func(tx *gorm.DB) error {
		_, err := CreateOAuthProviderLockTx(tx, "linuxdo", "same-user")
		return err
	}))
}

func TestUserOAuthIndexesAllowBlankValues(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 79001, Username: "blank_oauth_a", Status: 1, AffCode: "blankA"}).Error)
	require.NoError(t, DB.Create(&User{Id: 79002, Username: "blank_oauth_b", Status: 1, AffCode: "blankB"}).Error)
}
