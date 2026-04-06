// Package validate contains input validation helpers for HTTP query parameters.
package validate

import (
	"fmt"
	"time"
)

const maxTicketAnnouncementRange = 90 * 24 * time.Hour

// InstantRange holds validated UTC bounds for ticket announcement queries on scraped_at.
// Both ends are inclusive: scraped_at >= FromInclusive AND scraped_at <= ToInclusive.
type InstantRange struct {
	FromInclusive time.Time
	ToInclusive   time.Time
}

// ParseRequiredInstantRange parses from and to as RFC3339 (or RFC3339Nano), requires both,
// normalizes to UTC, enforces from <= to, and caps the span at 90 days.
func ParseRequiredInstantRange(fromStr, toStr string) (InstantRange, error) {
	var zero InstantRange
	if fromStr == "" || toStr == "" {
		return zero, fmt.Errorf(`query parameters "from" and "to" are required (RFC3339, e.g. 2006-04-09T00:30:00Z)`)
	}
	from, err := parseRFC3339Flexible(fromStr)
	if err != nil {
		return zero, fmt.Errorf(`invalid "from" instant: use RFC3339`)
	}
	to, err := parseRFC3339Flexible(toStr)
	if err != nil {
		return zero, fmt.Errorf(`invalid "to" instant: use RFC3339`)
	}
	from = from.UTC()
	to = to.UTC()
	if from.After(to) {
		return zero, fmt.Errorf(`"from" must be on or before "to"`)
	}
	if to.Sub(from) > maxTicketAnnouncementRange {
		return zero, fmt.Errorf(`time span between "from" and "to" must be at most 90 days`)
	}
	return InstantRange{FromInclusive: from, ToInclusive: to}, nil
}

func parseRFC3339Flexible(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}
