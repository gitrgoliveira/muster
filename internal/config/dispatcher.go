package config

import (
	"fmt"
	"strconv"
)

// ParseMaxConcurrent validates and returns the --max-concurrent-dispatches flag
// value (M4 scheduler capacity). Accepts any positive integer; empty string
// defaults to 4. Returns a typed error for non-positive or non-integer values
// so startup fails fast on misconfiguration (mirrors ParseDefaultVCS style).
func ParseMaxConcurrent(val string) (int, error) {
	if val == "" {
		return 4, nil
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("invalid --max-concurrent-dispatches value %q: must be a positive integer", val)
	}
	if n <= 0 {
		return 0, fmt.Errorf("invalid --max-concurrent-dispatches value %q: must be a positive integer (>0)", val)
	}
	return n, nil
}
