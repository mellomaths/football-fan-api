// Package validate contains input validation helpers for HTTP query parameters.
package validate

import (
	"fmt"
	"time"
)

const dateLayout = "2006-01-02"

// DateRange holds validated UTC calendar bounds for querying matches.
// Matches use [FromInclusive, ToExclusive) on kickoff_utc.
type DateRange struct {
	FromInclusive time.Time
	ToExclusive   time.Time
}

// ParseRequiredDateRange parses from and to as YYYY-MM-DD (UTC), requires both,
// enforces from <= to and (to - from) <= 31 days (PostgreSQL date subtraction semantics).
func ParseRequiredDateRange(fromStr, toStr string) (DateRange, error) {
	var zero DateRange
	if fromStr == "" || toStr == "" {
		return zero, fmt.Errorf("query parameters \"from\" and \"to\" are required (YYYY-MM-DD)")
	}
	from, err := time.ParseInLocation(dateLayout, fromStr, time.UTC)
	if err != nil {
		return zero, fmt.Errorf("invalid \"from\" date: use YYYY-MM-DD")
	}
	to, err := time.ParseInLocation(dateLayout, toStr, time.UTC)
	if err != nil {
		return zero, fmt.Errorf("invalid \"to\" date: use YYYY-MM-DD")
	}
	if from.After(to) {
		return zero, fmt.Errorf("\"from\" must be on or before \"to\"")
	}
	days := int(to.Sub(from).Hours() / 24)
	if days > 31 {
		return zero, fmt.Errorf("date range must be at most 31 days between \"from\" and \"to\"")
	}
	// Exclusive end: first instant after the last day of `to`.
	toExclusive := to.AddDate(0, 0, 1)
	return DateRange{FromInclusive: from, ToExclusive: toExclusive}, nil
}
