package business_hours_dto

import (
	"sync/atomic"
	"time"
)

// ExcludedWindow is a recurring slot inside the working day that does not
// count toward a ticket's time: a Daily Stand Up, a Sprint Planning / Grooming.
// Set per project in Admin > Aturan Board.
type ExcludedWindow struct {
	Name    string
	Weekday time.Weekday
	// StartMinute is minutes after midnight (WIB): 08:45 is 525.
	StartMinute int
	Minutes     int
	// SprintWeek 0 = every week; N = only the Nth calendar (Monday-based) week of
	// the sprint, which needs the sprint's start date (see Schedule.SprintStart).
	SprintWeek int
}

// Schedule is one project's excluded windows plus the start of the sprint the
// "week N" windows count from.
type Schedule struct {
	Windows     []ExcludedWindow
	SprintStart time.Time
}

// schedules is the registry every time calculation reads, keyed by project
// key, replaced whole by tracking_rule on every load/save (same style as
// jira_client_dto.SetRules).
var schedules atomic.Pointer[map[string]Schedule]

// SetSchedules installs the schedules; nil clears them.
func SetSchedules(m map[string]Schedule) {
	if m == nil {
		schedules.Store(nil)
		return
	}
	cp := make(map[string]Schedule, len(m))
	for k, v := range m {
		cp[k] = v
	}
	schedules.Store(&cp)
}

// ScheduleFor is the project's schedule (zero value: nothing excluded).
func ScheduleFor(project string) Schedule {
	if m := schedules.Load(); m != nil {
		return (*m)[project]
	}
	return Schedule{}
}

// ForProject returns the calendar with the project's excluded windows (and the
// active sprint's start) attached. The calendar itself is not changed.
func (c HolidayCalendar) ForProject(project string) HolidayCalendar {
	s := ScheduleFor(project)
	c.Windows, c.SprintStart = s.Windows, s.SprintStart
	return c
}

// WithSprintStart returns the calendar with the "week N" windows counted from
// start; the zero time turns the week-N windows off (the sprint is unknown).
func (c HolidayCalendar) WithSprintStart(start time.Time) HolidayCalendar {
	c.SprintStart = start
	return c
}

var wibZone = time.FixedZone("WIB", 7*3600)

// mondayOf is midnight WIB of the Monday of t's week.
func mondayOf(t time.Time) time.Time {
	t = t.In(wibZone)
	back := (int(t.Weekday()) + 6) % 7
	return time.Date(t.Year(), t.Month(), t.Day()-back, 0, 0, 0, 0, wibZone)
}

// SprintWeekOf is which calendar week of the sprint day falls in: 1 for the
// week the sprint starts in, 2 for the next, and so on (0 or less before it).
func SprintWeekOf(day, sprintStart time.Time) int {
	days := mondayOf(day).Sub(mondayOf(sprintStart)).Hours() / 24
	if days < 0 {
		return int(days / 7) // 0 or negative: before the sprint
	}
	return int(days/7+0.5) + 1
}

// ExcludedOn lists the excluded windows that apply on day's date (WIB) as
// absolute intervals [start, end).
func (c HolidayCalendar) ExcludedOn(day time.Time) [][2]time.Time {
	day = day.In(wibZone)
	var out [][2]time.Time
	for _, w := range c.Windows {
		if w.Weekday != day.Weekday() || w.Minutes <= 0 {
			continue
		}
		if w.SprintWeek > 0 {
			if c.SprintStart.IsZero() || SprintWeekOf(day, c.SprintStart) != w.SprintWeek {
				continue
			}
		}
		start := time.Date(day.Year(), day.Month(), day.Day(), 0, w.StartMinute, 0, 0, wibZone)
		out = append(out, [2]time.Time{start, start.Add(time.Duration(w.Minutes) * time.Minute)})
	}
	return out
}
