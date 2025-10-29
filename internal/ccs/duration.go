package ccs

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

var durationPattern = regexp.MustCompile(`(?i)(\d+)([dhms])`)

// ParseRetentionInterval converts strings like "30d" or "12h" into a time.Duration.
func ParseRetentionInterval(input string) (time.Duration, error) {
	matches := durationPattern.FindAllStringSubmatch(input, -1)
	if len(matches) == 0 {
		return 0, fmt.Errorf("invalid duration format: %s", input)
	}
	total := time.Duration(0)
	for _, parts := range matches {
		value, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, fmt.Errorf("invalid duration number: %w", err)
		}
		switch parts[2] {
		case "d", "D":
			total += time.Duration(value) * 24 * time.Hour
		case "h", "H":
			total += time.Duration(value) * time.Hour
		case "m", "M":
			total += time.Duration(value) * time.Minute
		case "s", "S":
			total += time.Duration(value) * time.Second
		default:
			return 0, fmt.Errorf("unsupported duration unit: %s", parts[2])
		}
	}
	return total, nil
}
