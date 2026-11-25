package tracking_rule

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"nudge/modules/business_hours/business_hours_dto"
)

// ExcludedSchedule is the part of the working day a project does not count
// toward a ticket's time (Aturan Board › Jadwal yang tidak dihitung): the Daily
// Stand Up per weekday and special rituals such as Sprint Planning / Grooming.
type ExcludedSchedule struct {
	DSU     []DSUSlot `json:"dsu"`
	Rituals []Ritual  `json:"rituals"`
}

// DSUSlot is a Daily Stand Up: a weekday (1 = Monday … 5 = Friday), the start
// "HH:MM" (WIB) and how many minutes it takes.
type DSUSlot struct {
	Weekday int    `json:"weekday"`
	Start   string `json:"start"`
	Minutes int    `json:"minutes"`
}

const (
	FrequencyWeekly     = "weekly"
	FrequencySprintWeek = "sprint_week"
)

// Ritual is a named recurring slot: every week, or only in the Nth calendar
// week of the sprint. A disabled ritual is kept but not applied.
type Ritual struct {
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	Weekday    int    `json:"weekday"`
	Start      string `json:"start"`
	Minutes    int    `json:"minutes"`
	Frequency  string `json:"frequency"`
	SprintWeek int    `json:"sprintWeek"`
}

const (
	maxSlotMinutes   = 9 * 60
	maxRituals       = 10
	maxRitualName    = 60
	maxSprintWeekNum = 6
)

// IsZero: nothing is excluded.
func (e ExcludedSchedule) IsZero() bool { return len(e.DSU) == 0 && len(e.Rituals) == 0 }

// parseClock reads "HH:MM" as minutes after midnight.
func parseClock(raw string) (int, bool) {
	hh, mm, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok || len(hh) != 2 || len(mm) != 2 {
		return 0, false
	}
	h, err1 := strconv.Atoi(hh)
	m, err2 := strconv.Atoi(mm)
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// Windows turns the schedule into what the time calculation reads: every DSU
// and every enabled ritual as a recurring window.
func (e ExcludedSchedule) Windows() []business_hours_dto.ExcludedWindow {
	var out []business_hours_dto.ExcludedWindow
	for _, d := range e.DSU {
		start, ok := parseClock(d.Start)
		if !ok || d.Minutes <= 0 {
			continue
		}
		out = append(out, business_hours_dto.ExcludedWindow{Name: "Daily Stand Up", Weekday: time.Weekday(d.Weekday), StartMinute: start, Minutes: d.Minutes})
	}
	for _, r := range e.Rituals {
		start, ok := parseClock(r.Start)
		if !r.Enabled || !ok || r.Minutes <= 0 {
			continue
		}
		w := business_hours_dto.ExcludedWindow{Name: r.Name, Weekday: time.Weekday(r.Weekday), StartMinute: start, Minutes: r.Minutes}
		if r.Frequency == FrequencySprintWeek {
			w.SprintWeek = r.SprintWeek
		}
		out = append(out, w)
	}
	return out
}

// NeedsSprintStart: some enabled ritual counts "week N of the sprint".
func (e ExcludedSchedule) NeedsSprintStart() bool {
	for _, r := range e.Rituals {
		if r.Enabled && r.Frequency == FrequencySprintWeek {
			return true
		}
	}
	return false
}

// normalizeExcluded checks the schedule from the form: weekdays Monday–Friday,
// times "HH:MM", lengths 1–540 minutes, one DSU per weekday, a name for every
// ritual. It returns the cleaned schedule (empty slices, never nil) or a message.
func normalizeExcluded(in *ExcludedSchedule) (ExcludedSchedule, string) {
	out := ExcludedSchedule{DSU: []DSUSlot{}, Rituals: []Ritual{}}
	if in == nil {
		return out, ""
	}
	seen := map[int]bool{}
	for _, d := range in.DSU {
		if d.Weekday < 1 || d.Weekday > 5 {
			return ExcludedSchedule{}, "Hari Daily Stand Up harus Senin sampai Jumat"
		}
		if seen[d.Weekday] {
			return ExcludedSchedule{}, "Daily Stand Up hanya satu per hari"
		}
		seen[d.Weekday] = true
		if _, ok := parseClock(d.Start); !ok {
			return ExcludedSchedule{}, fmt.Sprintf("Jam mulai DSU %q tidak valid, pakai format HH:MM", d.Start)
		}
		if d.Minutes < 1 || d.Minutes > maxSlotMinutes {
			return ExcludedSchedule{}, fmt.Sprintf("Durasi DSU harus 1 sampai %d menit", maxSlotMinutes)
		}
		out.DSU = append(out.DSU, DSUSlot{Weekday: d.Weekday, Start: strings.TrimSpace(d.Start), Minutes: d.Minutes})
	}
	if len(in.Rituals) > maxRituals {
		return ExcludedSchedule{}, fmt.Sprintf("Maksimal %d kegiatan khusus", maxRituals)
	}
	for _, r := range in.Rituals {
		name := strings.TrimSpace(r.Name)
		switch {
		case name == "" || len([]rune(name)) > maxRitualName:
			return ExcludedSchedule{}, fmt.Sprintf("Nama kegiatan wajib diisi, maksimal %d karakter", maxRitualName)
		case r.Weekday < 1 || r.Weekday > 5:
			return ExcludedSchedule{}, fmt.Sprintf("Hari kegiatan %q harus Senin sampai Jumat", name)
		case r.Minutes < 1 || r.Minutes > maxSlotMinutes:
			return ExcludedSchedule{}, fmt.Sprintf("Durasi kegiatan %q harus 1 sampai %d menit", name, maxSlotMinutes)
		case r.Frequency != FrequencyWeekly && r.Frequency != FrequencySprintWeek:
			return ExcludedSchedule{}, fmt.Sprintf("Frekuensi kegiatan %q tidak dikenal", name)
		case r.Frequency == FrequencySprintWeek && (r.SprintWeek < 1 || r.SprintWeek > maxSprintWeekNum):
			return ExcludedSchedule{}, fmt.Sprintf("Minggu sprint kegiatan %q harus 1 sampai %d", name, maxSprintWeekNum)
		}
		if _, ok := parseClock(r.Start); !ok {
			return ExcludedSchedule{}, fmt.Sprintf("Jam mulai kegiatan %q tidak valid, pakai format HH:MM", name)
		}
		week := 0
		if r.Frequency == FrequencySprintWeek {
			week = r.SprintWeek
		}
		out.Rituals = append(out.Rituals, Ritual{Name: name, Enabled: r.Enabled, Weekday: r.Weekday, Start: strings.TrimSpace(r.Start), Minutes: r.Minutes, Frequency: r.Frequency, SprintWeek: week})
	}
	return out, ""
}
