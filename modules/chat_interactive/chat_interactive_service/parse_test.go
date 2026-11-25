package chat_interactive_service

import (
	"testing"
	"time"
)

func TestHumanDuration_IsWordsWithoutZeroUnits(t *testing.T) {
	cases := map[int64]string{
		0:       "0 detik",
		52:      "52 detik",
		60:      "1 menit",
		2700:    "45 menit",
		7200:    "2 jam",
		8948:    "2 jam 29 menit 8 detik",
		-8948:   "2 jam 29 menit 8 detik", // the sign is ignored
		172_800: "48 jam",                 // two days are 48 hours
		90_061:  "25 jam 1 menit 1 detik", // no days: hours keep counting
	}
	for seconds, want := range cases {
		if got := humanDuration(seconds); got != want {
			t.Errorf("humanDuration(%d) = %q, want %q", seconds, got, want)
		}
	}
}

func TestDeadlineLabel_IncludesSecondsWhereWhenLabelDoesNot(t *testing.T) {
	now := time.Date(2026, 11, 25, 9, 0, 0, 0, wib)
	at := time.Date(2026, 11, 25, 11, 17, 26, 0, wib)
	if got := deadlineLabel(at, now); got != "11:17:26" {
		t.Errorf("deadlineLabel = %q, want 11:17:26", got)
	}
	if got := whenLabel(at, now); got != "11:17" {
		t.Errorf("whenLabel = %q, want the minute-precision 11:17 (mute/leave texts keep it)", got)
	}
}
