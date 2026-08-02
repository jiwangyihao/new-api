package model

import (
	"math"
	"strconv"
	"strings"
)

const amountMicrosPerUnit int64 = 1_000_000

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
