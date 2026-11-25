// Package work_calendar answers "what counts, and what does not, on each day":
// weekends, national holidays, the user's own leave, the lunch break and the
// board's Daily Stand Up / special activities. It is the read-only view of the
// same rules the time calculation uses, so a user can see why a ticket's time
// is what it is.
package work_calendar

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/board/board_dto"
	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/user_leave/user_leave_dto"
)

var (
	ErrInvalidMonth  = errors.New("work_calendar: month must be YYYY-MM")
	ErrBoardNotFound = errors.New("work_calendar: board not found")
)

var wib = time.FixedZone("WIB", 7*3600)

// Day kinds.
const (
	KindWorkday = "workday"
	KindWeekend = "weekend"
	KindHoliday = "holiday"
	KindLeave   = "leave"
)

// Slot kinds (what is left out of a workday).
const (
	SlotLunch  = "lunch"
	SlotDSU    = "dsu"
	SlotRitual = "ritual"
)

type Slot struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Start string `json:"start"` // HH:MM
	End   string `json:"end"`
	// SprintWeek: the slot only applies in that week of the sprint (0 = every week).
	SprintWeek int `json:"sprintWeek,omitempty"`
}

type Day struct {
	Date    string `json:"date"`
	Weekday int    `json:"weekday"` // 0 = Sunday … 6 = Saturday
	Kind    string `json:"kind"`
	// CountedSeconds is what counts toward a ticket's time that day: 0 off a workday.
	CountedSeconds int64  `json:"countedSeconds"`
	Note           string `json:"note,omitempty"` // the leave note
	Slots          []Slot `json:"slots"`
}

type SprintInfo struct {
	Name  string `json:"name"`
	Start string `json:"start"` // 2006-01-02
	End   string `json:"end"`
}

type BoardInfo struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	ProjectKey string `json:"projectKey"`
}

// Weekly is the fixed pattern: what the board leaves out every week.
type Weekly struct {
	DSU     []Slot `json:"dsu"`
	Rituals []Slot `json:"rituals"`
	// Weekday of each slot, parallel to the slices above.
	DSUWeekdays    []int `json:"dsuWeekdays"`
	RitualWeekdays []int `json:"ritualWeekdays"`
}

type Month struct {
	Month     string      `json:"month"`
	Board     *BoardInfo  `json:"board"`
	Sprint    *SprintInfo `json:"sprint"`
	WorkStart string      `json:"workStart"`
	WorkEnd   string      `json:"workEnd"`
	Lunch     Slot        `json:"lunch"`
	Weekly    Weekly      `json:"weekly"`
	Days      []Day       `json:"days"`
}

// LeaveSource lists a user's own leave days.
type LeaveSource interface {
	List(ctx context.Context, email string) ([]user_leave_dto.Leave, error)
}

// BoardSource is the cached board read of board_service.
type BoardSource interface {
	Get(ctx context.Context, boardID int64) (board_dto.Board, bool, error)
	ActiveSprint(ctx context.Context, boardID int64) (jira_client_dto.SprintRef, bool, error)
}

// Clock counts the working seconds between two instants.
type Clock interface {
	ElapsedBusinessSeconds(start, end time.Time, cal business_hours_dto.HolidayCalendar) int64
}

type Service struct {
	national business_hours_dto.HolidayCalendar
	leaves   LeaveSource
	boards   BoardSource
	clock    Clock
}

// NewService: national is the calendar of national holidays only (no leave).
func NewService(national business_hours_dto.HolidayCalendar, leaves LeaveSource, boards BoardSource, clock Clock) *Service {
	return &Service{national: national, leaves: leaves, boards: boards, clock: clock}
}

func clock(min int) string { return fmt.Sprintf("%02d:%02d", min/60, min%60) }

// Month builds one month for email. boardID 0 = no board: only holidays, leave
// and the lunch break are shown. An unknown sprint (none active, or Jira
// unreachable) never gets "week N" slots guessed for it.
func (s *Service) Month(ctx context.Context, email, month string, boardID int64) (Month, error) {
	first, err := time.ParseInLocation("2006-01", month, wib)
	if err != nil || first.Year() < 2020 || first.Year() > 2100 {
		return Month{}, ErrInvalidMonth
	}

	out := Month{
		Month:     first.Format("2006-01"),
		WorkStart: clock(business_hours_dto.WorkStartMinute),
		WorkEnd:   clock(business_hours_dto.WorkEndMinute),
		Lunch:     Slot{Kind: SlotLunch, Name: "Istirahat", Start: clock(business_hours_dto.LunchStartMinute), End: clock(business_hours_dto.LunchEndMinute)},
		Weekly:    Weekly{DSU: []Slot{}, Rituals: []Slot{}, DSUWeekdays: []int{}, RitualWeekdays: []int{}},
		Days:      []Day{},
	}

	var windows []business_hours_dto.ExcludedWindow
	var sprintStart, sprintEnd time.Time
	if boardID > 0 {
		board, found, err := s.boards.Get(ctx, boardID)
		if err != nil {
			return Month{}, err
		}
		if !found {
			return Month{}, ErrBoardNotFound
		}
		out.Board = &BoardInfo{ID: board.BoardID, Name: board.Name, ProjectKey: board.ProjectKey}
		windows = business_hours_dto.ScheduleFor(board.ProjectKey).Windows
		if sprint, ok, err := s.boards.ActiveSprint(ctx, boardID); err == nil && ok {
			start, hasStart := sprint.StartTime()
			end, hasEnd := sprint.EndTime()
			if hasStart && hasEnd {
				sprintStart, sprintEnd = start, end
				out.Sprint = &SprintInfo{Name: sprint.Name, Start: start.In(wib).Format("2006-01-02"), End: end.In(wib).Format("2006-01-02")}
			}
		}
	}
	out.Weekly = weekly(windows)

	leaveNotes := map[string]string{}
	if s.leaves != nil && email != "" {
		list, err := s.leaves.List(ctx, email)
		if err != nil {
			return Month{}, err
		}
		for _, l := range list {
			leaveNotes[l.Date] = l.Note
		}
	}

	next := first.AddDate(0, 1, 0)
	for day := first; day.Before(next); day = day.AddDate(0, 0, 1) {
		out.Days = append(out.Days, s.day(day, windows, sprintStart, sprintEnd, leaveNotes))
	}
	return out, nil
}

// weekly lists the board's fixed slots in weekday order.
func weekly(windows []business_hours_dto.ExcludedWindow) Weekly {
	w := Weekly{DSU: []Slot{}, Rituals: []Slot{}, DSUWeekdays: []int{}, RitualWeekdays: []int{}}
	sorted := append([]business_hours_dto.ExcludedWindow(nil), windows...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Weekday != sorted[j].Weekday {
			return sorted[i].Weekday < sorted[j].Weekday
		}
		return sorted[i].StartMinute < sorted[j].StartMinute
	})
	for _, win := range sorted {
		slot := Slot{Name: win.Name, Start: clock(win.StartMinute), End: clock(min(win.StartMinute+win.Minutes, 24*60)), SprintWeek: win.SprintWeek}
		if win.Kind == business_hours_dto.DSUKind {
			slot.Kind = SlotDSU
			w.DSU, w.DSUWeekdays = append(w.DSU, slot), append(w.DSUWeekdays, int(win.Weekday))
		} else {
			slot.Kind = SlotRitual
			w.Rituals, w.RitualWeekdays = append(w.Rituals, slot), append(w.RitualWeekdays, int(win.Weekday))
		}
	}
	return w
}

func (s *Service) day(day time.Time, windows []business_hours_dto.ExcludedWindow, sprintStart, sprintEnd time.Time, leaveNotes map[string]string) Day {
	date := day.Format("2006-01-02")
	d := Day{Date: date, Weekday: int(day.Weekday()), Slots: []Slot{}}
	switch {
	case s.national.IsHoliday(day):
		d.Kind = KindHoliday
		return d
	case day.Weekday() == time.Saturday || day.Weekday() == time.Sunday:
		d.Kind = KindWeekend
		return d
	}
	if note, onLeave := leaveNotes[date]; onLeave {
		d.Kind, d.Note = KindLeave, note
		return d
	}
	d.Kind = KindWorkday

	// A "week N of the sprint" slot only applies on a day inside the active
	// sprint; outside it the sprint (and so the week) is not known.
	inSprint := !sprintStart.IsZero() && !day.Before(dayStart(sprintStart)) && day.Before(dayStart(sprintEnd).AddDate(0, 0, 1))
	applicable := make([]business_hours_dto.ExcludedWindow, 0, len(windows))
	for _, w := range windows {
		if w.SprintWeek > 0 && !inSprint {
			continue
		}
		applicable = append(applicable, w)
	}
	cal := business_hours_dto.HolidayCalendar{Holidays: s.national.Holidays, Windows: applicable, SprintStart: sprintStart}

	d.CountedSeconds = s.clock.ElapsedBusinessSeconds(day, day.Add(24*time.Hour-time.Second), cal)
	d.Slots = append(d.Slots, Slot{Kind: SlotLunch, Name: "Istirahat", Start: clock(business_hours_dto.LunchStartMinute), End: clock(business_hours_dto.LunchEndMinute)})
	for _, w := range cal.ExcludedOn(day) {
		start, end := minutesOf(w[0], day), minutesOf(w[1], day)
		// Only the part inside the working day matters.
		start, end = max(start, business_hours_dto.WorkStartMinute), min(end, business_hours_dto.WorkEndMinute)
		if end <= start {
			continue
		}
		name, kind, week := windowIdentity(applicable, day, w[0])
		d.Slots = append(d.Slots, Slot{Kind: kind, Name: name, Start: clock(start), End: clock(end), SprintWeek: week})
	}
	sort.SliceStable(d.Slots, func(i, j int) bool { return d.Slots[i].Start < d.Slots[j].Start })
	return d
}

func dayStart(t time.Time) time.Time {
	t = t.In(wib)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, wib)
}

func minutesOf(t, day time.Time) int {
	t = t.In(wib)
	return int(t.Sub(dayStart(day)).Minutes())
}

// windowIdentity finds the name/kind of the window that starts at start on day.
func windowIdentity(windows []business_hours_dto.ExcludedWindow, day, start time.Time) (name, kind string, week int) {
	startMin := minutesOf(start, day)
	for _, w := range windows {
		if w.Weekday == day.Weekday() && w.StartMinute == startMin {
			kind = SlotRitual
			if w.Kind == business_hours_dto.DSUKind {
				kind = SlotDSU
			}
			return w.Name, kind, w.SprintWeek
		}
	}
	return "Kegiatan khusus", SlotRitual, 0
}

// ---- HTTP ----

type API interface {
	Month(ctx context.Context, email, month string, boardID int64) (Month, error)
}

type Handler struct{ api API }

func NewHandler(api API) *Handler { return &Handler{api: api} }

// UserRoutes mounts the calendar on a group already behind AuthMiddlewareAPI.
func UserRoutes(g *gin.RouterGroup, h *Handler) { g.GET("/calendar", h.month) }

func (h *Handler) month(c *gin.Context) {
	session := c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session)
	month := c.Query("month")
	if month == "" {
		month = time.Now().In(wib).Format("2006-01")
	}
	var boardID int64
	if raw := c.Query("board"); raw != "" {
		if _, err := fmt.Sscanf(raw, "%d", &boardID); err != nil || boardID < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "board tidak valid"})
			return
		}
	}
	res, err := h.api.Month(c.Request.Context(), session.Email, month, boardID)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, res)
	case errors.Is(err, ErrInvalidMonth):
		c.JSON(http.StatusBadRequest, gin.H{"error": "bulan harus berformat YYYY-MM"})
	case errors.Is(err, ErrBoardNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "board tidak ditemukan"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memuat kalender, coba lagi"})
	}
}
