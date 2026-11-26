package business_hours_service

import (
	"testing"
	"time"

	"nudge/modules/business_hours/business_hours_dto"
)

func wibAt(y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, jakartaLocation)
}

func TestNextCountingChange(t *testing.T) {
	// Thu 26 Nov 2026 is a working day; Fri 27 is too; 28-29 is the weekend.
	cal := business_hours_dto.HolidayCalendar{}
	cases := []struct {
		name string
		from time.Time
		want time.Time
	}{
		{"morning → lunch", wibAt(2026, 11, 26, 10, 15), wibAt(2026, 11, 26, 12, 0)},
		{"lunch → afternoon", wibAt(2026, 11, 26, 12, 30), wibAt(2026, 11, 26, 13, 0)},
		{"afternoon → end of day", wibAt(2026, 11, 26, 16, 0), wibAt(2026, 11, 26, 17, 30)},
		{"evening → next morning", wibAt(2026, 11, 26, 19, 0), wibAt(2026, 11, 27, 8, 30)},
		{"before the day starts", wibAt(2026, 11, 26, 7, 0), wibAt(2026, 11, 26, 8, 30)},
		{"friday evening → monday", wibAt(2026, 11, 27, 18, 0), wibAt(2026, 11, 30, 8, 30)},
		{"saturday → monday", wibAt(2026, 11, 28, 11, 0), wibAt(2026, 11, 30, 8, 30)},
	}
	for _, c := range cases {
		got, ok := NextCountingChange(c.from, cal)
		if !ok || !got.Equal(c.want) {
			t.Errorf("%s: got %v (%v), want %v", c.name, got, ok, c.want)
		}
	}
}

func TestNextCountingChange_SkipsHolidaysAndStopsAtExcludedWindows(t *testing.T) {
	// Friday 27 Nov is a holiday: from Thursday evening the next counted moment is Monday.
	cal := business_hours_dto.HolidayCalendar{Holidays: map[string]bool{"2026-11-27": true}}
	got, ok := NextCountingChange(wibAt(2026, 11, 26, 18, 0), cal)
	if !ok || !got.Equal(wibAt(2026, 11, 30, 8, 30)) {
		t.Fatalf("holiday: got %v (%v)", got, ok)
	}

	// A Daily Stand Up 08:45-09:15 on Thursdays: counting stops at 08:45 and resumes at 09:15.
	dsu := business_hours_dto.HolidayCalendar{Windows: []business_hours_dto.ExcludedWindow{{Weekday: time.Thursday, StartMinute: 8*60 + 45, Minutes: 30}}}
	got, _ = NextCountingChange(wibAt(2026, 11, 26, 8, 40), dsu)
	if !got.Equal(wibAt(2026, 11, 26, 8, 45)) {
		t.Fatalf("DSU start: got %v", got)
	}
	got, _ = NextCountingChange(wibAt(2026, 11, 26, 9, 0), dsu)
	if !got.Equal(wibAt(2026, 11, 26, 9, 15)) {
		t.Fatalf("DSU end: got %v", got)
	}
}
