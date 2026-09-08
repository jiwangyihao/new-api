package model

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var availabilityCatalogCache = struct {
	sync.Mutex
	db         *gorm.DB
	at         time.Time
	catalog    *AvailabilityCatalog
	generation uint64
}{}

var availabilityCatalogBuild = make(chan struct{}, 1)

// InvalidateAvailabilityCatalog invalidates future snapshots without mutating
// snapshots already held by requests. Call after channel or membership changes.
func InvalidateAvailabilityCatalog() {
	availabilityCatalogCache.Lock()
	availabilityCatalogCache.catalog = nil
	availabilityCatalogCache.generation++
	availabilityCatalogCache.Unlock()
}

// LoadAvailabilityCatalog uses the subscription display rule without exposing
// upstream channel names. Deleted groups are excluded by GORM's normal scope.
func LoadAvailabilityCatalog(ctx context.Context) (*AvailabilityCatalog, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	availabilityCatalogCache.Lock()
	if availabilityCatalogCache.db == DB && availabilityCatalogCache.catalog != nil && time.Since(availabilityCatalogCache.at) < 5*time.Second {
		catalog := availabilityCatalogCache.catalog
		availabilityCatalogCache.Unlock()
		return catalog, nil
	}
	availabilityCatalogCache.Unlock()
	select {
	case availabilityCatalogBuild <- struct{}{}:
		defer func() { <-availabilityCatalogBuild }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	availabilityCatalogCache.Lock()
	if availabilityCatalogCache.db == DB && availabilityCatalogCache.catalog != nil && time.Since(availabilityCatalogCache.at) < 5*time.Second {
		catalog := availabilityCatalogCache.catalog
		availabilityCatalogCache.Unlock()
		return catalog, nil
	}
	generation := availabilityCatalogCache.generation
	availabilityCatalogCache.Unlock()
	catalog, err := loadAvailabilityCatalog(ctx)
	if err != nil {
		return nil, err
	}
	availabilityCatalogCache.Lock()
	if generation == availabilityCatalogCache.generation {
		availabilityCatalogCache.db, availabilityCatalogCache.at, availabilityCatalogCache.catalog = DB, time.Now(), catalog
	}
	availabilityCatalogCache.Unlock()
	return catalog, nil
}

// Reports bypass the short request-path cache so current visibility is enforced
// on every read, even when their sanitized statistics can be reused.
func loadAvailabilityCatalog(ctx context.Context) (*AvailabilityCatalog, error) {
	if DB == nil {
		return nil, fmt.Errorf("availability catalog database is nil")
	}
	var groups []*ChannelGroup
	var members []ChannelGroupChannel
	var channels []struct {
		Id                     int
		Type                   int
		Status                 int
		Models                 string
		TokenBillingMultiplier float64
		CreditBillingMode      string
		FixedRequestCredits    int64
	}
	readSnapshot := func(tx *gorm.DB) error {
		if err := tx.Order("id ASC").Find(&groups).Error; err != nil {
			return err
		}
		if err := tx.Find(&members).Error; err != nil {
			return err
		}
		return tx.Model(&Channel{}).Select("id", "type", "status", "models", "token_billing_multiplier", "credit_billing_mode", "fixed_request_credits").Order("id ASC").Find(&channels).Error
	}
	var err error
	if DB.Dialector.Name() == "sqlite" {
		err = DB.WithContext(ctx).Transaction(readSnapshot)
	} else {
		err = DB.WithContext(ctx).Transaction(readSnapshot, availabilityReadOptions)
	}
	if err != nil {
		return nil, err
	}
	assignments := assignChannelNonDefaultGroups(groups, members)
	rows := make([]channelCreditBillingRow, 0, len(channels))
	for _, channel := range channels {
		if channel.Status == common.ChannelStatusEnabled {
			rows = append(rows, channelCreditBillingRow{Id: channel.Id, Type: channel.Type, TokenBillingMultiplier: channel.TokenBillingMultiplier, CreditBillingMode: channel.CreditBillingMode, FixedRequestCredits: channel.FixedRequestCredits})
		}
	}
	visible := make(map[int]bool)
	for _, g := range buildChannelCreditBillingGroups(rows, assignments) {
		visible[g.ChannelType] = true
	}
	catalog := &AvailabilityCatalog{Groups: []AvailabilityCatalogGroup{}, ChannelGroups: map[int]int{}, NamedGroups: map[string][]int{}, ChannelModels: map[int][]string{}, ChannelNames: map[int][]string{}}
	byID := make(map[int]*ChannelGroup, len(groups))
	defaultID, defaultExplicit := 0, false
	for _, g := range groups {
		byID[g.Id] = g
		if g.IsDefault() {
			defaultID = g.Id
		}
	}
	for _, m := range members {
		g := byID[m.ChannelGroupId]
		if g == nil {
			continue
		}
		if g.Id == defaultID {
			defaultExplicit = true
		}
		catalog.NamedGroups[g.Name] = append(catalog.NamedGroups[g.Name], m.ChannelId)
		catalog.ChannelNames[m.ChannelId] = append(catalog.ChannelNames[m.ChannelId], g.Name)
	}
	modelSets := make(map[int]map[string]bool)
	for _, ch := range channels {
		models := availabilityModelNames(ch.Models)
		catalog.ChannelModels[ch.Id] = models
		if !defaultExplicit {
			catalog.NamedGroups[DefaultChannelGroupName] = append(catalog.NamedGroups[DefaultChannelGroupName], ch.Id)
			catalog.ChannelNames[ch.Id] = append(catalog.ChannelNames[ch.Id], DefaultChannelGroupName)
		}
		assignment, ok := assignments[ch.Id]
		if !ok {
			continue
		}
		catalog.ChannelGroups[ch.Id] = assignment.groupId
		if ch.Status != common.ChannelStatusEnabled || !visible[assignment.groupId] {
			continue
		}
		if modelSets[assignment.groupId] == nil {
			modelSets[assignment.groupId] = map[string]bool{}
		}
		for _, name := range models {
			modelSets[assignment.groupId][name] = true
		}
	}
	for _, g := range groups {
		if g.IsDefault() || !visible[g.Id] {
			continue
		}
		models := make([]string, 0, len(modelSets[g.Id]))
		for name := range modelSets[g.Id] {
			models = append(models, name)
		}
		sort.Strings(models)
		catalog.Groups = append(catalog.Groups, AvailabilityCatalogGroup{ID: g.Id, Name: g.Name, Description: g.Description, Models: models})
	}
	return catalog, nil
}

func availabilityModelNames(value string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, name := range strings.Split(value, ",") {
		name = strings.TrimSpace(name)
		if name != "" && !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
}
