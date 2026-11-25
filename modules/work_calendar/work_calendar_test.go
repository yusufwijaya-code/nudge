package work_calendar

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/board/board_dto"
	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/business_hours/business_hours_service"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/user_leave/user_leave_dto"
)

type fakeLeaves struct{ list []user_leave_dto.Leave }

func (f fakeLeaves) List(context.Context, string) ([]user_leave_dto.Leave, error) { return f.list, nil }

type fakeBoards struct {
	board     board_dto.Board
	found     bool
	sprint    jira_client_dto.SprintRef
	hasSprint bool
	sprintErr error
}

func (f fakeBoards) Get(context.Context, int64) (board_dto.Board, bool, error) {
	return f.board, f.found, nil
}
func (f fakeBoards) ActiveSprint(context.Context, int64) (jira_client_dto.SprintRef, bool, error) {
	return f.sprint, f.hasSprint, f.sprintErr
}

// Dec 2026: Tue 1, Wed 2 ... Mon 7, Mon 14. Nov 2026: Mon 23, Mon 30.
// The sprint runs 23 Nov - 4 Dec.
func newService(boards fakeBoards, leaves ...user_leave_dto.Leave) *Service {
	national := business_hours_dto.HolidayCalendar{Holidays: map[string]bool{"2026-12-24": true, "2026-12-25": true, "2026-12-06": true}}
	return NewService(national, fakeLeaves{list: leaves}, boards, business_hours_service.NewBusinessHoursService())
}

func coreBoard() fakeBoards {
	return fakeBoards{
		board: board_dto.Board{BoardID: 26, Name: "ESB Core Board", ProjectKey: "CORE"}, found: true, hasSprint: true,
		sprint: jira_client_dto.SprintRef{ID: 1, Name: "Core Sprint 24 2026", StartDate: "2026-11-23T02:00:00.000Z", EndDate: "2026-12-04T10:00:00.000Z"},
	}
}

func withSchedule(t *testing.T) {
	t.Helper()
	business_hours_dto.SetSchedules(map[string]business_hours_dto.Schedule{"CORE": {
		Windows: []business_hours_dto.ExcludedWindow{
			{Name: "Daily Stand Up", Kind: business_hours_dto.DSUKind, Weekday: time.Tuesday, StartMinute: 8*60 + 45, Minutes: 30},
			{Name: "Sprint Planning / Grooming", Kind: business_hours_dto.RitualKind, Weekday: time.Monday, StartMinute: 11 * 60, Minutes: 390, SprintWeek: 2},
		},
	}})
	t.Cleanup(func() { business_hours_dto.SetSchedules(nil) })
}

func dayOf(m Month, date string) Day {
	for _, d := range m.Days {
		if d.Date == date {
			return d
		}
	}
	return Day{}
}

func kinds(d Day) []string {
	var out []string
	for _, s := range d.Slots {
		out = append(out, s.Kind+" "+s.Start+"-"+s.End)
	}
	return out
}

func TestMonth_WeekendsHolidaysLeaveAndWorkdaysAreToldApart(t *testing.T) {
	svc := newService(fakeBoards{}, user_leave_dto.Leave{Date: "2026-12-10", Note: "Cuti tahunan"}, user_leave_dto.Leave{Date: "2026-12-24"})
	m, err := svc.Month(context.Background(), "a@esb.co.id", "2026-12", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Days) != 31 || m.Month != "2026-12" {
		t.Fatalf("month = %s with %d days", m.Month, len(m.Days))
	}
	want := map[string]string{
		"2026-12-01": KindWorkday, "2026-12-05": KindWeekend,
		"2026-12-06": KindHoliday, // a holiday on a Sunday is still a holiday
		"2026-12-10": KindLeave,
		"2026-12-24": KindHoliday, // a holiday wins over a leave day
		"2026-12-25": KindHoliday,
	}
	for date, kind := range want {
		if got := dayOf(m, date).Kind; got != kind {
			t.Errorf("%s = %q, want %q", date, got, kind)
		}
	}
	if dayOf(m, "2026-12-10").Note != "Cuti tahunan" {
		t.Error("the leave note must come through")
	}
	for _, off := range []string{"2026-12-05", "2026-12-06", "2026-12-10", "2026-12-24"} {
		if d := dayOf(m, off); d.CountedSeconds != 0 || len(d.Slots) != 0 {
			t.Errorf("%s counts %d s with slots %v, want nothing on a day off", off, d.CountedSeconds, d.Slots)
		}
	}
}

func TestMonth_AWorkdayCountsEightHoursAndShowsTheLunchBreak(t *testing.T) {
	m, err := newService(fakeBoards{}).Month(context.Background(), "a@esb.co.id", "2026-12", 0)
	if err != nil {
		t.Fatal(err)
	}
	d := dayOf(m, "2026-12-02")
	if d.CountedSeconds != 8*3600 {
		t.Errorf("counted = %d s, want 28800 (08:30-12:00 + 13:00-17:30)", d.CountedSeconds)
	}
	if got := kinds(d); len(got) != 1 || got[0] != "lunch 12:00-13:00" {
		t.Errorf("slots = %v, want only the lunch break", got)
	}
	if m.WorkStart != "08:30" || m.WorkEnd != "17:30" || m.Lunch.Start != "12:00" || m.Lunch.End != "13:00" || m.Board != nil || m.Sprint != nil {
		t.Errorf("header = %+v", m)
	}
}

func TestMonth_TheBoardsDSUIsLeftOutAndAWeekNActivityOutsideTheSprintIsNot(t *testing.T) {
	withSchedule(t)
	m, err := newService(coreBoard()).Month(context.Background(), "a@esb.co.id", "2026-12", 26)
	if err != nil {
		t.Fatal(err)
	}
	if m.Board == nil || m.Board.ProjectKey != "CORE" || m.Sprint == nil || m.Sprint.Start != "2026-11-23" || m.Sprint.End != "2026-12-04" {
		t.Fatalf("board/sprint = %+v / %+v", m.Board, m.Sprint)
	}

	// Tuesday 1 Dec: the DSU is cut, 8h - 30m = 7.5h.
	tue := dayOf(m, "2026-12-01")
	if tue.CountedSeconds != 7*3600+1800 {
		t.Errorf("Tuesday counts %d s, want 27000", tue.CountedSeconds)
	}
	if got := kinds(tue); len(got) != 2 || got[0] != "dsu 08:45-09:15" || got[1] != "lunch 12:00-13:00" {
		t.Errorf("Tuesday slots = %v", got)
	}
	// Monday 7 Dec is after the sprint ended: no sprint, so no week-N activity.
	if mon := dayOf(m, "2026-12-07"); len(mon.Slots) != 1 || mon.CountedSeconds != 8*3600 {
		t.Errorf("Monday 7 Dec = %v / %d s, want no activity (outside the sprint)", kinds(mon), mon.CountedSeconds)
	}
}

func TestMonth_AWeekNActivityAppearsInsideTheActiveSprintClippedToTheWorkingDay(t *testing.T) {
	withSchedule(t)
	nov, err := newService(coreBoard()).Month(context.Background(), "a@esb.co.id", "2026-11", 26)
	if err != nil {
		t.Fatal(err)
	}
	// The sprint started Mon 23 Nov, so Mon 30 Nov is week 2: 08:30-11:00 counts, 11:00 on does not.
	week2 := dayOf(nov, "2026-11-30")
	if week2.CountedSeconds != 2*3600+1800 {
		t.Errorf("Monday 30 Nov counts %d s, want 9000", week2.CountedSeconds)
	}
	var planning Slot
	for _, s := range week2.Slots {
		if s.Kind == SlotRitual {
			planning = s
		}
	}
	if planning.Name != "Sprint Planning / Grooming" || planning.Start != "11:00" || planning.End != "17:30" || planning.SprintWeek != 2 {
		t.Errorf("ritual slot = %+v (it is clipped to the working day)", planning)
	}
	// Week 1 (Mon 23 Nov): counted in full.
	if d := dayOf(nov, "2026-11-23"); d.CountedSeconds != 8*3600 {
		t.Errorf("Monday 23 Nov counts %d s, want a full day", d.CountedSeconds)
	}
}

func TestMonth_WithoutAKnownSprintAWeekNActivityIsNeverGuessed(t *testing.T) {
	withSchedule(t)
	b := coreBoard()
	b.hasSprint = false
	m, err := newService(b).Month(context.Background(), "a@esb.co.id", "2026-11", 26)
	if err != nil {
		t.Fatal(err)
	}
	if m.Sprint != nil {
		t.Error("no sprint, none reported")
	}
	if d := dayOf(m, "2026-11-30"); d.CountedSeconds != 8*3600 {
		t.Errorf("Monday 30 Nov counts %d s, want 28800 with no sprint known", d.CountedSeconds)
	}
	// A failing sprint lookup must not fail the page.
	b.sprintErr = errors.New("jira down")
	if _, err := newService(b).Month(context.Background(), "a@esb.co.id", "2026-11", 26); err != nil {
		t.Errorf("a sprint lookup failure must not fail the month: %v", err)
	}
}

func TestMonth_TheWeeklyPatternListsTheBoardsSlots(t *testing.T) {
	withSchedule(t)
	m, _ := newService(coreBoard()).Month(context.Background(), "a@esb.co.id", "2026-12", 26)
	if len(m.Weekly.DSU) != 1 || m.Weekly.DSU[0].Start != "08:45" || m.Weekly.DSU[0].End != "09:15" || m.Weekly.DSUWeekdays[0] != int(time.Tuesday) {
		t.Errorf("DSU = %+v %v", m.Weekly.DSU, m.Weekly.DSUWeekdays)
	}
	if len(m.Weekly.Rituals) != 1 || m.Weekly.Rituals[0].SprintWeek != 2 || m.Weekly.Rituals[0].End != "17:30" || m.Weekly.RitualWeekdays[0] != int(time.Monday) {
		t.Errorf("rituals = %+v %v", m.Weekly.Rituals, m.Weekly.RitualWeekdays)
	}
	none, _ := newService(fakeBoards{}).Month(context.Background(), "a@esb.co.id", "2026-12", 0)
	if none.Weekly.DSU == nil || none.Weekly.Rituals == nil {
		t.Error("an empty pattern must be empty lists, not null")
	}
}

func TestMonth_RefusesABadMonthAndAnUnknownBoard(t *testing.T) {
	svc := newService(fakeBoards{found: false})
	for _, bad := range []string{"2026-13", "202612", "december", "2019-12", ""} {
		if _, err := svc.Month(context.Background(), "a@esb.co.id", bad, 0); !errors.Is(err, ErrInvalidMonth) {
			t.Errorf("month %q: err = %v, want ErrInvalidMonth", bad, err)
		}
	}
	if _, err := svc.Month(context.Background(), "a@esb.co.id", "2026-12", 99); !errors.Is(err, ErrBoardNotFound) {
		t.Errorf("unknown board: err = %v", err)
	}
}

// The calendar's constants and the time calculation must describe the same working day.
func TestWorkingDayConstantsMatchTheTimeCalculation(t *testing.T) {
	svc := business_hours_service.NewBusinessHoursService()
	at := func(min int) time.Time { return time.Date(2026, 12, 2, 0, min, 0, 0, wib) }
	cal := business_hours_dto.HolidayCalendar{}
	wantSeconds := int64((business_hours_dto.LunchStartMinute - business_hours_dto.WorkStartMinute + business_hours_dto.WorkEndMinute - business_hours_dto.LunchEndMinute) * 60)
	if got := svc.ElapsedBusinessSeconds(at(0), at(23*60+59), cal); got != wantSeconds {
		t.Errorf("a workday counts %d s, want %d: the constants drifted from the time calculation", got, wantSeconds)
	}
	edges := []struct {
		min    int
		inside bool
	}{
		{business_hours_dto.WorkStartMinute - 1, false}, {business_hours_dto.WorkStartMinute, true},
		{business_hours_dto.LunchStartMinute - 1, true}, {business_hours_dto.LunchStartMinute, false},
		{business_hours_dto.LunchEndMinute - 1, false}, {business_hours_dto.LunchEndMinute, true},
		{business_hours_dto.WorkEndMinute - 1, true}, {business_hours_dto.WorkEndMinute, false},
	}
	for _, e := range edges {
		if got := svc.IsWithinBusinessHours(at(e.min), cal); got != e.inside {
			t.Errorf("minute %d: inside = %v, want %v", e.min, got, e.inside)
		}
	}
}

// ---- HTTP ----

type fakeAPI struct {
	err      error
	gotEmail string
	gotMonth string
	gotBoard int64
}

func (f *fakeAPI) Month(_ context.Context, email, month string, board int64) (Month, error) {
	f.gotEmail, f.gotMonth, f.gotBoard = email, month, board
	return Month{Month: month, Days: []Day{}}, f.err
}

func serve(api *fakeAPI, query string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api/dashboard", func(c *gin.Context) {
		c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{UserID: 1, Email: "a@esb.co.id"})
	})
	UserRoutes(g, NewHandler(api))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/dashboard/calendar"+query, nil))
	return w
}

func TestHandler_PassesTheSessionUserMonthAndBoardAndMapsErrors(t *testing.T) {
	api := &fakeAPI{}
	w := serve(api, "?month=2026-12&board=26")
	if w.Code != http.StatusOK || api.gotEmail != "a@esb.co.id" || api.gotMonth != "2026-12" || api.gotBoard != 26 {
		t.Errorf("code=%d got %q %q %d", w.Code, api.gotEmail, api.gotMonth, api.gotBoard)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["month"] != "2026-12" {
		t.Errorf("body = %s", w.Body.String())
	}

	api = &fakeAPI{}
	serve(api, "")
	if !strings.HasPrefix(api.gotMonth, "20") || api.gotBoard != 0 {
		t.Errorf("defaults: month %q board %d, want the current month and no board", api.gotMonth, api.gotBoard)
	}
	for _, c := range []struct {
		name  string
		err   error
		query string
		code  int
	}{
		{"bad month", ErrInvalidMonth, "?month=x", http.StatusBadRequest},
		{"unknown board", ErrBoardNotFound, "?board=99", http.StatusNotFound},
		{"storage", errors.New("db"), "", http.StatusInternalServerError},
		{"bad board id", nil, "?board=abc", http.StatusBadRequest},
		{"negative board id", nil, "?board=-1", http.StatusBadRequest},
	} {
		if w := serve(&fakeAPI{err: c.err}, c.query); w.Code != c.code {
			t.Errorf("%s: code = %d, want %d", c.name, w.Code, c.code)
		}
	}
}
