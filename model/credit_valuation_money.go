package model

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const amountMicrosPerUnit int64 = 1_000_000

type SubscriptionPlanPriceInput struct {
	DisplayAmount         string
	DisplayAmountProvided bool
	AmountMicros          string
	AmountMicrosProvided  bool
}

type SubscriptionPlanPrice struct {
	DisplayAmount float64
	AmountMicros  *int64
}

func ParseDecimalAmountMicros(text string) (int64, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, ErrSubscriptionPlanPriceInvalid
	}
	if strings.HasPrefix(text, "-") {
		return 0, ErrSubscriptionPlanPriceNegative
	}
	if strings.HasPrefix(text, "+") {
		return 0, ErrSubscriptionPlanPriceInvalid
	}
	parts := strings.Split(text, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, ErrSubscriptionPlanPriceInvalid
	}
	for _, digit := range parts[0] {
		if digit < '0' || digit > '9' {
			return 0, ErrSubscriptionPlanPriceInvalid
		}
	}
	whole, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0, ErrCreditValuationOverflow
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(fraction) > 6 {
		return 0, ErrSubscriptionPlanPricePrecision
	}
	for _, digit := range fraction {
		if digit < '0' || digit > '9' {
			return 0, ErrSubscriptionPlanPriceInvalid
		}
	}
	fraction += strings.Repeat("0", 6-len(fraction))
	fractionMicros := uint64(0)
	if fraction != "" {
		fractionMicros, err = strconv.ParseUint(fraction, 10, 64)
		if err != nil {
			return 0, ErrSubscriptionPlanPriceInvalid
		}
	}
	max := uint64(math.MaxInt64)
	if whole > max/uint64(amountMicrosPerUnit) ||
		(whole == max/uint64(amountMicrosPerUnit) && fractionMicros > max%uint64(amountMicrosPerUnit)) {
		return 0, ErrCreditValuationOverflow
	}
	return int64(whole*uint64(amountMicrosPerUnit) + fractionMicros), nil
}

func NormalizeSubscriptionPlanPrice(input SubscriptionPlanPriceInput) (SubscriptionPlanPrice, error) {
	displayMicros := int64(0)
	if input.DisplayAmountProvided {
		parsed, err := ParseDecimalAmountMicros(input.DisplayAmount)
		if err != nil {
			return SubscriptionPlanPrice{}, err
		}
		displayMicros = parsed
	}

	exactMicros := int64(0)
	if input.AmountMicrosProvided {
		if input.AmountMicros == "" || strings.TrimSpace(input.AmountMicros) != input.AmountMicros {
			return SubscriptionPlanPrice{}, ErrSubscriptionPlanPriceInvalid
		}
		if strings.HasPrefix(input.AmountMicros, "-") {
			return SubscriptionPlanPrice{}, ErrSubscriptionPlanPriceNegative
		}
		parsed, err := strconv.ParseUint(input.AmountMicros, 10, 64)
		if err != nil || parsed > uint64(math.MaxInt64) {
			return SubscriptionPlanPrice{}, ErrCreditValuationOverflow
		}
		exactMicros = int64(parsed)
	}

	if !input.AmountMicrosProvided {
		if displayMicros > 0 {
			return SubscriptionPlanPrice{}, ErrSubscriptionPlanPriceRequired
		}
		return SubscriptionPlanPrice{}, nil
	}
	if input.DisplayAmountProvided && displayMicros != exactMicros {
		return SubscriptionPlanPrice{}, ErrSubscriptionPlanPriceMismatch
	}
	displayText := strconv.FormatInt(exactMicros/amountMicrosPerUnit, 10) + "." +
		fmt.Sprintf("%06d", exactMicros%amountMicrosPerUnit)
	displayAmount, err := strconv.ParseFloat(displayText, 64)
	if err != nil {
		return SubscriptionPlanPrice{}, ErrSubscriptionPlanPriceInvalid
	}
	return SubscriptionPlanPrice{DisplayAmount: displayAmount, AmountMicros: &exactMicros}, nil
}
