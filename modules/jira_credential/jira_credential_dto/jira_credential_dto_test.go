package jira_credential_dto

import (
	"errors"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

func TestParseExpiryDate_ReadsTheDayAsWIBAndKeepsItWhateverTheServerZone(t *testing.T) {
	day, err := ParseExpiryDate(" 2027-10-26 ", now)
	if err != nil || day == nil {
		t.Fatalf("ParseExpiryDate = %v, %v", day, err)
	}
	if got := FormatExpiryDate(day); got != "2027-10-26" {
		t.Errorf("round trip = %q, want 2027-10-26", got)
	}
	if _, offset := day.Zone(); offset != 7*3600 {
		t.Errorf("zone offset = %d, want WIB", offset)
	}
}

func TestParseExpiryDate_EmptyMeansUnknownAndPastDatesAreKept(t *testing.T) {
	for _, empty := range []string{"", "   "} {
		if day, err := ParseExpiryDate(empty, now); day != nil || err != nil {
			t.Errorf("ParseExpiryDate(%q) = %v, %v; want nil, nil", empty, day, err)
		}
	}
	// An already expired token still deserves a banner.
	if day, err := ParseExpiryDate("2026-11-23", now); err != nil || day == nil {
		t.Errorf("a past date must be accepted: %v, %v", day, err)
	}
}

func TestParseExpiryDate_RejectsTyposAndImplausibleYears(t *testing.T) {
	for _, bad := range []string{"26-10-2027", "2027/10/26", "2027-13-01", "2027-02-30", "soon", "2019-12-31", "2037-01-01"} {
		if _, err := ParseExpiryDate(bad, now); !errors.Is(err, ErrInvalidExpiry) {
			t.Errorf("ParseExpiryDate(%q) err = %v, want ErrInvalidExpiry", bad, err)
		}
	}
}

func TestFormatExpiryDate_NilIsEmpty(t *testing.T) {
	if got := FormatExpiryDate(nil); got != "" {
		t.Errorf("FormatExpiryDate(nil) = %q", got)
	}
}
