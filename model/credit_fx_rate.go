package model

import (
	"errors"
	"math"
	"strings"
)

const (
	CreditFXDirectionIdentity = "IDENTITY"
	CreditFXDirectionUSDtoCNY = "USD_TO_CNY"
	CreditFXDirectionCNYtoUSD = "CNY_TO_USD"
)

var (
	ErrCreditFXRateInvalid         = errors.New("credit_fx_rate_invalid")
	ErrCreditFXRateMissing         = errors.New("credit_fx_rate_missing")
	ErrCreditFXRateEmpty           = errors.New("credit_fx_rate_empty")
	ErrCreditFXInvalidDecimal      = errors.New("credit_fx_invalid_decimal")
	ErrCreditFXPrecisionExceeded   = errors.New("credit_fx_precision_exceeded")
	ErrCreditFXNonPositive         = errors.New("credit_fx_non_positive")
	ErrCreditFXUnsupportedCurrency = errors.New("credit_fx_unsupported_currency")
	ErrCreditFXDirectionMismatch   = errors.New("credit_fx_direction_mismatch")
	ErrCreditFXOverflow            = errors.New("credit_fx_overflow")
)

type CreditFXRateSnapshotInput struct {
	SourceCurrency    string
	ValuationCurrency string
	Direction         string
	RateText          *string
	CapturedAt        int64
}

type CreditFXRateSnapshot struct {
	SourceCurrency    string `json:"source_currency"`
	ValuationCurrency string `json:"valuation_currency"`
	Numerator         int64  `json:"numerator,string"`
	Denominator       int64  `json:"denominator,string"`
	CapturedAt        int64  `json:"captured_at"`
	Direction         string `json:"direction"`
}

func (snapshot CreditFXRateSnapshot) ConvertMicros(amountMicros int64) (int64, error) {
	if amountMicros < 0 || snapshot.Numerator <= 0 || snapshot.Denominator <= 0 {
		return 0, ErrCreditFXRateInvalid
	}
	converted, err := mulDivFloor(amountMicros, snapshot.Numerator, snapshot.Denominator)
	if err == nil {
		return converted, nil
	}
	if errors.Is(err, ErrCreditValuationOverflow) {
		return 0, ErrCreditFXOverflow
	}
	return 0, ErrCreditFXRateInvalid
}

func ParseCreditFXRateSnapshot(input CreditFXRateSnapshotInput) (CreditFXRateSnapshot, error) {
	sourceCurrency, err := NormalizeCreditValuationCurrency(input.SourceCurrency)
	if err != nil {
		return CreditFXRateSnapshot{}, ErrCreditFXUnsupportedCurrency
	}
	valuationCurrency, err := NormalizeCreditValuationCurrency(input.ValuationCurrency)
	if err != nil {
		return CreditFXRateSnapshot{}, ErrCreditFXUnsupportedCurrency
	}
	if input.CapturedAt <= 0 {
		return CreditFXRateSnapshot{}, ErrCreditFXRateInvalid
	}

	direction := creditFXDirection(sourceCurrency, valuationCurrency)
	if input.Direction != "" && input.Direction != direction {
		return CreditFXRateSnapshot{}, ErrCreditFXDirectionMismatch
	}
	numerator, denominator := int64(1), int64(1)
	if direction != CreditFXDirectionIdentity {
		if input.RateText == nil {
			return CreditFXRateSnapshot{}, ErrCreditFXRateMissing
		}
		numerator, denominator, err = parsePositiveDecimalRatio(*input.RateText)
		if err != nil {
			return CreditFXRateSnapshot{}, err
		}
		if direction == CreditFXDirectionCNYtoUSD {
			numerator, denominator = denominator, numerator
		}
	}
	return CreditFXRateSnapshot{
		SourceCurrency:    sourceCurrency,
		ValuationCurrency: valuationCurrency,
		Numerator:         numerator,
		Denominator:       denominator,
		CapturedAt:        input.CapturedAt,
		Direction:         direction,
	}, nil
}

func creditFXDirection(sourceCurrency string, valuationCurrency string) string {
	if sourceCurrency == valuationCurrency {
		return CreditFXDirectionIdentity
	}
	if sourceCurrency == "USD" {
		return CreditFXDirectionUSDtoCNY
	}
	return CreditFXDirectionCNYtoUSD
}

func parsePositiveDecimalRatio(text string) (int64, int64, error) {
	if text == "" || strings.TrimSpace(text) == "" {
		return 0, 0, ErrCreditFXRateEmpty
	}
	if strings.TrimSpace(text) != text {
		return 0, 0, ErrCreditFXInvalidDecimal
	}

	negative := strings.HasPrefix(text, "-")
	unsignedText := text
	if negative {
		unsignedText = text[1:]
	}
	if unsignedText == "" {
		return 0, 0, ErrCreditFXInvalidDecimal
	}

	dot := strings.IndexByte(unsignedText, '.')
	if dot == 0 || dot == len(unsignedText)-1 || (dot >= 0 && strings.IndexByte(unsignedText[dot+1:], '.') >= 0) {
		return 0, 0, ErrCreditFXInvalidDecimal
	}
	whole := unsignedText
	fraction := ""
	if dot >= 0 {
		whole = unsignedText[:dot]
		fraction = unsignedText[dot+1:]
	}
	if len(fraction) > 18 {
		return 0, 0, ErrCreditFXPrecisionExceeded
	}
	for _, part := range []string{whole, fraction} {
		for index := range len(part) {
			character := part[index]
			if character < '0' || character > '9' {
				return 0, 0, ErrCreditFXInvalidDecimal
			}
		}
	}
	fraction = strings.TrimRight(fraction, "0")

	numerator := uint64(0)
	for _, part := range []string{whole, fraction} {
		for index := range len(part) {
			digit := uint64(part[index] - '0')
			if numerator > (uint64(math.MaxInt64)-digit)/10 {
				return 0, 0, ErrCreditFXRateInvalid
			}
			numerator = numerator*10 + digit
		}
	}
	if negative || numerator == 0 {
		return 0, 0, ErrCreditFXNonPositive
	}

	denominator := int64(1)
	for range len(fraction) {
		denominator *= 10
	}
	divisor := greatestCommonDivisor(int64(numerator), denominator)
	return int64(numerator) / divisor, denominator / divisor, nil
}

func greatestCommonDivisor(left int64, right int64) int64 {
	for right != 0 {
		left, right = right, left%right
	}
	return left
}
