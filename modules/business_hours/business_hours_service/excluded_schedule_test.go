package business_hours_service

import (
	"testing"
	"time"

	"nudge/modules/business_hours/business_hours_dto"
)

var wib = time.FixedZone("WIB", 7*3600)

func at(y int, m time.Month, d, h, min int) time.Time { return time.Date(y, m, d, h, min, 0, 0, wib) }

// Nov 2026: Mon 9, Tue 10, Wed 11 ... Mon 16, Tue 17.
func dsuTuesdays() business_hours_dto.HolidayCalendar {
	return business_hours_dto.HolidayCalendar{Windows: []business_hours_dto.ExcludedWindow{
		{Name: "DSU", Weekday: time.Tuesday, StartMinute: 8*60 + 45, Minutes: 30},
	}}
}

func TestElapsed_ADailyStandUpIsNotCounted(t *testing.T) {
	svc := NewBusinessHoursService()
	cal := dsuTuesdays()

	// Tuesday 08:30-12:00: 3.5h minus the 08:45-09:15 DSU = 3h.
	if got := svc.ElapsedBusinessSeconds(at(2026, 11, 10, 8, 30), at(2026, 11, 10, 12, 0), cal); got != 3*3600 {
		t.Errorf("Tuesday morning = %d s, want 10800 (DSU left out)", got)
	}
	// Monday has no DSU: the same stretch counts in full.
	if got := svc.ElapsedBusinessSeconds(at(2026, 11, 9, 8, 30), at(2026, 11, 9, 12, 0), cal); got != 3*3600+1800 {
		t.Errorf("Monday morning = %d s, want 12600", got)
	}
	// A stretch that ends inside the DSU only counts up to its start.
	if got := svc.ElapsedBusinessSeconds(at(2026, 11, 10, 8, 30), at(2026, 11, 10, 9, 0), cal); got != 15*60 {
		t.Errorf("until mid-DSU = %d s, want 900", got)
	}
	// A stretch fully inside the DSU counts nothing.
	if got := svc.ElapsedBusinessSeconds(at(2026, 11, 10, 8, 50), at(2026, 11, 10, 9, 10), cal); got != 0 {
		t.Errorf("inside the DSU = %d s, want 0", got)
	}
}

// The user's own example: Sprint Planning / Grooming Monday 11:00, 390 minutes
// (11:00-17:30), only in week 2 of the sprint.
func TestElapsed_ARitualOnlyInWeekTwoOfTheSprint(t *testing.T) {
	svc := NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{
		Windows: []business_hours_dto.ExcludedWindow{{Name: "Sprint Planning / Grooming", Weekday: time.Monday, StartMinute: 11 * 60, Minutes: 390, SprintWeek: 2}},
		// the sprint started Wednesday 4 Nov: week 1 = 2-8 Nov, week 2 = 9-15 Nov
		SprintStart: at(2026, 11, 4, 9, 0),
	}
	// Monday 9 Nov (week 2): 08:30-12:00 minus lunch = 3h, minus 11:00-12:00 = 2.5h
	if got := svc.ElapsedBusinessSeconds(at(2026, 11, 9, 8, 30), at(2026, 11, 9, 17, 30), cal); got != 2*3600+1800 {
		t.Errorf("week 2 Monday = %d s, want 9000 (only 08:30-11:00 counts)", got)
	}
	// Monday 16 Nov is week 3: counts in full (8h).
	if got := svc.ElapsedBusinessSeconds(at(2026, 11, 16, 8, 30), at(2026, 11, 16, 17, 30), cal); got != 8*3600 {
		t.Errorf("week 3 Monday = %d s, want 28800", got)
	}
	// Without a known sprint start a "week N" window is off, never guessed.
	if got := svc.ElapsedBusinessSeconds(at(2026, 11, 9, 8, 30), at(2026, 11, 9, 17, 30), cal.WithSprintStart(time.Time{})); got != 8*3600 {
		t.Errorf("unknown sprint = %d s, want 28800", got)
	}
}

func TestElapsed_OverlappingWindowsAndTheLunchBreakAreNotSubtractedTwice(t *testing.T) {
	svc := NewBusinessHoursService()
	cal := business_hours_dto.HolidayCalendar{Windows: []business_hours_dto.ExcludedWindow{
		{Name: "A", Weekday: time.Tuesday, StartMinute: 11 * 60, Minutes: 120},    // 11:00-13:00 spans the lunch break
		{Name: "B", Weekday: time.Tuesday, StartMinute: 11*60 + 30, Minutes: 120}, // 11:30-13:30 overlaps A
	}}
	// Tuesday: 08:30-11:00 (2.5h) + 13:30-17:30 (4h) = 6.5h
	if got := svc.ElapsedBusinessSeconds(at(2026, 11, 10, 8, 30), at(2026, 11, 10, 17, 30), cal); got != 6*3600+1800 {
		t.Errorf("Tuesday = %d s, want 23400", got)
	}
}

func TestSprintWeekOf_CountsMondayBasedWeeksFromTheSprintStart(t *testing.T) {
	start := at(2026, 11, 4, 9, 0) // a Wednesday
	cases := map[time.Time]int{
		at(2026, 11, 2, 10, 0):  1, // Monday of the start week
		at(2026, 11, 4, 10, 0):  1,
		at(2026, 11, 8, 10, 0):  1, // Sunday
		at(2026, 11, 9, 10, 0):  2,
		at(2026, 11, 15, 23, 0): 2,
		at(2026, 11, 16, 10, 0): 3,
		at(2026, 10, 28, 10, 0): -1,
	}
	for day, want := range cases {
		if got := business_hours_dto.SprintWeekOf(day, start); got != want {
			t.Errorf("SprintWeekOf(%s) = %d, want %d", day.Format("Mon 2 Jan"), got, want)
		}
	}
}

func TestDeadline_SkipsTheExcludedWindowToo(t *testing.T) {
	svc := NewBusinessHoursService()
	cal := dsuTuesdays()
	// Tuesday from 08:30, 1 hour left: 08:30-08:45 (15m), skip the DSU, 09:15-10:00 (45m) -> 10:00.
	got := svc.DeadlineAfterBusinessSeconds(at(2026, 11, 10, 8, 30), 3600, cal)
	if !got.Equal(at(2026, 11, 10, 10, 0)) {
		t.Errorf("deadline = %s, want 10:00", got.Format("15:04"))
	}
	// Elapsed and deadline agree: the time between "now" and the deadline is exactly what was asked.
	if back := svc.ElapsedBusinessSeconds(at(2026, 11, 10, 8, 30), got, cal); back != 3600 {
		t.Errorf("elapsed to the deadline = %d s, want 3600", back)
	}
}

func TestIsWithinBusinessHours_ExcludedWindowIsOutside(t *testing.T) {
	svc := NewBusinessHoursService()
	cal := dsuTuesdays()
	if svc.IsWithinBusinessHours(at(2026, 11, 10, 9, 0), cal) {
		t.Error("09:00 on Tuesday is inside the DSU")
	}
	if !svc.IsWithinBusinessHours(at(2026, 11, 10, 9, 15), cal) || !svc.IsWithinBusinessHours(at(2026, 11, 9, 9, 0), cal) {
		t.Error("09:15 Tuesday and 09:00 Monday are working time")
	}
}

func TestSchedules_AreAttachedPerProjectAndTheCalendarStaysUntouched(t *testing.T) {
	t.Cleanup(func() { business_hours_dto.SetSchedules(nil) })
	business_hours_dto.SetSchedules(map[string]business_hours_dto.Schedule{
		"CORE": {Windows: dsuTuesdays().Windows, SprintStart: at(2026, 11, 4, 9, 0)},
	})
	base := business_hours_dto.HolidayCalendar{Holidays: map[string]bool{"2026-11-11": true}}

	core := base.ForProject("CORE")
	if len(core.Windows) != 1 || core.SprintStart.IsZero() || !core.Holidays["2026-11-11"] {
		t.Errorf("CORE calendar = %+v", core)
	}
	if other := base.ForProject("OMS"); len(other.Windows) != 0 {
		t.Errorf("OMS has no schedule: %+v", other)
	}
	if len(base.Windows) != 0 {
		t.Error("ForProject must not change the calendar it is called on")
	}
	if with := core.With([]string{"2026-11-12"}); len(with.Windows) != 1 || !with.Holidays["2026-11-12"] {
		t.Errorf("With must keep the windows: %+v", with)
	}
}
