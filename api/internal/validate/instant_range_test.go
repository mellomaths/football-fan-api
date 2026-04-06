package validate

import (
	"testing"
	"time"
)

func TestParseRequiredInstantRangeOK(t *testing.T) {
	t.Parallel()
	r, err := ParseRequiredInstantRange("2026-04-01T00:00:00Z", "2026-04-09T00:30:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if !r.FromInclusive.Equal(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("from: %v", r.FromInclusive)
	}
	if !r.ToInclusive.Equal(time.Date(2026, 4, 9, 0, 30, 0, 0, time.UTC)) {
		t.Fatalf("to: %v", r.ToInclusive)
	}
}

func TestParseRequiredInstantRangeRejectsFromAfterTo(t *testing.T) {
	t.Parallel()
	_, err := ParseRequiredInstantRange("2026-04-10T00:00:00Z", "2026-04-01T00:00:00Z")
	if err == nil {
		t.Fatal("want error")
	}
}
