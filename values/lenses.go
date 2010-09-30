package values
import (
	"fmt"
	"os"
)

// Float2String returns a Lens which maps between float64
// and string. The given formats are used to effect the conversion;
// fmt.Sprintf(printf, x) is used to convert the float64 value x to string;
// fmt.Sscanf(s, scanf, &x) is used to convert the string s to the float64
// value x.
//
func Float2String(printf, scanf string) *Lens {
	// do early sanity check on format.
	s := fmt.Sprintf(printf, float64(0))
	var f float64
	_, err := fmt.Sscanf(s, scanf, &f)
	if err != nil || f != 0 {
		panic(fmt.Sprintf("non-reversible format %#v<->%#v (got %#v), err %v", printf, scanf, s, err))
	}
	return NewLens(
		func(f float64) (string, os.Error) {
			return fmt.Sprintf(printf, f), nil
		},
		func(s string) (float64, os.Error) {
			var f float64
			_, err := fmt.Sscanf(s, scanf, &f)
			return f, err
		},
	)
}

func Float2Int() *Lens {
	return NewLens(
		func(f float64) (int, os.Error) {
			return int(f + 0.5), nil
		},
		func(i int) (float64, os.Error) {
			return float64(i), nil
		},
	)
}

// UnitFloat2RangedFloat peforms a linear conversion between a float64
// value in [0, 1] and a float64 value in [lo, hi].
//
func UnitFloat2RangedFloat(lo, hi float64) *Lens {
	return NewLens(
		func(uf float64) (float64, os.Error) {
			if uf > 1 {
				return 0, os.NewError("value too high")
			}
			if uf < 0 {
				return 0, os.NewError("value too low")
			}
			return lo + (uf * (hi - lo)), nil
		},
		func(rf float64) (float64, os.Error) {
			if rf > hi {
				return 0, os.NewError("value too high")
			}
			if rf < lo {
				return 0, os.NewError("value too low")
			}
			return (rf - lo) / (hi - lo), nil
		},
	)
}

// FloatMultiply multiplies by x.
//
func FloatMultiply(x float64) *Lens {
	return NewLens(
		func(f float64) (float64, os.Error) {
			return f * x, nil
		},
		func(rf float64) (float64, os.Error) {
			return rf / x, nil
		},
	)
}

