package model

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestConsumeTrialCode_MaxRedemptionsAtomicGuard(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&TrialCode{}, &TrialRedemption{}))
	plan := seedTrialPlanForTest(t, 7631)
	seedTrialCodeForTest(t, 7632, "ONCE", plan.Id)
	require.NoError(t, DB.Model(&TrialCode{}).Where("id = ?", 7632).Update("max_redemptions", 1).Error)
	require.NoError(t, DB.Create(&User{Id: 7633, Username: "trial_once_a", Status: common.UserStatusEnabled, AffCode: "aff7633"}).Error)
	require.NoError(t, DB.Create(&User{Id: 7634, Username: "trial_once_b", Status: common.UserStatusEnabled, AffCode: "aff7634"}).Error)

	var successes atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, userId := range []int{7633, 7634} {
		wg.Add(1)
		go func(userId int) {
			defer wg.Done()
			<-start
			err := DB.Transaction(func(tx *gorm.DB) error {
				_, err := ConsumeTrialCode(tx, userId, "ONCE")
				return err
			})
			if err == nil {
				successes.Add(1)
			}
		}(userId)
	}
	close(start)
	wg.Wait()

	assert.Equal(t, int32(1), successes.Load())
	var redemptionCount int64
	require.NoError(t, DB.Model(&TrialRedemption{}).Where("trial_code_id = ?", 7632).Count(&redemptionCount).Error)
	assert.Equal(t, int64(1), redemptionCount)
	var trialCode TrialCode
	require.NoError(t, DB.First(&trialCode, 7632).Error)
	assert.Equal(t, 1, trialCode.RedeemedCount)
}
