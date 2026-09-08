package model

import (
	"database/sql"

	"gorm.io/gorm"
)

func (b *availabilityReportBuilder) load(tx *gorm.DB, ids []int) error {
	var coverage availabilityCoverage
	if err := tx.First(&coverage, 1).Error; err != nil {
		return err
	}
	b.report.CoverageStart = coverage.StartedAt
	start, end, current := b.report.WindowStart, b.report.WindowEnd, b.report.CurrentStart
	firstFull, lastFull := (start+59)/60*60, end/60*60
	// Discover retained models before applying global gaps. Last observation is
	// independent of the selected range; excluded traffic is not an observation.
	type lastRow struct {
		GroupID      int
		ModelName    string
		LastObserved int64
	}
	for offset := 0; ; offset += availabilityPageSize {
		var rows []lastRow
		if err := tx.Model(&availabilityFrequency{}).Select("group_id, model_name, MAX(last_observed) AS last_observed").Where("minute >= ? AND minute < ? AND group_id IN ?", end-availabilityRetentionSeconds, lastFull, ids).Group("group_id, model_hash, model_name").Order("group_id, model_hash, model_name").Offset(offset).Limit(availabilityPageSize).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if row.GroupID != 0 && row.ModelName != "" {
				b.series(row.GroupID, row.ModelName)
			}
		}
		if len(rows) < availabilityPageSize {
			break
		}
	}
	for offset := 0; ; offset += availabilityPageSize {
		var rows []lastRow
		if err := tx.Model(&availabilityFrequency{}).Select("group_id, model_name, MAX(last_observed) AS last_observed").Where("minute >= ? AND minute < ? AND group_id IN ? AND outcome IN ?", end-availabilityRetentionSeconds, lastFull, ids, []string{AvailabilitySuccess, AvailabilityFailure}).Group("group_id, model_hash, model_name").Order("group_id, model_hash, model_name").Offset(offset).Limit(availabilityPageSize).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			g := b.groups[row.GroupID]
			if g == nil {
				continue
			}
			if row.LastObserved > g.group.last {
				g.group.last = row.LastObserved
			}
			if row.ModelName != "" {
				b.series(row.GroupID, row.ModelName).last = row.LastObserved
			}
		}
		if len(rows) < availabilityPageSize {
			break
		}
	}
	// Read partial-minute requests first, also discovering models that appear
	// for the first time in the trailing incomplete minute.
	minutes := []int64{}
	if start%60 != 0 {
		minutes = append(minutes, start/60*60, current/60*60)
	}
	if end%60 != 0 {
		minutes = append(minutes, lastFull)
	}
	type edgeGapKey struct {
		group int
		model string
		at    int64
	}
	edgeGaps := map[edgeGapKey]bool{}
	for _, minute := range minutes {
		cursorID := ""
		cursorAt := minute
		for {
			var rows []availabilityRequest
			if err := tx.Where("completed_at >= ? AND completed_at < ? AND (completed_at > ? OR (completed_at = ? AND id > ?)) AND group_id IN ? AND outcome IN ?", minute, minute+60, cursorAt, cursorAt, cursorID, ids, []string{AvailabilitySuccess, AvailabilityFailure, AvailabilityUnknown}).Order("completed_at, id").Limit(availabilityPageSize).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				cursorID, cursorAt = row.ID, row.CompletedAt
				if row.CompletedAt < start || row.CompletedAt >= end {
					continue
				}
				latency := int64(-1)
				if row.Outcome == AvailabilitySuccess && row.FirstResponseMs != nil {
					latency = *row.FirstResponseMs
				}
				if row.GroupID == 0 || row.Outcome == AvailabilityUnknown {
					model := row.ModelName
					if row.GroupID == 0 {
						model = ""
					}
					edgeGaps[edgeGapKey{group: row.GroupID, model: model, at: row.CompletedAt}] = true
				}
				if row.GroupID == 0 {
					continue
				}
				if minute == current/60*60 && minute >= firstFull && minute < lastFull {
					if row.CompletedAt >= current {
						if g := b.groups[row.GroupID]; g != nil {
							g.group.current.add(row.Outcome, 1, latency, row.CompletedAt)
							if row.ModelName != "" {
								b.series(row.GroupID, row.ModelName).current.add(row.Outcome, 1, latency, row.CompletedAt)
							}
						}
					}
				} else {
					b.add(row.GroupID, row.ModelName, row.CompletedAt, row.Outcome, 1, latency, row.CompletedAt)
				}
			}
			if len(rows) < availabilityPageSize {
				break
			}
		}
	}
	var cursor int64
	for {
		var rows []availabilityFrequency
		if err := tx.Where("id > ? AND minute >= ? AND minute < ? AND group_id IN ?", cursor, firstFull, lastFull, ids).Order("id").Limit(availabilityPageSize).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if row.GroupID != 0 {
				b.add(row.GroupID, row.ModelName, row.Minute, row.Outcome, row.Frequency, row.Latency, row.LastObserved)
			}
			if (row.GroupID == 0 || row.Outcome == AvailabilityUnknown) && (current%60 == 0 || row.Minute != current/60*60) {
				model := row.ModelName
				if row.GroupID == 0 {
					model = ""
				}
				b.gap(row.GroupID, model, row.Minute, row.Minute+60)
			}
			cursor = row.ID
		}
		if len(rows) < availabilityPageSize {
			break
		}
	}
	for gap := range edgeGaps {
		b.gap(gap.group, gap.model, gap.at, gap.at+1)
	}
	cursorID := ""
	for {
		var rows []availabilityRequest
		if err := tx.Where("id > ? AND outcome = ? AND started_at < ? AND started_at > ? AND group_id IN ?", cursorID, availabilityPending, end-availabilityPendingSeconds, start-2*availabilityPendingSeconds, ids).Order("id").Limit(availabilityPageSize).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			model := row.ModelName
			if row.GroupID == 0 {
				model = ""
			}
			gapEnd := row.StartedAt + 2*availabilityPendingSeconds
			if gapEnd > end {
				gapEnd = end
			}
			b.gap(row.GroupID, model, row.StartedAt+availabilityPendingSeconds, gapEnd)
			cursorID = row.ID
		}
		if len(rows) < availabilityPageSize {
			break
		}
	}
	cursorID = ""
	for {
		var rows []availabilityGap
		if err := tx.Where("id > ? AND start < ? AND ended_at > ?", cursorID, end, start).Order("id").Limit(availabilityPageSize).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			b.gap(0, "", row.Start, row.End)
			cursorID = row.ID
		}
		if len(rows) < availabilityPageSize {
			break
		}
	}
	return nil
}

var availabilityReadOptions = &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}
