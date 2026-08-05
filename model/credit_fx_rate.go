package model

import (
	"errors"
	"math"
	"strings"
)

const CreditFXDirectionUSDtoCNY = "USD_TO_CNY"

var ErrCreditFXRateInvalid = errors.New("credit_fx_rate_invalid")

type CreditFXRateSnapshotInput struct {
	SourceCurrency    string
	ValuationCurrency string
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

func ParseCreditFXRateSnapshot(input CreditFXRateSnapshotInput) (CreditFXRateSnapshot, error) {
	sourceCurrency, err := NormalizeCreditValuationCurrency(input.SourceCurrency)
	if err != nil {
		return CreditFXRateSnapshot{}, ErrCreditFXRateInvalid
	}
	valuationCurrency, err := NormalizeCreditValuationCurrency(input.ValuationCurrency)
	if err != nil {
		return CreditFXRateSnapshot{}, ErrCreditFXRateInvalid
	}
	if sourceCurrency != "USD" || valuationCurrency != "CNY" || input.RateText == nil || input.CapturedAt <= 0 {
		return CreditFXRateSnapshot{}, ErrCreditFXRateInvalid
	}

	numerator, denominator, err := parsePositiveDecimalRatio(*input.RateText)
	if err != nil {
		return CreditFXRateSnapshot{}, err
	}
	return CreditFXRateSnapshot{
		SourceCurrency:    sourceCurrency,
		ValuationCurrency: valuationCurrency,
		Numerator:         numerator,
		Denominator:       denominator,
		CapturedAt:        input.CapturedAt,
		Direction:         CreditFXDirectionUSDtoCNY,
	}, nil
}

func parsePositiveDecimalRatio(text string) (int64, int64, error) {
	if text == "" || strings.TrimSpace(text) != text {
		return 0, 0, ErrCreditFXRateInvalid
	}

	dot := strings.IndexByte(text, '.')
	if dot == 0 || dot == len(text)-1 || (dot >= 0 && strings.IndexByte(text[dot+1:], '.') >= 0) {
		return 0, 0, ErrCreditFXRateInvalid
	}
	whole := text
	fraction := ""
	if dot >= 0 {
		whole = text[:dot]
		fraction = text[dot+1:]
	}
	if len(fraction) > 18 {
		return 0, 0, ErrCreditFXRateInvalid
	}
	fraction = strings.TrimRight(fraction, "0")

	numerator := uint64(0)
	for _, part := range []string{whole, fraction} {
		for index := 0; index < len(part); index++ {
			character := part[index]
			if character < '0' || character > '9' {
				return 0, 0, ErrCreditFXRateInvalid
			}
			digit := uint64(character - '0')
			if numerator > (uint64(math.MaxInt64)-digit)/10 {
				return 0, 0, ErrCreditFXRateInvalid
			}
			numerator = numerator*10 + digit
		}
	}
	if numerator == 0 {
		return 0, 0, ErrCreditFXRateInvalid
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
