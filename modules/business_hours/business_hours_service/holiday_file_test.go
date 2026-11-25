package business_hours_service

import (
	"testing"
	"time"
)

// The vendored calendar holds the national public holidays only. Cuti bersama
// (the extra days off the government sets) are NOT holidays here: the company
// does not observe them, so those days count as working days.
func TestVendoredHolidayCalendar_HasNationalHolidaysAndNoCutiBersama(t *testing.T) {
	cal, err := NewBusinessHoursService().LoadHolidayCalendar("../../../config/holidays_id.json")
	if err != nil {
		t.Fatalf("LoadHolidayCalendar: %v", err)
	}

	national := []string{
		"2026-01-01", "2026-01-16", "2026-02-17", "2026-03-19", "2026-03-21", "2026-03-22", "2026-04-03", "2026-04-05",
		"2026-05-01", "2026-05-14", "2026-05-27", "2026-05-31", "2026-06-01", "2026-06-16", "2026-08-17", "2026-08-25", "2026-12-25",
	}
	for _, day := range national {
		if !cal.Holidays[day] {
			t.Errorf("%s is a national holiday and must be in the calendar", day)
		}
	}
	cutiBersama := []string{"2026-02-16", "2026-03-18", "2026-03-20", "2026-03-23", "2026-03-24", "2026-05-15", "2026-05-28", "2026-12-24"}
	for _, day := range cutiBersama {
		if cal.Holidays[day] {
			t.Errorf("%s is a cuti bersama: the company does not observe it, so it must not be a holiday", day)
		}
	}
	if len(cal.Holidays) != len(national) {
		t.Errorf("the calendar holds %d days, want exactly the %d national holidays", len(cal.Holidays), len(national))
	}

	// 24 Dec 2026 is a Thursday: a full working day now.
	svc := NewBusinessHoursService()
	wib := time.FixedZone("WIB", 7*3600)
	got := svc.ElapsedBusinessSeconds(time.Date(2026, 12, 24, 8, 30, 0, 0, wib), time.Date(2026, 12, 24, 17, 30, 0, 0, wib), cal)
	if got != 8*3600 {
		t.Errorf("24 Dec counts %d s, want 28800", got)
	}
}
