package model

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type historicalCreditPriceFacts struct {
	PriceMicros int64
	PlanCredit  int64
	Currency    string
}

type historicalCreditPlanPrice struct {
	Facts     historicalCreditPriceFacts
	Ambiguous bool
}

type historicalCreditSourceIndex struct {
	Orders              map[int]SubscriptionOrder
	ConversionsByLedger map[int]SubscriptionConversion
	Redemptions         map[int]Redemption
	AdjustmentsByLedger map[int]CreditBalanceAdjustment
	PlanPrices          map[int]historicalCreditPlanPrice
}

func loadHistoricalCreditSourceIndex(db *gorm.DB) (historicalCreditSourceIndex, error) {
	index := historicalCreditSourceIndex{
		Orders:              make(map[int]SubscriptionOrder),
		ConversionsByLedger: make(map[int]SubscriptionConversion),
		Redemptions:         make(map[int]Redemption),
		AdjustmentsByLedger: make(map[int]CreditBalanceAdjustment),
		PlanPrices:          make(map[int]historicalCreditPlanPrice),
	}
	if db.Migrator().HasTable(&SubscriptionOrder{}) {
		var orders []SubscriptionOrder
		if err := db.Where("status = ?", common.TopUpStatusSuccess).Order("id ASC").Find(&orders).Error; err != nil {
			return index, err
		}
		for _, order := range orders {
			index.Orders[order.Id] = order
			planID, facts, ok := historicalEntitlementPriceFacts(order.EntitlementSnapshot)
			if ok && planID == order.PlanId {
				addHistoricalPlanPrice(index.PlanPrices, planID, facts)
			}
		}
	}
	if db.Migrator().HasTable(&Redemption{}) {
		var redemptions []Redemption
		if err := db.Where("redeemed_time > 0 OR fulfillment_subscription_id > 0").Order("id ASC").Find(&redemptions).Error; err != nil {
			return index, err
		}
		for _, redemption := range redemptions {
			index.Redemptions[redemption.Id] = redemption
			planID, facts, ok := historicalRedemptionPriceFacts(redemption.FulfillmentSnapshot)
			if ok && planID == redemption.PlanId {
				addHistoricalPlanPrice(index.PlanPrices, planID, facts)
			}
		}
	}
	if db.Migrator().HasTable(&SubscriptionConversion{}) {
		var conversions []SubscriptionConversion
		if err := db.Order("id ASC").Find(&conversions).Error; err != nil {
			return index, err
		}
		for _, conversion := range conversions {
			if conversion.LedgerId > 0 {
				index.ConversionsByLedger[conversion.LedgerId] = conversion
			}
			if facts, ok := historicalConversionStoredPriceFacts(conversion); ok {
				addHistoricalPlanPrice(index.PlanPrices, conversion.SourcePlanId, facts)
			}
		}
	}
	if db.Migrator().HasTable(&CreditBalanceAdjustment{}) {
		var adjustments []CreditBalanceAdjustment
		if err := db.Where("operation = ?", CreditBalanceAdjustmentIncrease).Order("id ASC").Find(&adjustments).Error; err != nil {
			return index, err
		}
		for _, adjustment := range adjustments {
			if adjustment.LedgerId > 0 {
				index.AdjustmentsByLedger[adjustment.LedgerId] = adjustment
			}
		}
	}
	return index, nil
}

func addHistoricalPlanPrice(catalog map[int]historicalCreditPlanPrice, planID int, facts historicalCreditPriceFacts) {
	if planID <= 0 || !validHistoricalCreditPriceFacts(facts) {
		return
	}
	entry, exists := catalog[planID]
	if !exists {
		catalog[planID] = historicalCreditPlanPrice{Facts: facts}
		return
	}
	if entry.Facts != facts {
		entry.Ambiguous = true
		catalog[planID] = entry
	}
}

func validHistoricalCreditPriceFacts(facts historicalCreditPriceFacts) bool {
	if facts.PriceMicros <= 0 || facts.PlanCredit <= 0 {
		return false
	}
	currency, err := NormalizeCreditValuationCurrency(facts.Currency)
	return err == nil && currency == facts.Currency
}

func historicalPlanPriceFacts(catalog map[int]historicalCreditPlanPrice, planID int) (historicalCreditPriceFacts, bool) {
	entry, ok := catalog[planID]
	return entry.Facts, ok && !entry.Ambiguous && validHistoricalCreditPriceFacts(entry.Facts)
}

func historicalEntitlementPriceFacts(payload string) (int, historicalCreditPriceFacts, bool) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return 0, historicalCreditPriceFacts{}, false
	}
	var snapshot SubscriptionEntitlementSnapshot
	if err := common.UnmarshalJsonStr(payload, &snapshot); err != nil {
		return 0, historicalCreditPriceFacts{}, false
	}
	priceMicros := int64(0)
	if snapshot.ListPriceMicros != nil {
		priceMicros = *snapshot.ListPriceMicros
	}
	if priceMicros <= 0 {
		var raw struct {
			PriceAmount json.RawMessage `json:"price_amount"`
		}
		if err := common.UnmarshalJsonStr(payload, &raw); err != nil {
			return 0, historicalCreditPriceFacts{}, false
		}
		parsed, err := ParseDecimalAmountMicros(common.JsonRawMessageToString(raw.PriceAmount))
		if err != nil {
			return 0, historicalCreditPriceFacts{}, false
		}
		priceMicros = parsed
	}
	currency := strings.ToUpper(strings.TrimSpace(snapshot.ListPriceCurrency))
	if currency == "" {
		currency = strings.ToUpper(strings.TrimSpace(snapshot.Currency))
	}
	facts := historicalCreditPriceFacts{PriceMicros: priceMicros, PlanCredit: snapshot.MonthlyTokenLimit, Currency: currency}
	return snapshot.PlanID, facts, validHistoricalCreditPriceFacts(facts)
}

func historicalRedemptionPriceFacts(payload string) (int, historicalCreditPriceFacts, bool) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return 0, historicalCreditPriceFacts{}, false
	}
	var raw struct {
		Entitlement json.RawMessage `json:"entitlement"`
	}
	if err := common.UnmarshalJsonStr(payload, &raw); err != nil || len(raw.Entitlement) == 0 {
		return 0, historicalCreditPriceFacts{}, false
	}
	return historicalEntitlementPriceFacts(string(raw.Entitlement))
}

func historicalConversionStoredPriceFacts(conversion SubscriptionConversion) (historicalCreditPriceFacts, bool) {
	currency := strings.ToUpper(strings.TrimSpace(conversion.FxSourceCurrency))
	if currency == "" {
		currency = strings.ToUpper(strings.TrimSpace(conversion.ValuationCurrency))
	}
	facts := historicalCreditPriceFacts{
		PriceMicros: conversion.ValuationSourcePriceMicros,
		PlanCredit:  conversion.ValuationCreditBasis,
		Currency:    currency,
	}
	return facts, validHistoricalCreditPriceFacts(facts)
}

func recoverHistoricalCreditLedger(row CreditBalanceLedger, index historicalCreditSourceIndex) CreditBalanceLedger {
	if row.GrossCredit <= 0 {
		return row
	}
	if netCredit, ok := historicalCreditNetCredit(row); ok {
		row.NetCredit = netCredit
	}
	if row.NetCredit <= 0 {
		return row
	}
	if row.SourcePriceMicros > 0 && row.SourcePlanCredit > 0 && strings.TrimSpace(row.FxSourceCurrency) != "" {
		return row
	}
	var facts historicalCreditPriceFacts
	var ok bool
	switch strings.TrimSpace(row.SourceType) {
	case CreditBalanceLedgerSourceSubscriptionOrder:
		order, found := index.Orders[row.SourceId]
		if found && order.UserId == row.UserId && order.FulfilledSubscriptionID == row.UserSubscriptionId && (order.CreditGrantAmount == 0 || order.CreditGrantAmount == row.GrossCredit) {
			planID, recovered, valid := historicalEntitlementPriceFacts(order.EntitlementSnapshot)
			if valid && planID == order.PlanId {
				facts, ok = recovered, true
				if row.SourceKey == "" {
					row.SourceKey = "subscription_order:" + strconv.Itoa(order.Id)
				}
			}
		}
	case CreditBalanceLedgerSourceSubscriptionConversion:
		conversion, found := index.ConversionsByLedger[row.Id]
		if found && conversion.UserId == row.UserId && conversion.TargetSubscriptionId == row.UserSubscriptionId && conversion.GrossCredit == row.GrossCredit {
			facts, ok = historicalConversionStoredPriceFacts(conversion)
			if !ok {
				facts, ok = historicalPlanPriceFacts(index.PlanPrices, conversion.SourcePlanId)
				if ok && conversion.CreditBasis > 0 && facts.PlanCredit != conversion.CreditBasis {
					ok = false
				}
			}
			if ok && row.SourceKey == "" {
				row.SourceKey = "subscription_conversion:" + strconv.Itoa(conversion.SourceSubscriptionId)
			}
		}
	case CreditBalanceLedgerSourceRedemption:
		redemption, found := index.Redemptions[row.SourceId]
		if found && redemption.UsedUserId == row.UserId && redemption.FulfillmentSubscriptionId == row.UserSubscriptionId {
			planID, recovered, valid := historicalRedemptionPriceFacts(redemption.FulfillmentSnapshot)
			if valid && planID == redemption.PlanId {
				facts, ok = recovered, true
				if row.SourceKey == "" {
					row.SourceKey = "redemption:" + strconv.Itoa(redemption.Id)
				}
			}
		}
	case CreditBalanceLedgerSourceAdminAdjustment:
		adjustment, found := index.AdjustmentsByLedger[row.Id]
		if found && adjustment.UserId == row.UserId && adjustment.Amount == row.GrossCredit {
			var source creditBalanceAdjustmentSourceSnapshot
			if common.UnmarshalJsonStr(row.SourceSnapshot, &source) == nil && source.Valuation.SourcePriceMicros != nil {
				facts = historicalCreditPriceFacts{
					PriceMicros: *source.Valuation.SourcePriceMicros,
					PlanCredit:  source.Valuation.SourcePlanCredit,
					Currency:    strings.ToUpper(strings.TrimSpace(source.Valuation.SourceCurrency)),
				}
				ok = validHistoricalCreditPriceFacts(facts)
			}
			if !ok {
				facts, ok = historicalPlanPriceFacts(index.PlanPrices, adjustment.PlanId)
			}
			if ok && row.SourceKey == "" {
				row.SourceKey = "admin_adjustment:" + strconv.Itoa(adjustment.Id)
			}
		}
	}
	if !ok {
		return row
	}
	row.SourcePriceMicros = facts.PriceMicros
	row.SourcePlanCredit = facts.PlanCredit
	row.FxSourceCurrency = facts.Currency
	return row
}

func historicalCreditNetCredit(row CreditBalanceLedger) (int64, bool) {
	if row.NetCredit > 0 {
		return row.NetCredit, true
	}
	delta, ok := checkedSubInt64(row.AvailableCreditAfter, row.AvailableCreditBefore)
	if !ok || delta < 0 {
		return 0, false
	}
	expected, ok := checkedSubInt64(row.GrossCredit, row.DebtOffset)
	if !ok || expected < 0 || delta != expected {
		return 0, false
	}
	return delta, true
}
