package validate

import (
	"testing"
	"time"
)

func TestParseRequiredDateRange(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		from    string
		to      string
		wantErr bool
	}{
		{name: "missing from", from: "", to: "2026-04-30", wantErr: true},
		{name: "missing to", from: "2026-04-01", to: "", wantErr: true},
		{name: "bad from", from: "04-01-2026", to: "2026-04-30", wantErr: true},
		{name: "from after to", from: "2026-05-01", to: "2026-04-01", wantErr: true},
		{name: "span too long", from: "2026-04-01", to: "2026-05-03", wantErr: true},
		{name: "april window ok", from: "2026-04-01", to: "2026-04-30", wantErr: false},
		{name: "31 day span ok", from: "2026-04-01", to: "2026-05-02", wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dr, err := ParseRequiredDateRange(tt.from, tt.to)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if dr.ToExclusive.Sub(dr.FromInclusive) <= 0 {
				t.Fatalf("invalid range")
			}
		})
	}
}

func TestParseRequiredDateRangeExclusiveEnd(t *testing.T) {
	t.Parallel()
	dr, err := ParseRequiredDateRange("2026-04-01", "2026-04-01")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	if !dr.ToExclusive.Equal(want) {
		t.Fatalf("ToExclusive = %v, want %v", dr.ToExclusive, want)
	}
}
