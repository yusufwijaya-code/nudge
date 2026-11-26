package time_report

import (
	"math/rand"
	"testing"
	"time"

	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/business_hours/business_hours_service"
)

func at(y int, m time.Month, d, h, min int) time.Time { return time.Date(y, m, d, h, min, 0, 0, wib) }

func dedOf(b Breakdown, key string) int64 {
	for _, d := range b.Deductions {
		if d.Key == key {
			return d.Seconds
		}
	}
	return 0
}

func checkSum(t *testing.T, b Breakdown) {
	t.Helper()
	if b.RawSeconds != b.CountedSeconds+b.DeductedSeconds() {
		t.Fatalf("raw %d != counted %d + deducted %d", b.RawSeconds, b.CountedSeconds, b.DeductedSeconds())
	}
}

func TestMeasure_AWorkingDayIsSplitIntoItsReasons(t *testing.T) {
	// Thursday 26 Nov 2026, 07:00 → 19:00: 12 h raw.
	var b Breakdown
	Calendar{}.Measure(at(2026, 11, 26, 7, 0), at(2026, 11, 26, 19, 0), &b)
	checkSum(t, b)
	if b.RawSeconds != 12*3600 || b.CountedSeconds != 8*3600 {
		t.Fatalf("raw %d counted %d, want 12 h and 8 h", b.RawSeconds, b.CountedSeconds)
	}
	if dedOf(b, CatOutside) != 3*3600 || dedOf(b, CatLunch) != 3600 {
		t.Fatalf("outside %d lunch %d, want 3 h and 1 h", dedOf(b, CatOutside), dedOf(b, CatLunch))
	}
}

func TestMeasure_DaysOffTakeTheWholeDay(t *testing.T) {
	cal := Calendar{National: map[string]bool{"2026-12-25": true}, Leave: map[string]bool{"2026-12-24": true}}
	var b Breakdown
	// Wed 23 Dec 17:30 → Mon 28 Dec 08:30: Thu leave, Fri holiday, Sat+Sun weekend.
	cal.Measure(at(2026, 12, 23, 17, 30), at(2026, 12, 28, 8, 30), &b)
	checkSum(t, b)
	if b.CountedSeconds != 0 {
		t.Fatalf("counted %d, want 0", b.CountedSeconds)
	}
	if dedOf(b, CatLeave) != 24*3600 || dedOf(b, CatHoliday) != 24*3600 || dedOf(b, CatWeekend) != 48*3600 {
		t.Fatalf("leave %d holiday %d weekend %d", dedOf(b, CatLeave), dedOf(b, CatHoliday), dedOf(b, CatWeekend))
	}
	// Wed 17:30 → 24:00 (6.5 h) and Mon 00:00 → 08:30 (8.5 h) are outside the working day.
	if dedOf(b, CatOutside) != 15*3600 {
		t.Fatalf("outside %d, want 15 h", dedOf(b, CatOutside))
	}
}

func TestMeasure_BoardSlotsAreNamedAndOverlappingOnesTakeEachSecondOnce(t *testing.T) {
	cal := Calendar{Windows: []business_hours_dto.ExcludedWindow{
		{Kind: "dsu", Name: "x", Weekday: time.Thursday, StartMinute: 8*60 + 45, Minutes: 30},
		{Kind: "ritual", Name: "Sprint Planning", Weekday: time.Thursday, StartMinute: 9 * 60, Minutes: 120},            // overlaps the DSU by 15 min
		{Kind: "ritual", Name: "Retro", Weekday: time.Thursday, StartMinute: 11*60 + 30, Minutes: 90},                   // 30 min before lunch, the rest is lunch
		{Kind: "ritual", Name: "Week 2 only", Weekday: time.Thursday, StartMinute: 15 * 60, Minutes: 60, SprintWeek: 2}, // sprint unknown: off
	}}
	var b Breakdown
	cal.Measure(at(2026, 11, 26, 8, 30), at(2026, 11, 26, 17, 30), &b)
	checkSum(t, b)
	if dedOf(b, "slot:Daily Stand Up") != 30*60 || dedOf(b, "slot:Sprint Planning") != 105*60 || dedOf(b, "slot:Retro") != 30*60 {
		t.Fatalf("slots: dsu %d planning %d retro %d", dedOf(b, "slot:Daily Stand Up"), dedOf(b, "slot:Sprint Planning"), dedOf(b, "slot:Retro"))
	}
	if dedOf(b, "slot:Week 2 only") != 0 || dedOf(b, CatLunch) != 3600 {
		t.Fatalf("week-2 slot without a sprint must not apply; lunch %d", dedOf(b, CatLunch))
	}
	if b.CountedSeconds != 8*3600-165*60 { // 8 h working day minus 30 + 105 + 30 min of slots
		t.Fatalf("counted %d", b.CountedSeconds)
	}
}

func TestMeasure_WeekNSlotsFollowTheSprintOfTheDay(t *testing.T) {
	cal := Calendar{
		Windows: []business_hours_dto.ExcludedWindow{{Kind: "ritual", Name: "Grooming", Weekday: time.Monday, StartMinute: 13 * 60, Minutes: 60, SprintWeek: 2}},
		// A sprint started on Monday 16 Nov 2026: 23 Nov is its week 2, 30 Nov its week 3.
		SprintStartOn: func(time.Time) time.Time { return at(2026, 11, 16, 9, 0) },
	}
	var week2, week3 Breakdown
	cal.Measure(at(2026, 11, 23, 8, 30), at(2026, 11, 23, 17, 30), &week2)
	cal.Measure(at(2026, 11, 30, 8, 30), at(2026, 11, 30, 17, 30), &week3)
	if dedOf(week2, "slot:Grooming") != 3600 || dedOf(week3, "slot:Grooming") != 0 {
		t.Fatalf("week 2 %d, week 3 %d", dedOf(week2, "slot:Grooming"), dedOf(week3, "slot:Grooming"))
	}
}

// The counted time must be exactly what Nudge's own time calculation gives, for any
// stretch: the report only explains the number, it never computes a different one.
func TestMeasure_CountedTimeMatchesTheTimeCalculationForRandomStretches(t *testing.T) {
	bh := business_hours_service.NewBusinessHoursService()
	windows := []business_hours_dto.ExcludedWindow{
		{Kind: "dsu", Weekday: time.Tuesday, StartMinute: 8*60 + 45, Minutes: 30},
		{Kind: "dsu", Weekday: time.Thursday, StartMinute: 8*60 + 45, Minutes: 30},
		{Kind: "ritual", Name: "Planning", Weekday: time.Monday, StartMinute: 11 * 60, Minutes: 390, SprintWeek: 1},
		{Kind: "ritual", Name: "Review", Weekday: time.Friday, StartMinute: 12*60 + 30, Minutes: 150},
	}
	sprintStart := at(2026, 11, 23, 9, 0)
	national := map[string]bool{"2026-12-25": true, "2027-01-01": true}
	leave := map[string]bool{"2026-12-02": true, "2026-12-03": true}
	merged := map[string]bool{}
	for k := range national {
		merged[k] = true
	}
	for k := range leave {
		merged[k] = true
	}
	engineCal := business_hours_dto.HolidayCalendar{Holidays: merged, Windows: windows, SprintStart: sprintStart}
	cal := Calendar{National: national, Leave: leave, Windows: windows, SprintStartOn: func(time.Time) time.Time { return sprintStart }}

	r := rand.New(rand.NewSource(42))
	base := at(2026, 11, 20, 0, 0)
	for i := 0; i < 3000; i++ {
		// Jira's changelog has whole seconds: so do these stretches.
		start := base.Add(time.Duration(r.Int63n(45*24*3600)) * time.Second)
		end := start.Add(time.Duration(r.Int63n(10*24*3600)) * time.Second)
		var b Breakdown
		cal.Measure(start, end, &b)
		checkSum(t, b)
		if want := bh.ElapsedBusinessSeconds(start, end, engineCal); b.CountedSeconds != want {
			t.Fatalf("%v → %v: counted %d, time calculation %d", start, end, b.CountedSeconds, want)
		}
	}
}

func TestMergeAddsStretchesAndKeepsTheSum(t *testing.T) {
	var a, b, total Breakdown
	Calendar{}.Measure(at(2026, 11, 26, 7, 0), at(2026, 11, 26, 12, 30), &a)
	Calendar{}.Measure(at(2026, 11, 28, 9, 0), at(2026, 11, 28, 10, 0), &b)
	total.Merge(a)
	total.Merge(b)
	checkSum(t, total)
	if total.RawSeconds != a.RawSeconds+b.RawSeconds || dedOf(total, CatWeekend) != 3600 {
		t.Fatalf("total = %+v", total)
	}
}
