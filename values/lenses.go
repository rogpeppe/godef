package values

import (
	"errors"
	"fmt"
)

// Float64ToString returns a Lens which transforms from float64
// to string. The given formats are used to effect the conversion;
// fmt.Sprintf(printf, x) is used to convert the float64 value x to string;
// fmt.Sscanf(s, scanf, &x) is used to scan the string s into the float64
// value x.
//
func Float64ToString(printf, scanf string) *Lens {
	// do early sanity check on format.
	s := fmt.Sprintf(printf, float64(0))
	var f float64
	_, err := fmt.Sscanf(s, scanf, &f)
	if err != nil || f != 0 {
		panic(fmt.Sprintf("non-reversible format %#v<->%#v (got %#v), err %v", printf, scanf, s, err))
	}
	return NewLens(
		func(f float64) (string, error) {
			return fmt.Sprintf(printf, f), nil
		},
		func(s string) (float64, error) {
			var f float64
			_, err := fmt.Sscanf(s, scanf, &f)
			return f, err
		},
	)
}

func round(f float64) int {
	if f < 0 {
		return int(f - 0.5)
	}
	return int(f + 0.5)
}

// Float64ToInt returns a Lens that transforms a float64
// value to the nearest int.
func Float64ToInt() *Lens {
	return NewLens(
		func(f float64) (int, error) {
			return round(f), nil
		},
		func(i int) (float64, error) {
			return float64(i), nil
		},
	)
}

// UnitFloat64ToRangedFloat64 returns a Lens that peforms a linear
// transformation from a float64
// value in [0, 1] to a float64 value in [lo, hi].
//
func UnitFloat64ToRangedFloat64(lo, hi float64) *Lens {
	return NewLens(
		func(uf float64) (float64, error) {
			if uf > 1 {
				return 0, errors.New("value too high")
			}
			if uf < 0 {
				return 0, errors.New("value too low")
			}
			return lo + (uf * (hi - lo)), nil
		},
		func(rf float64) (float64, error) {
			if rf > hi {
				return 0, errors.New("value too high")
			}
			if rf < lo {
				return 0, errors.New("value too low")
			}
			return (rf - lo) / (hi - lo), nil
		},
	)
}

// Float64Multiply returns a Lens that multiplies by x.
//
func Float64Multiply(x float64) *Lens {
	return NewLens(
		func(f float64) (float64, error) {
			return f * x, nil
		},
		func(rf float64) (float64, error) {
			return rf / x, nil
		},
	)
}
