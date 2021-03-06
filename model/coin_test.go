package model

import (
	"errors"
	"math"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	// LowerBoundRat - the lower bound of Rat
	LowerBoundRat = NewDecFromRat(1, Decimals)
	// UpperBoundRat - the upper bound of Rat
	UpperBoundRat = sdk.NewDec(math.MaxInt64 / Decimals)
)

const (
	Decimals = 100000
)

func TestCoinToLNO(t *testing.T) {
	testCases := map[string]struct {
		inputLino     string
		expectCoinStr string
		expectLino    string
	}{
		"lino without 0, coin with 5 zeros": {
			inputLino:     "123",
			expectCoinStr: "12300000",
			expectLino:    "123",
		},
		"lino with some 0, coin with some zeros": {
			inputLino:     "100.00023",
			expectCoinStr: "10000023",
			expectLino:    "100.00023",
		},
		"lino with one 0, coin with more than 5 zeros": {
			inputLino:     "1230",
			expectCoinStr: "123000000",
			expectLino:    "1230",
		},
		"lino with one digit, coin with less than 5 zeros": {
			inputLino:     "12.3",
			expectCoinStr: "1230000",
			expectLino:    "12.3",
		},
		"lino with three digits, coin with less than 5 zeros": {
			inputLino:     "0.123",
			expectCoinStr: "12300",
			expectLino:    "0.123",
		},
		"lino with five digits, coin with no zero": {
			inputLino:     "0.00123",
			expectCoinStr: "123",
			expectLino:    "0.00123",
		},
		"lino with 3 zero, coin with no zero": {
			inputLino:     "100082.92819",
			expectCoinStr: "10008292819",
			expectLino:    "100082.92819",
		},
	}

	for testName, tc := range testCases {
		coin, err := LinoToCoin(tc.inputLino)
		if err != nil {
			t.Errorf("%s: failed to convert lino to coin, got err %v", testName, err)
		}

		if coin.Amount.String() != tc.expectCoinStr {
			t.Errorf("%s: diff coin amount, got %v, want %v", testName, coin.Amount.String(), tc.expectCoinStr)
		}

		got := coin.CoinToLNO()
		if got != tc.expectLino {
			t.Errorf("%s: diff lino, got %v, want %v", testName, got, tc.expectLino)
		}
	}
}

//
// helper function
//

// NewCoinFromInt64 - return int64 amount of Coin
func NewCoinFromInt64(amount int64) Coin {
	// return Coin{big.NewInt(amount)}
	return Coin{Int{I: sdk.NewInt(amount).BigInt()}}
}

// LinoToCoin - convert 1 LNO to 10^5 Coin
func LinoToCoin(lino string) (Coin, error) {
	rat, err := sdk.NewDecFromStr(lino)
	if err != nil {
		return NewCoinFromInt64(0), errors.New("Illegal LNO")
	}
	if rat.GT(UpperBoundRat) {
		return NewCoinFromInt64(0), errors.New("LNO overflow")
	}
	if rat.LT(LowerBoundRat) {
		return NewCoinFromInt64(0), errors.New("LNO can't be less than lower bound")
	}
	return DecToCoin(rat.Mul(sdk.NewDec(Decimals))), nil
}

// DecToCoin - convert sdk.Dec to LNO coin
// XXX(yumin): the unit of @p rat must be coin.
func DecToCoin(rat sdk.Dec) Coin {
	return Coin{Int{I: rat.RoundInt().BigInt()}}
}
