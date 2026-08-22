package model

import (
	"errors"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

const (
	BatchUpdateTypeUserQuota = iota
	BatchUpdateTypeTokenQuota
	BatchUpdateTypeUsedQuota
	BatchUpdateTypeChannelUsedQuota
	BatchUpdateTypeRequestCount
	BatchUpdateTypeCount // if you add a new type, you need to add a new map and a new lock
)

type BatchUpdatePending struct {
	ByType map[int]int `json:"by_type"`
	Total  int         `json:"total"`
}

func (p BatchUpdatePending) String() string {
	return "pending=" + common.GetJsonString(p.ByType)
}

var batchUpdateStores []map[int]int
var batchUpdateLocks []sync.Mutex
var batchUpdateFlushLocks []sync.Mutex

func init() {
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateStores = append(batchUpdateStores, make(map[int]int))
		batchUpdateLocks = append(batchUpdateLocks, sync.Mutex{})
		batchUpdateFlushLocks = append(batchUpdateFlushLocks, sync.Mutex{})
	}
}

func InitBatchUpdater() {
	gopool.Go(func() {
		for {
			time.Sleep(time.Duration(common.BatchUpdateInterval) * time.Second)
			batchUpdate()
		}
	})
}

func addNewRecord(type_ int, id int, value int) {
	batchUpdateLocks[type_].Lock()
	defer batchUpdateLocks[type_].Unlock()
	if _, ok := batchUpdateStores[type_][id]; !ok {
		batchUpdateStores[type_][id] = value
	} else {
		batchUpdateStores[type_][id] += value
	}
}

func AddUserQuotaBatchForMigrationDrain(id int, value int) {
	addNewRecord(BatchUpdateTypeUserQuota, id, value)
}

func BatchUpdatePendingSnapshot() BatchUpdatePending {
	snapshot := BatchUpdatePending{ByType: make(map[int]int, BatchUpdateTypeCount)}
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateFlushLocks[i].Lock()
		batchUpdateLocks[i].Lock()
		count := len(batchUpdateStores[i])
		batchUpdateLocks[i].Unlock()
		batchUpdateFlushLocks[i].Unlock()
		snapshot.ByType[i] = count
		snapshot.Total += count
	}
	return snapshot
}

func BatchUpdatePendingCount(type_ int) int {
	if type_ < 0 || type_ >= BatchUpdateTypeCount {
		return 0
	}
	batchUpdateFlushLocks[type_].Lock()
	defer batchUpdateFlushLocks[type_].Unlock()
	batchUpdateLocks[type_].Lock()
	defer batchUpdateLocks[type_].Unlock()
	return len(batchUpdateStores[type_])
}

var migrationFlushAfterSwapHookForTest func()

func FlushBatchUpdateTypeForMigration(type_ int) error {
	if type_ < 0 || type_ >= BatchUpdateTypeCount {
		return errors.New("unsupported migration batch update type")
	}
	batchUpdateFlushLocks[type_].Lock()
	defer batchUpdateFlushLocks[type_].Unlock()

	batchUpdateLocks[type_].Lock()
	snapshot := batchUpdateStores[type_]
	batchUpdateStores[type_] = make(map[int]int)
	batchUpdateLocks[type_].Unlock()

	if migrationFlushAfterSwapHookForTest != nil {
		migrationFlushAfterSwapHookForTest()
	}

	flushed := make(map[int]struct{}, len(snapshot))
	for key, value := range snapshot {
		if value == 0 {
			flushed[key] = struct{}{}
			continue
		}
		if err := flushBatchUpdateRecordForMigration(type_, key, value); err != nil {
			batchUpdateLocks[type_].Lock()
			for pendingKey, pendingValue := range snapshot {
				if pendingValue == 0 {
					continue
				}
				if _, ok := flushed[pendingKey]; ok {
					continue
				}
				batchUpdateStores[type_][pendingKey] += pendingValue
			}
			batchUpdateLocks[type_].Unlock()
			return err
		}
		flushed[key] = struct{}{}
	}
	return nil
}

func FlushAllBatchUpdatesForMigration() error {
	for type_ := 0; type_ < BatchUpdateTypeCount; type_++ {
		if err := FlushBatchUpdateTypeForMigration(type_); err != nil {
			return err
		}
	}
	return nil
}
func FlushAllWritersForMigration() error {
	if err := FlushAllBatchUpdatesForMigration(); err != nil {
		return err
	}
	FlushUsageCounterUpdates()
	FlushSubscriptionTokenDeltaUpdates()
	FlushConsumeLogUpdates()
	return nil
}

func flushBatchUpdateRecordForMigration(type_, id, value int) error {
	switch type_ {
	case BatchUpdateTypeUserQuota:
		return flushUserQuotaForMigration(id, value)
	case BatchUpdateTypeTokenQuota:
		return increaseTokenQuota(id, value)
	case BatchUpdateTypeUsedQuota:
		return DB.Model(&User{}).Where("id = ?", id).Update("used_quota", gorm.Expr("used_quota + ?", value)).Error
	case BatchUpdateTypeChannelUsedQuota:
		return DB.Model(&Channel{}).Where("id = ?", id).Update("used_quota", gorm.Expr("used_quota + ?", value)).Error
	case BatchUpdateTypeRequestCount:
		return DB.Model(&User{}).Where("id = ?", id).Update("request_count", gorm.Expr("request_count + ?", value)).Error
	default:
		return errors.New("unsupported migration batch update type")
	}
}

func flushUserQuotaForMigration(id int, quota int) error {
	if quota == 0 {
		return nil
	}
	result := DB.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota + ?", quota))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	var count int64
	if err := DB.Model(&User{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func batchUpdate() {
	// check if there's any data to update
	hasData := false
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateLocks[i].Lock()
		if len(batchUpdateStores[i]) > 0 {
			hasData = true
			batchUpdateLocks[i].Unlock()
			break
		}
		batchUpdateLocks[i].Unlock()
	}

	if !hasData {
		return
	}

	common.SysLog("batch update started")
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateFlushLocks[i].Lock()
		batchUpdateLocks[i].Lock()
		store := batchUpdateStores[i]
		batchUpdateStores[i] = make(map[int]int)
		batchUpdateLocks[i].Unlock()
		for key, value := range store {
			switch i {
			case BatchUpdateTypeUserQuota:
				err := increaseUserQuota(key, value)
				if err != nil {
					common.SysLog("failed to batch update user quota: " + err.Error())
				}
			case BatchUpdateTypeTokenQuota:
				err := increaseTokenQuota(key, value)
				if err != nil {
					common.SysLog("failed to batch update token quota: " + err.Error())
				}
			case BatchUpdateTypeUsedQuota:
				updateUserUsedQuota(key, value)
			case BatchUpdateTypeRequestCount:
				updateUserRequestCount(key, value)
			case BatchUpdateTypeChannelUsedQuota:
				updateChannelUsedQuota(key, value)
			}
		}
		batchUpdateFlushLocks[i].Unlock()
	}
	common.SysLog("batch update finished")
}

func RecordExist(err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}

func shouldUpdateRedis(fromDB bool, err error) bool {
	return common.RedisEnabled && fromDB && err == nil
}
