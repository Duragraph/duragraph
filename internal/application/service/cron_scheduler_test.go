package service

import (
	"testing"
	"time"
)

func TestValidateSchedule(t *testing.T) {
	tests := []struct {
		name     string
		schedule string
		wantErr  bool
	}{
		{"valid every midnight", "0 0 * * *", false},
		{"valid every 15 min", "*/15 * * * *", false},
		{"valid weekdays 9am", "0 9 * * 1-5", false},
		{"invalid expression", "not-a-cron", true},
		{"invalid too few fields", "0 0 *", true},
		{"invalid too many fields", "0 0 * * * *", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSchedule(tt.schedule)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSchedule(%q) error = %v, wantErr %v", tt.schedule, err, tt.wantErr)
			}
		})
	}
}

func TestComputeNextRun(t *testing.T) {
	from := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	next, err := ComputeNextRun("0 12 * * *", "UTC", from)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("next = %v, want %v", next, expected)
	}
}

func TestComputeNextRun_Timezone(t *testing.T) {
	from := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	next, err := ComputeNextRun("0 9 * * *", "America/New_York", from)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nyLoc, _ := time.LoadLocation("America/New_York")
	localNext := next.In(nyLoc)
	if localNext.Hour() != 9 {
		t.Errorf("expected 9:00 NY time, got %v", localNext)
	}
}

func TestComputeNextRun_InvalidTimezone(t *testing.T) {
	_, err := ComputeNextRun("0 0 * * *", "Invalid/Zone", time.Now())
	if err == nil {
		t.Error("expected error for invalid timezone")
	}
}

func TestComputeNextRun_InvalidSchedule(t *testing.T) {
	_, err := ComputeNextRun("bad", "UTC", time.Now())
	if err == nil {
		t.Error("expected error for invalid schedule")
	}
}
