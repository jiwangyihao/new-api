package model

import "math/bits"

func mulDivFloor(a, b, denominator int64) (int64, error) {
	if a < 0 || b < 0 || denominator < 0 {
		return 0, ErrCreditValuationNegativeInput
	}
	if denominator == 0 {
		return 0, ErrCreditValuationDivisionByZero
	}
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	if hi >= uint64(denominator) {
		return 0, ErrCreditValuationOverflow
	}
	quotient, _ := bits.Div64(hi, lo, uint64(denominator))
	if quotient > uint64(^uint64(0)>>1) {
		return 0, ErrCreditValuationOverflow
	}
	return int64(quotient), nil
}

func prorateFloor(value, share, total int64) (int64, error) {
	if value < 0 || share < 0 || total < 0 || share > total {
		return 0, ErrCreditValuationNegativeInput
	}
	if total == 0 {
		return 0, ErrCreditValuationDivisionByZero
	}
	if share == total {
		return value, nil
	}
	return mulDivFloor(value, share, total)
}
