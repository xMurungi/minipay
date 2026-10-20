package money

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrInvalidAmount = errors.New("invalid amount")

// ParseKES turns "1500", "1500.5" or "1500.50" into minor units (150050).
func ParseKES(s string) (int64, error) {
	s = strings.TrimSpace(s)
	whole, frac, _ := strings.Cut(s, ".")
	if whole == "" || len(frac) > 2 {
		return 0, ErrInvalidAmount
	}
	for _, r := range whole + frac {
		if r < '0' || r > '9' {
			return 0, ErrInvalidAmount
		}
	}
	for len(frac) < 2 {
		frac += "0"
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, ErrInvalidAmount
	}
	f, _ := strconv.ParseInt(frac, 10, 64)
	return w*100 + f, nil
}

func FormatKES(minor int64) string {
	return fmt.Sprintf("%d.%02d", minor/100, minor%100)
}
