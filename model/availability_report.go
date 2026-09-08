package model

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type availabilityCacheKey struct {
	db        *gorm.DB
	catalog   *gorm.DB
	rangeName string
}
type availabilityCacheEntry struct {
	at      int64
	catalog string
	data    []byte
}

var availabilityReportCache = struct {
	sync.Mutex
	entries map[availabilityCacheKey]availabilityCacheEntry
}{entries: make(map[availabilityCacheKey]availabilityCacheEntry)}

// Serializes cache misses only; prevents simultaneous user refreshes from
// repeating the same historical scan. Cached responses remain immutable bytes.
var availabilityReportBuild = make(chan struct{}, 1)

func availabilityInvalidateReport(db *gorm.DB) {
	availabilityReportCache.Lock()
	defer availabilityReportCache.Unlock()
	for key := range availabilityReportCache.entries {
		if key.db == db {
			delete(availabilityReportCache.entries, key)
		}
	}
}

type availabilityTotals struct {
	success    int64
	failure    int64
	latencies  map[int64]int64
	last       int64
	incomplete bool
}

func (a *availabilityTotals) add(outcome string, count, latency, last int64) {
	if outcome == AvailabilityUnknown {
		a.incomplete = true
		return
	}
	if outcome == AvailabilitySuccess {
		a.success += count
		if latency >= 0 {
			if a.latencies == nil {
				a.latencies = map[int64]int64{}
			}
			a.latencies[latency] += count
		}
	} else if outcome == AvailabilityFailure {
		a.failure += count
	} else {
		return
	}
	if last > a.last {
		a.last = last
	}
}
func (a *availabilityTotals) metric(start, coverageStart, last int64) AvailabilityMetric {
	m := AvailabilityMetric{State: "unknown", Coverage: "complete", HasFailures: a.failure > 0}
	if last < a.last {
		last = a.last
	}
	if last > 0 {
		m.LastObservedAt = &last
	}
	count := a.success + a.failure
	if count > 0 {
		rate := float64(a.success) / float64(count) * 100
		m.SuccessRate = &rate
		m.LowSample = count < 20
		switch {
		case rate >= 90:
			m.State = "healthy"
		case rate >= 80:
			m.State = "degraded"
		default:
			m.State = "unhealthy"
		}
	}
	if a.incomplete || start < coverageStart {
		m.State = "incomplete"
		m.Coverage = "incomplete"
	}
	m.FirstResponseMs = availabilityMedian(a.latencies)
	return m
}
func availabilityMedian(histogram map[int64]int64) *float64 {
	if len(histogram) == 0 {
		return nil
	}
	values := make([]int64, 0, len(histogram))
	var total int64
	for value, count := range histogram {
		values = append(values, value)
		total += count
	}
	if total == 0 {
		return nil
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	left, right := (total-1)/2, total/2
	var seen int64
	var median float64
	for _, value := range values {
		previous := seen
		seen += histogram[value]
		if previous <= left && left < seen {
			median += float64(value) / 2
		}
		if previous <= right && right < seen {
			median += float64(value) / 2
		}
	}
	return &median
}

type availabilitySeries struct {
	current  availabilityTotals
	summary  availabilityTotals
	buckets  []availabilityTotals
	last     int64
	active   bool
	inWindow bool
}
type availabilityGroupSeries struct {
	group  availabilitySeries
	models map[string]*availabilitySeries
}
type availabilityReportBuilder struct {
	report *AvailabilityReport
	groups map[int]*availabilityGroupSeries
	starts []int64
	ends   []int64
}

func (b *availabilityReportBuilder) series(group int, model string) *availabilitySeries {
	g := b.groups[group]
	if g == nil {
		return nil
	}
	s := g.models[model]
	if s == nil {
		s = &availabilitySeries{buckets: make([]availabilityTotals, len(b.starts))}
		g.models[model] = s
	}
	return s
}
func (b *availabilityReportBuilder) addSeries(s *availabilitySeries, at int64, outcome string, count, latency, last int64) {
	if (outcome == AvailabilitySuccess || outcome == AvailabilityFailure) && last > s.last {
		s.last = last
	}
	if at >= b.report.WindowStart && at < b.report.WindowEnd {
		s.inWindow = true
		s.summary.add(outcome, count, latency, last)
		i := sort.Search(len(b.ends), func(i int) bool { return b.ends[i] > at })
		if i < len(s.buckets) {
			s.buckets[i].add(outcome, count, latency, last)
		}
	}
	if at >= b.report.CurrentStart && at < b.report.WindowEnd {
		s.current.add(outcome, count, latency, last)
	}
}
func (b *availabilityReportBuilder) add(group int, model string, at int64, outcome string, count, latency, last int64) {
	if group == 0 {
		b.gap(0, "", at, at+1)
		return
	}
	g := b.groups[group]
	if g == nil {
		return
	}
	b.addSeries(&g.group, at, outcome, count, latency, last)
	if model != "" {
		b.addSeries(b.series(group, model), at, outcome, count, latency, last)
	} else if outcome == AvailabilityUnknown {
		b.gap(group, "", at, at+1)
	}
}
func (b *availabilityReportBuilder) gap(group int, model string, start, end int64) {
	mark := func(s *availabilitySeries) {
		if start < b.report.WindowEnd && end > b.report.WindowStart {
			s.summary.incomplete = true
		}
		if start < b.report.WindowEnd && end > b.report.CurrentStart {
			s.current.incomplete = true
		}
		for i := range b.starts {
			if start < b.ends[i] && end > b.starts[i] {
				s.buckets[i].incomplete = true
			}
		}
	}
	for id, g := range b.groups {
		if group != 0 && group != id {
			continue
		}
		mark(&g.group)
		if model != "" {
			mark(b.series(id, model))
		} else {
			for _, s := range g.models {
				mark(s)
			}
		}
	}
}
func (b *availabilityReportBuilder) buckets(s *availabilitySeries) []AvailabilityBucket {
	result := make([]AvailabilityBucket, len(b.starts))
	for i := range result {
		result[i] = AvailabilityBucket{Start: b.starts[i], End: b.ends[i], AvailabilityMetric: s.buckets[i].metric(b.starts[i], b.report.CoverageStart, s.buckets[i].last)}
	}
	return result
}

// GetAvailabilityReport reads minute frequency tables, not the raw request
// history. Only the two partial minutes are read from request rows, preserving
// exact [start,end) boundaries and zero-millisecond timings. All scans page.
func GetAvailabilityReport(ctx context.Context, rangeName string, now time.Time) (*AvailabilityReport, error) {
	if rangeName == "" {
		rangeName = "24h"
	}
	var duration, bucket int64
	switch rangeName {
	case "1h":
		duration, bucket = 3600, 60
	case "24h":
		duration, bucket = 86400, 900
	case "7d":
		duration, bucket = 7*86400, 7200
	case "30d":
		duration, bucket = 30*86400, 21600
	default:
		return nil, fmt.Errorf("invalid availability range")
	}
	if LOG_DB == nil {
		return nil, fmt.Errorf("availability log database is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	catalog, err := loadAvailabilityCatalog(ctx)
	if err != nil {
		return nil, err
	}
	catalogJSON, err := common.Marshal(catalog.Groups)
	if err != nil {
		return nil, err
	}
	key := availabilityCacheKey{db: LOG_DB, catalog: DB, rangeName: rangeName}
	availabilityReportCache.Lock()
	cached, found := availabilityReportCache.entries[key]
	availabilityReportCache.Unlock()
	memoryGaps := availabilityMemoryGapSnapshot(LOG_DB)
	if found && now.Unix() >= cached.at && now.Unix()-cached.at < 60 && cached.catalog == string(catalogJSON) && len(memoryGaps) == 0 {
		var report AvailabilityReport
		if err := common.Unmarshal(cached.data, &report); err != nil {
			return nil, err
		}
		return &report, nil
	}
	select {
	case availabilityReportBuild <- struct{}{}:
		defer func() { <-availabilityReportBuild }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	availabilityReportCache.Lock()
	cached, found = availabilityReportCache.entries[key]
	availabilityReportCache.Unlock()
	memoryGaps = availabilityMemoryGapSnapshot(LOG_DB)
	if found && now.Unix() >= cached.at && now.Unix()-cached.at < 60 && cached.catalog == string(catalogJSON) && len(memoryGaps) == 0 {
		var report AvailabilityReport
		if err := common.Unmarshal(cached.data, &report); err != nil {
			return nil, err
		}
		return &report, nil
	}
	report := &AvailabilityReport{Range: rangeName, WindowStart: now.Unix() - duration, WindowEnd: now.Unix(), CurrentStart: now.Unix() - 900, GeneratedAt: now.Unix(), RefreshSeconds: 60, BucketSeconds: bucket, Groups: []AvailabilityGroup{}}
	b := &availabilityReportBuilder{report: report, groups: map[int]*availabilityGroupSeries{}}
	for start := report.WindowStart; start < report.WindowEnd; {
		end := (start/bucket + 1) * bucket
		if end > report.WindowEnd {
			end = report.WindowEnd
		}
		b.starts = append(b.starts, start)
		b.ends = append(b.ends, end)
		start = end
	}
	ids := []int{0}
	for _, group := range catalog.Groups {
		g := &availabilityGroupSeries{group: availabilitySeries{buckets: make([]availabilityTotals, len(b.starts))}, models: map[string]*availabilitySeries{}}
		b.groups[group.ID] = g
		ids = append(ids, group.ID)
		for _, name := range group.Models {
			b.series(group.ID, name).active = true
		}
	}
	// A repeatable read keeps boundary raw rows, histograms, and last-observed
	// values from describing different commits during concurrent completions.
	if LOG_DB.Dialector.Name() == "sqlite" {
		err = LOG_DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return b.load(tx, ids) })
	} else {
		err = LOG_DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return b.load(tx, ids) }, availabilityReadOptions)
	}
	if err != nil {
		return nil, err
	}
	for _, gap := range memoryGaps {
		b.gap(0, "", gap.Start, gap.End)
	}
	for _, group := range catalog.Groups {
		g := b.groups[group.ID]
		view := AvailabilityGroup{ID: group.ID, Name: group.Name, Description: group.Description,
			Current: g.group.current.metric(report.CurrentStart, report.CoverageStart, g.group.last), Summary: g.group.summary.metric(report.WindowStart, report.CoverageStart, g.group.last), Buckets: b.buckets(&g.group), Models: []AvailabilityModel{}}
		names := make([]string, 0, len(g.models))
		for name, s := range g.models {
			if s.active || s.inWindow {
				names = append(names, name)
			}
		}
		sort.Slice(names, func(i, j int) bool {
			a, z := g.models[names[i]], g.models[names[j]]
			if a.active != z.active {
				return a.active
			}
			return names[i] < names[j]
		})
		for _, name := range names {
			s := g.models[name]
			view.Models = append(view.Models, AvailabilityModel{Name: name, Active: s.active, Current: s.current.metric(report.CurrentStart, report.CoverageStart, s.last), Summary: s.summary.metric(report.WindowStart, report.CoverageStart, s.last), Buckets: b.buckets(s)})
		}
		report.Groups = append(report.Groups, view)
	}
	data, err := common.Marshal(report)
	if err != nil {
		return nil, err
	}
	availabilityReportCache.Lock()
	if len(availabilityReportCache.entries) >= 64 {
		clear(availabilityReportCache.entries)
	}
	availabilityReportCache.entries[key] = availabilityCacheEntry{at: now.Unix(), catalog: string(catalogJSON), data: data}
	availabilityReportCache.Unlock()
	return report, nil
}
