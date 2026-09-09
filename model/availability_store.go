package model

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	availabilityPending                = "pending"
	availabilityPendingSeconds   int64 = 24 * 60 * 60
	availabilityRetentionSeconds int64 = 31 * 24 * 60 * 60
	availabilityIdentitySeconds  int64 = 62 * 24 * 60 * 60
	availabilityPageSize               = 500
)

// These tables are independent from billing logs and never contain user input,
// upstream credentials, channel IDs, or raw error messages.
type availabilityRequest struct {
	ID              string `gorm:"primaryKey;type:varchar(64);index:idx_availability_completion,priority:2"`
	StartedAt       int64  `gorm:"not null;index:idx_availability_pending,priority:2;index"`
	CompletedAt     int64  `gorm:"not null;index:idx_availability_completion,priority:1"`
	GroupID         int    `gorm:"not null"`
	ModelName       string `gorm:"type:text;not null"`
	Outcome         string `gorm:"type:varchar(16);not null;index:idx_availability_pending,priority:1"`
	FirstResponseMs *int64
	Reason          string `gorm:"type:varchar(64);not null;default:''"`
}

func (availabilityRequest) TableName() string { return "availability_requests" }

type availabilityFrequency struct {
	ID           int64  `gorm:"primaryKey;autoIncrement"`
	Minute       int64  `gorm:"not null;uniqueIndex:uk_availability_frequency,priority:1;index"`
	GroupID      int    `gorm:"not null;uniqueIndex:uk_availability_frequency,priority:2"`
	ModelHash    string `gorm:"type:char(64);not null;uniqueIndex:uk_availability_frequency,priority:3"`
	Outcome      string `gorm:"type:varchar(16);not null;uniqueIndex:uk_availability_frequency,priority:4"`
	Latency      int64  `gorm:"not null;uniqueIndex:uk_availability_frequency,priority:5"`
	ModelName    string `gorm:"type:text;not null"`
	Frequency    int64  `gorm:"not null"`
	LastObserved int64  `gorm:"not null"`
}

func (availabilityFrequency) TableName() string { return "availability_frequencies" }

type availabilityCoverage struct {
	ID        int   `gorm:"primaryKey;autoIncrement:false"`
	StartedAt int64 `gorm:"not null"`
}

func (availabilityCoverage) TableName() string { return "availability_coverage" }

type availabilityGap struct {
	ID    string `gorm:"primaryKey;type:char(64)"`
	Start int64  `gorm:"not null;index"`
	End   int64  `gorm:"column:ended_at;not null;index"`
}

func (availabilityGap) TableName() string { return "availability_gaps" }

var availabilityMemoryGaps = struct {
	sync.Mutex
	byDB map[*gorm.DB][]availabilityGap
}{byDB: make(map[*gorm.DB][]availabilityGap)}

// MarkAvailabilityGap preserves evidence of a write failure until LOG_DB recovers.
// Adjacent failure intervals coalesce; this path must not require a working DB.
func MarkAvailabilityGap(start, end int64) {
	if end <= start {
		end = start + 1
	}
	availabilityMemoryGaps.Lock()
	defer availabilityMemoryGaps.Unlock()
	gaps := availabilityMemoryGaps.byDB[LOG_DB]
	for i := range gaps {
		if start <= gaps[i].End && end >= gaps[i].Start {
			if start < gaps[i].Start {
				gaps[i].Start = start
			}
			if end > gaps[i].End {
				gaps[i].End = end
			}
			availabilityMemoryGaps.byDB[LOG_DB] = gaps
			return
		}
	}
	availabilityMemoryGaps.byDB[LOG_DB] = append(gaps, availabilityGap{Start: start, End: end})
}

func availabilityMemoryGapSnapshot(db *gorm.DB) []availabilityGap {
	availabilityMemoryGaps.Lock()
	defer availabilityMemoryGaps.Unlock()
	return append([]availabilityGap(nil), availabilityMemoryGaps.byDB[db]...)
}

func availabilityPersistGaps(ctx context.Context, db *gorm.DB) error {
	gaps := availabilityMemoryGapSnapshot(db)
	if len(gaps) > availabilityPageSize {
		gaps = gaps[:availabilityPageSize]
	}
	for _, original := range gaps {
		gap := original
		gap.ID = fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d:%d", gap.Start, gap.End))))
		if err := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&gap).Error; err != nil {
			return err
		}
		// Remove only an unchanged interval. Concurrently extended gaps remain
		// visible and are persisted next pass; duplicate inserts are harmless.
		availabilityMemoryGaps.Lock()
		pending := availabilityMemoryGaps.byDB[db]
		for i, candidate := range pending {
			if candidate.Start == original.Start && candidate.End == original.End {
				pending = append(pending[:i], pending[i+1:]...)
				break
			}
		}
		if len(pending) == 0 {
			delete(availabilityMemoryGaps.byDB, db)
		} else {
			availabilityMemoryGaps.byDB[db] = pending
		}
		availabilityMemoryGaps.Unlock()
	}
	return nil
}

// MigrateAvailability starts coverage once; it never imputes historical billing
// logs whose final outcome and request-time ownership cannot be established.
func MigrateAvailability(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("availability log database is nil")
	}
	if err := db.AutoMigrate(&availabilityRequest{}, &availabilityFrequency{}, &availabilityCoverage{}, &availabilityGap{}); err != nil {
		return err
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&availabilityCoverage{ID: 1, StartedAt: time.Now().Unix()}).Error
}

// RecordAvailability commits each terminal outcome and its rollup atomically.
// The conditional pending update is the cross-process claim: a duplicate final
// write (even with different values) can never increment a histogram twice.
func RecordAvailability(ctx context.Context, observation AvailabilityObservation) error {
	if LOG_DB == nil {
		return fmt.Errorf("availability log database is nil")
	}
	if observation.ID == "" || len(observation.ID) > 64 || observation.StartedAt <= 0 || observation.GroupID < 0 {
		return fmt.Errorf("invalid availability observation identity")
	}
	switch observation.Outcome {
	case availabilityPending, AvailabilitySuccess, AvailabilityFailure, AvailabilityExcluded, AvailabilityUnknown:
	default:
		return fmt.Errorf("invalid availability outcome")
	}
	if observation.Outcome != availabilityPending && observation.CompletedAt < observation.StartedAt {
		return fmt.Errorf("availability completion precedes start")
	}
	if observation.FirstResponseMs != nil && *observation.FirstResponseMs < 0 {
		return fmt.Errorf("negative availability first response")
	}
	if observation.StartedAt < time.Now().Unix()-availabilityIdentitySeconds {
		return fmt.Errorf("availability observation exceeds replay retention")
	}
	reason := observation.Reason
	if len(reason) > 64 {
		reason = "unclassified"
	}
	for _, character := range reason {
		if !((character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_' || character == '-') {
			reason = "unclassified"
			break
		}
	}
	row := availabilityRequest{ID: observation.ID, StartedAt: observation.StartedAt, GroupID: observation.GroupID, ModelName: observation.ModelName, Outcome: availabilityPending}
	err := LOG_DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// MySQL's missing-row UPDATE can acquire a gap lock. Keep its original
		// insert-first order so concurrent terminal-before-pending writes do not deadlock.
		insertFirst := tx.Dialector.Name() == "mysql"
		if observation.Outcome == availabilityPending || insertFirst {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
			if observation.Outcome == availabilityPending {
				return nil
			}
		}

		update := func() (int64, error) {
			result := tx.Model(&availabilityRequest{}).Where("id = ? AND outcome = ?", observation.ID, availabilityPending).Updates(map[string]interface{}{
				"completed_at": observation.CompletedAt, "group_id": observation.GroupID, "model_name": observation.ModelName,
				"outcome": observation.Outcome, "first_response_ms": observation.FirstResponseMs, "reason": reason,
			})
			return result.RowsAffected, result.Error
		}

		updated, err := update()
		if err != nil {
			return err
		}
		if updated == 0 && !insertFirst {
			// The identity may be missing or already terminal. Insert without
			// overwriting either case, then retry the conditional transition.
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
			updated, err = update()
			if err != nil {
				return err
			}
		}
		if updated == 0 || observation.Outcome == AvailabilityExcluded {
			return nil
		}
		latency := int64(-1)
		if observation.Outcome == AvailabilitySuccess && observation.FirstResponseMs != nil {
			latency = *observation.FirstResponseMs
		}
		frequency := availabilityFrequency{Minute: observation.CompletedAt / 60 * 60, GroupID: observation.GroupID,
			ModelHash: fmt.Sprintf("%x", sha256.Sum256([]byte(observation.ModelName))), ModelName: observation.ModelName,
			Outcome: observation.Outcome, Latency: latency, Frequency: 1, LastObserved: observation.CompletedAt}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "minute"}, {Name: "group_id"}, {Name: "model_hash"}, {Name: "outcome"}, {Name: "latency"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"frequency":     gorm.Expr("? + 1", clause.Column{Table: "availability_frequencies", Name: "frequency"}),
				"last_observed": gorm.Expr("CASE WHEN ? > ? THEN ? ELSE ? END", clause.Column{Table: "availability_frequencies", Name: "last_observed"}, frequency.LastObserved, clause.Column{Table: "availability_frequencies", Name: "last_observed"}, frequency.LastObserved),
			}),
		}).Create(&frequency).Error
	})
	return err
}

// MaintainAvailability drains bounded transactions for up to twenty seconds.
// Completed and unresolved identities have a 62-day replay horizon; histograms
// retain 31 days. Stale pending gaps end at 48 hours, never poisoning all future
// windows, and a late final within the identity horizon still resolves the gap.
func MaintainAvailability(ctx context.Context, now time.Time) error {
	if LOG_DB == nil {
		return fmt.Errorf("availability log database is nil")
	}
	work, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	deadline := time.Now().Add(20 * time.Second)
	cutoff, identityCutoff := now.Unix()-availabilityRetentionSeconds, now.Unix()-availabilityIdentitySeconds
	defer availabilityInvalidateReport(LOG_DB)
	for time.Now().Before(deadline) {
		if err := availabilityPersistGaps(work, LOG_DB); err != nil {
			return err
		}
		removed := 0
		err := LOG_DB.WithContext(work).Transaction(func(tx *gorm.DB) error {
			var ids []string
			if err := tx.Model(&availabilityRequest{}).Where("started_at < ? AND (completed_at = 0 OR completed_at < ?)", identityCutoff, cutoff).Order("started_at, id").Limit(availabilityPageSize).Pluck("id", &ids).Error; err != nil {
				return err
			}
			removed += len(ids)
			if len(ids) > 0 {
				if err := tx.Where("id IN ?", ids).Delete(&availabilityRequest{}).Error; err != nil {
					return err
				}
			}
			var frequencyIDs []int64
			if err := tx.Model(&availabilityFrequency{}).Where("minute < ?", cutoff/60*60).Order("minute, id").Limit(availabilityPageSize).Pluck("id", &frequencyIDs).Error; err != nil {
				return err
			}
			removed += len(frequencyIDs)
			if len(frequencyIDs) > 0 {
				if err := tx.Where("id IN ?", frequencyIDs).Delete(&availabilityFrequency{}).Error; err != nil {
					return err
				}
			}
			ids = nil
			if err := tx.Model(&availabilityGap{}).Where("ended_at < ?", cutoff).Order("ended_at, id").Limit(availabilityPageSize).Pluck("id", &ids).Error; err != nil {
				return err
			}
			removed += len(ids)
			if len(ids) > 0 {
				return tx.Where("id IN ?", ids).Delete(&availabilityGap{}).Error
			}
			return nil
		})
		if err != nil {
			return err
		}
		if removed == 0 && len(availabilityMemoryGapSnapshot(LOG_DB)) == 0 {
			return nil
		}
	}
	return nil
}
