// Package time_report is the user's Laporan: every ticket of theirs created in a
// date range, with its time read again from Jira's changelog, broken down from the
// raw time in the counted statuses to the counted time (what is taken off, and why),
// then compared with the estimate: deviation and the cascading key result score.
package time_report

import (
	"time"

	"nudge/modules/business_hours/business_hours_dto"
)

var wib = time.FixedZone("WIB", 7*3600)

// The categories a stretch of raw time can fall in when it is not counted.
const (
	CatOutside = "outside" // before the working day starts or after it ends
	CatLunch   = "lunch"   // the lunch break
	CatWeekend = "weekend" // Saturday or Sunday
	CatHoliday = "holiday" // a national holiday (tanggal merah)
	CatLeave   = "leave"   // the user's own leave (cuti)
	CatSlot    = "slot"    // a board slot: Daily Stand Up or a special activity
)

// Deduction is time taken off the raw time, by category (a board slot by its name).
type Deduction struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Category string `json:"category"`
	Seconds  int64  `json:"seconds"`
}

// Breakdown is raw time split into what counts and what is taken off. It always
// holds that RawSeconds = CountedSeconds + the sum of the deductions.
type Breakdown struct {
	RawSeconds     int64       `json:"rawSeconds"`
	CountedSeconds int64       `json:"countedSeconds"`
	Deductions     []Deduction `json:"deductions"`
}

func (b *Breakdown) add(key, label, category string, seconds int64) {
	if seconds <= 0 {
		return
	}
	for i := range b.Deductions {
		if b.Deductions[i].Key == key {
			b.Deductions[i].Seconds += seconds
			return
		}
	}
	b.Deductions = append(b.Deductions, Deduction{Key: key, Label: label, Category: category, Seconds: seconds})
}

// Merge adds other into b (the totals of several stretches or tickets).
func (b *Breakdown) Merge(other Breakdown) {
	b.RawSeconds += other.RawSeconds
	b.CountedSeconds += other.CountedSeconds
	for _, d := range other.Deductions {
		b.add(d.Key, d.Label, d.Category, d.Seconds)
	}
}

// DeductedSeconds is everything taken off.
func (b Breakdown) DeductedSeconds() int64 {
	var n int64
	for _, d := range b.Deductions {
		n += d.Seconds
	}
	return n
}

// Calendar is what decides, for one user and one project, whether a moment counts:
// the same rules as the time calculation, kept apart so every reason can be named.
type Calendar struct {
	// National holidays and the user's leave days, "2006-01-02" (WIB).
	National map[string]bool
	Leave    map[string]bool
	// Windows are the project's board slots (Daily Stand Up, special activities).
	Windows []business_hours_dto.ExcludedWindow
	// SprintStartOn is the start of the sprint the day belongs to, for the slots that
	// only apply in week N of a sprint; the zero time turns those slots off that day.
	SprintStartOn func(day time.Time) time.Time
}

const (
	labelOutside = "Di luar jam kerja"
	labelLunch   = "Istirahat"
	labelWeekend = "Akhir pekan"
	labelHoliday = "Libur nasional"
	labelLeave   = "Cuti"
	labelDSU     = "Daily Stand Up"
)

// Labels of the fixed categories, for the page and the PDF.
func CategoryLabel(category string) string {
	switch category {
	case CatOutside:
		return labelOutside
	case CatLunch:
		return labelLunch
	case CatWeekend:
		return labelWeekend
	case CatHoliday:
		return labelHoliday
	case CatLeave:
		return labelLeave
	}
	return "Jadwal board"
}

func minutesInto(day time.Time, minutes int) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), 0, minutes, 0, 0, wib)
}

func overlap(aStart, aEnd, bStart, bEnd time.Time) int64 {
	start, end := aStart, aEnd
	if bStart.After(start) {
		start = bStart
	}
	if bEnd.Before(end) {
		end = bEnd
	}
	if !end.After(start) {
		return 0
	}
	return int64(end.Sub(start) / time.Second)
}

// subtract removes cut from every interval, keeping what is left on either side
// (the same as the time calculation's own subtractInterval).
func subtract(intervals [][2]time.Time, cut [2]time.Time) [][2]time.Time {
	var out [][2]time.Time
	for _, iv := range intervals {
		if !cut[1].After(iv[0]) || !cut[0].Before(iv[1]) {
			out = append(out, iv)
			continue
		}
		if cut[0].After(iv[0]) {
			out = append(out, [2]time.Time{iv[0], cut[0]})
		}
		if cut[1].Before(iv[1]) {
			out = append(out, [2]time.Time{cut[1], iv[1]})
		}
	}
	return out
}

// slotLabel is how a board slot is named in the report.
func slotLabel(w business_hours_dto.ExcludedWindow) string {
	if w.Kind == "dsu" {
		return labelDSU
	}
	if w.Name != "" {
		return w.Name
	}
	return "Kegiatan khusus"
}

// slotsOn are the board slots that apply on day, in the board's order.
func (c Calendar) slotsOn(day time.Time) []business_hours_dto.ExcludedWindow {
	var out []business_hours_dto.ExcludedWindow
	for _, w := range c.Windows {
		if w.Weekday != day.Weekday() || w.Minutes <= 0 {
			continue
		}
		if w.SprintWeek > 0 {
			var start time.Time
			if c.SprintStartOn != nil {
				start = c.SprintStartOn(day)
			}
			if start.IsZero() || business_hours_dto.SprintWeekOf(day, start) != w.SprintWeek {
				continue
			}
		}
		out = append(out, w)
	}
	return out
}

// Measure adds the stretch [start, end) to b: its raw length, what of it counts and,
// for the rest, why not. A day off (weekend, holiday, leave) takes the whole day; on
// a working day the time outside 08:30–17:30, the lunch break and the board slots
// are taken off, each second once, in that order.
func (c Calendar) Measure(start, end time.Time, b *Breakdown) {
	// Whole seconds, like the time calculation: no second is counted twice or lost.
	start, end = start.In(wib).Truncate(time.Second), end.In(wib).Truncate(time.Second)
	if !end.After(start) {
		return
	}
	b.RawSeconds += int64(end.Sub(start) / time.Second)

	for day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, wib); day.Before(end); day = day.AddDate(0, 0, 1) {
		next := day.AddDate(0, 0, 1)
		from, to := start, end
		if day.After(from) {
			from = day
		}
		if next.Before(to) {
			to = next
		}
		length := overlap(from, to, from, to)
		if length == 0 {
			continue
		}
		key := day.Format("2006-01-02")
		switch {
		case day.Weekday() == time.Saturday || day.Weekday() == time.Sunday:
			b.add(CatWeekend, labelWeekend, CatWeekend, length)
			continue
		case c.National[key]:
			b.add(CatHoliday, labelHoliday, CatHoliday, length)
			continue
		case c.Leave[key]:
			b.add(CatLeave, labelLeave, CatLeave, length)
			continue
		}

		workStart := minutesInto(day, business_hours_dto.WorkStartMinute)
		workEnd := minutesInto(day, business_hours_dto.WorkEndMinute)
		lunchStart := minutesInto(day, business_hours_dto.LunchStartMinute)
		lunchEnd := minutesInto(day, business_hours_dto.LunchEndMinute)

		b.add(CatOutside, labelOutside, CatOutside, length-overlap(from, to, workStart, workEnd))
		b.add(CatLunch, labelLunch, CatLunch, overlap(from, to, lunchStart, lunchEnd))

		intervals := [][2]time.Time{{workStart, lunchStart}, {lunchEnd, workEnd}}
		for _, w := range c.slotsOn(day) {
			slotStart := minutesInto(day, w.StartMinute)
			cut := [2]time.Time{slotStart, slotStart.Add(time.Duration(w.Minutes) * time.Minute)}
			var taken int64
			for _, iv := range intervals {
				s, e := iv[0], iv[1]
				if cut[0].After(s) {
					s = cut[0]
				}
				if cut[1].Before(e) {
					e = cut[1]
				}
				taken += overlap(from, to, s, e)
			}
			label := slotLabel(w)
			b.add("slot:"+label, label, CatSlot, taken)
			intervals = subtract(intervals, cut)
		}
		for _, iv := range intervals {
			b.CountedSeconds += overlap(from, to, iv[0], iv[1])
		}
	}
}
