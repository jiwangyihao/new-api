package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type OAuthProviderLock struct {
	Id             int    `json:"id" gorm:"primaryKey"`
	Provider       string `json:"provider" gorm:"type:varchar(64);not null;uniqueIndex:ux_oauth_provider_user"`
	ProviderUserId string `json:"provider_user_id" gorm:"type:varchar(256);not null;uniqueIndex:ux_oauth_provider_user"`
	UserId         int    `json:"user_id" gorm:"index"`
	CreatedAt      int64  `json:"created_at" gorm:"type:bigint"`
}

func (l *OAuthProviderLock) BeforeCreate(tx *gorm.DB) error {
	l.Provider = strings.TrimSpace(l.Provider)
	l.ProviderUserId = strings.TrimSpace(l.ProviderUserId)
	l.CreatedAt = common.GetTimestamp()
	return nil
}

func CreateOAuthProviderLockTx(tx *gorm.DB, provider string, providerUserId string) (*OAuthProviderLock, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	provider = strings.TrimSpace(provider)
	providerUserId = strings.TrimSpace(providerUserId)
	if provider == "" || providerUserId == "" {
		return nil, errors.New("invalid oauth provider lock")
	}
	lock := &OAuthProviderLock{Provider: provider, ProviderUserId: providerUserId}
	if err := tx.Create(lock).Error; err != nil {
		return nil, err
	}
	return lock, nil
}

func BindOAuthProviderLockTx(tx *gorm.DB, lockId int, userId int) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	if lockId <= 0 || userId <= 0 {
		return errors.New("invalid oauth provider lock binding")
	}
	return tx.Model(&OAuthProviderLock{}).Where("id = ?", lockId).Update("user_id", userId).Error
}
