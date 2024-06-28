package main

import (
	"fmt"
	"math"
	"strings"
)

const (
	shortK     float64 = 1000
	shortMil           = shortK * 1000
	shortBil           = shortMil * 1000
	shortTri           = shortBil * 1000
	shortQuadr         = shortTri * 1000
	shortQuint         = shortQuadr * 1000
	shortSext          = shortQuint * 1000
	shortSept          = shortSext * 1000
)

type rangeStep struct {
	max    float64
	suffix string
	base   float64
}

// shortFormatter returns the value trimming the value on
// high numbers adding a suffix.
// supports: K, Mil, Bil, tri, Quadr, Quint, Sext, Sept.
// Examples:
//   - 1000 = 1 k
//   - 2000000 = 2 Mil
var shortFormatter = newRangedFormatter([]rangeStep{
	{max: shortK, base: 1, suffix: ""},
	{max: shortMil, base: shortK, suffix: " K"},
	{max: shortBil, base: shortMil, suffix: " Mil"},
	{max: shortTri, base: shortBil, suffix: " Bil"},
	{max: shortQuadr, base: shortTri, suffix: " Tri"},
	{max: shortQuint, base: shortQuadr, suffix: " Quadr"},
	{max: shortSext, base: shortQuint, suffix: " Quint"},
	{max: shortSept, base: shortSext, suffix: " Sext"},
	{base: shortSept, suffix: " Sept"},
})

// noneFormatter returns the value as it is
// in a float representation with decimal trim.
func noneFormatter(value float64, decimals int) string {
	f := suffixDecimalFormat(decimals, "")
	return fmt.Sprintf(f, value)
}

// Formatter knows how to interact with different values
// to give then a format and a representation.
// They are based on units that can do the conversion to
// other units and represent them in the returning string.
type Formatter func(value float64, decimals int) string

func suffixDecimalFormat(decimals int, suffix string) string {
	suffix = strings.ReplaceAll(suffix, "%", "%%") // Safe `%` character for fmt.
	return fmt.Sprintf("%%.%df%s", decimals, suffix)
}

// newRangeFormatter returns a formatter based on a stepped
// range.
func newRangedFormatter(r []rangeStep) Formatter {
	return func(value float64, decimals int) string {
		// Check if the duration is less than 0.
		prefix := ""
		if value < 0 {
			prefix = "-"
			value = math.Abs(value)
		}

		step := r[0]
		for _, s := range r {
			if value < step.max {
				break
			}
			step = s
		}

		dFmt := prefix + suffixDecimalFormat(decimals, step.suffix)
		return fmt.Sprintf(dFmt, value/step.base)
	}
}
