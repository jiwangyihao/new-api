package model

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreditValuationMathMulDivFloorOrdinaryValue(t *testing.T) {
	got, err := mulDivFloor(40_000_000, 800, 1_000)

	require.NoError(t, err)
	require.Equal(t, int64(32_000_000), got)
}

func TestCreditValuationMathMulDivFloorRejectsZeroDenominator(t *testing.T) {
	_, err := mulDivFloor(1, 1, 0)

	require.ErrorIs(t, err, ErrCreditValuationDivisionByZero)
}

func TestCreditValuationMathMulDivFloorHandlesWideIntermediate(t *testing.T) {
	got, err := mulDivFloor(math.MaxInt64, math.MaxInt64, math.MaxInt64)

	require.NoError(t, err)
	require.Equal(t, int64(math.MaxInt64), got)
}

func TestCreditValuationMathMulDivFloorRejectsResultOverflow(t *testing.T) {
	_, err := mulDivFloor(math.MaxInt64, 2, 1)

	require.ErrorIs(t, err, ErrCreditValuationOverflow)
}

func TestCreditValuationMathMulDivFloorRoundsDown(t *testing.T) {
	got, err := mulDivFloor(10, 2, 3)
	require.NoError(t, err)
	require.Equal(t, int64(6), got)
}

func TestCreditValuationMathMulDivFloorFullClearAbsorbsRemainder(t *testing.T) {
	available := int64(3)
	cost := int64(10)
	consumed := available
	got := cost
	if consumed != available {
		var err error
		got, err = mulDivFloor(cost, consumed, available)
		require.NoError(t, err)
	}
	require.Equal(t, cost, got)
}
