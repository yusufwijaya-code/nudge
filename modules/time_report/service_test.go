package time_report

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/board/board_dto"
	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/registration/registration_dto"
	"nudge/modules/user_leave/user_leave_dto"
)

type fakeJira struct {
	mu        sync.Mutex
	issues    []jira_client_dto.Issue
	histories map[string][]jira_client_dto.ChangelogEntry
	gate      chan struct{} // when set, FullChangelog waits on it
	projects  []string
	from, to  string
	searchErr error
	badKey    string
}

func (f *fakeJira) IssuesCreatedBetween(_ context.Context, _ *jira_client_dto.Credential, _ string, projects []string, from, to string, max int) ([]jira_client_dto.Issue, bool, error) {
	f.mu.Lock()
	f.projects, f.from, f.to = projects, from, to
	f.mu.Unlock()
	return f.issues, false, f.searchErr
}
func (f *fakeJira) FullChangelog(ctx context.Context, _ *jira_client_dto.Credential, key string) ([]jira_client_dto.ChangelogEntry, error) {
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if key == f.badKey {
		return nil, errors.New("boom")
	}
	return f.histories[key], nil
}

type fakeUsers struct{ none bool }

func (f fakeUsers) FindUserByEmail(context.Context, string) (*registration_dto.User, bool, error) {
	if f.none {
		return nil, false, nil
	}
	return &registration_dto.User{UserName: "Dev One", Email: "dev@esb.co.id", JiraAccountID: "acc-1"}, true, nil
}

type fakeCreds struct{}

func (fakeCreds) ResolveCredential(context.Context, string) (*jira_client_dto.Credential, error) {
	return nil, nil
}

type fakeBoards struct{}

func (fakeBoards) Get(_ context.Context, id int64) (board_dto.Board, bool, error) {
	switch id {
	case 26:
		return board_dto.Board{BoardID: 26, Name: "ESB Core Board", ProjectKey: "CORE"}, true, nil
	case 40:
		return board_dto.Board{BoardID: 40, Name: "Unregistered", ProjectKey: "NEW"}, true, nil
	}
	return board_dto.Board{}, false, nil
}

type fakeLeaves struct{}

func (fakeLeaves) List(context.Context, string) ([]user_leave_dto.Leave, error) {
	return []user_leave_dto.Leave{{Date: "2026-11-25"}}, nil
}

type fakeSync struct{ err error }

func (f fakeSync) RunForEmail(context.Context, string) (int, int, error) { return 0, 0, f.err }

func newService(t *testing.T, j *fakeJira, sync Syncer) *Service {
	t.Helper()
	withSPRule(t)
	t.Cleanup(func() { jira_client_dto.SetRules(nil) })
	return NewService(Deps{
		Users: fakeUsers{}, Creds: fakeCreds{}, Jira: j, Boards: fakeBoards{}, Leaves: fakeLeaves{}, Sync: sync,
		National: business_hours_dto.HolidayCalendar{Holidays: map[string]bool{}},
		Cap:      func() float64 { return 12 }, JiraBaseURL: "https://jira.example/",
		ActiveProjects: func() []string { return []string{"CORE", "OMS"} },
		Now:            func() time.Time { return at(2026, 11, 27, 9, 0) },
	})
}

func waitDone(t *testing.T, s *Service, id string) Progress {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		p, err := s.Progress("dev@esb.co.id", id)
		if err != nil {
			t.Fatal(err)
		}
		if p.State != "running" {
			return p
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the report never finished")
	return Progress{}
}

func twoTickets() *fakeJira {
	return &fakeJira{
		issues: []jira_client_dto.Issue{issueWith("CORE-2", 4, at(2026, 11, 20, 9, 0)), issueWith("CORE-1", 2, at(2026, 11, 3, 9, 0)), issueWith("CORE-3", 0, at(2026, 11, 21, 9, 0))},
		histories: map[string][]jira_client_dto.ChangelogEntry{
			// CORE-1: Tue 24 Nov 09:00 → 11:00, 2 h of a 2 SP estimate: on target.
			"CORE-1": {change(at(2026, 11, 24, 9, 0), "To Do", "Code Process"), change(at(2026, 11, 24, 11, 0), "Code Process", "Code Completed")},
			// CORE-2: Tue 24 Nov 13:00 → Wed 25 Nov 17:30, but Wednesday is leave: 4.5 h of 4: deviation 12.5%.
			"CORE-2": {change(at(2026, 11, 24, 13, 0), "To Do", "Code Process"), change(at(2026, 11, 25, 17, 30), "Code Process", "Code Completed")},
			"CORE-3": {change(at(2026, 11, 24, 9, 0), "To Do", "Code Process"), change(at(2026, 11, 24, 10, 0), "Code Process", "Code Completed")},
		},
	}
}

func TestReport_ReadsJiraScoresEveryTicketAndExplainsTheRest(t *testing.T) {
	j := twoTickets()
	s := newService(t, j, fakeSync{})
	start, err := s.Start(context.Background(), "dev@esb.co.id", Params{From: "2026-11-01", To: "2026-11-30", BoardID: 26})
	if err != nil || start.State != "running" || len(start.Steps) != 5 {
		t.Fatalf("start = %+v, err = %v", start, err)
	}
	p := waitDone(t, s, start.ID)
	if p.State != "done" || p.Percent != 100 || p.Report == nil {
		t.Fatalf("progress = %+v", p)
	}
	r := p.Report
	if strings.Join(j.projects, ",") != "CORE" || j.from != "2026-11-01" || j.to != "2026-12-01" {
		t.Fatalf("Jira asked for %v %s..%s: the board's project, the end day included", j.projects, j.from, j.to)
	}
	if r.BoardName != "ESB Core Board" || r.UserName != "Dev One" || r.CapPercent != 12 {
		t.Fatalf("report = %+v", r)
	}
	if len(r.Tickets) != 2 || r.Tickets[0].Key != "CORE-1" || r.Tickets[1].Key != "CORE-2" {
		t.Fatalf("tickets = %+v (scored ones, oldest created first)", r.Tickets)
	}
	if !r.Tickets[0].Within || r.Tickets[0].CountedSeconds != 2*3600 {
		t.Fatalf("CORE-1 = %+v", r.Tickets[0])
	}
	if c2 := r.Tickets[1]; c2.CountedSeconds != 4*3600+1800 || dedOf(c2.Breakdown, CatLeave) == 0 || c2.Within {
		t.Fatalf("CORE-2 = %+v: the leave day must be taken off and 12.5%% is outside 12%%", c2)
	}
	if len(r.Excluded) != 1 || r.Excluded[0].Key != "CORE-3" || !strings.Contains(r.Excluded[0].Reason, "Story Point") || r.Excluded[0].URL != "https://jira.example/browse/CORE-3" {
		t.Fatalf("excluded = %+v", r.Excluded)
	}
	if r.Summary.TicketCount != 3 || r.Summary.ScoredCount != 2 || r.Summary.WithinCount != 1 || r.Summary.EstimateSeconds != 6*3600 {
		t.Fatalf("summary = %+v", r.Summary)
	}
}

func TestReport_EveryBoardUsesEveryRegisteredProject(t *testing.T) {
	j := twoTickets()
	s := newService(t, j, nil)
	start, _ := s.Start(context.Background(), "dev@esb.co.id", Params{From: "2026-11-01", To: "2026-11-30"})
	r := waitDone(t, s, start.ID).Report
	if strings.Join(j.projects, ",") != "CORE,OMS" || r.BoardName != "Semua board" {
		t.Fatalf("projects = %v board = %q", j.projects, r.BoardName)
	}
}

func TestReport_ASyncFailureIsAWarningNotAFailure(t *testing.T) {
	s := newService(t, twoTickets(), fakeSync{err: errors.New("jira down")})
	start, _ := s.Start(context.Background(), "dev@esb.co.id", Params{From: "2026-11-01", To: "2026-11-30"})
	p := waitDone(t, s, start.ID)
	if p.State != "done" || !strings.Contains(p.Report.SyncWarning, "Sinkron") {
		t.Fatalf("progress = %+v", p)
	}
}

func TestReport_AnUnreadableHistoryIsExcludedWithItsReason(t *testing.T) {
	j := twoTickets()
	j.badKey = "CORE-2"
	s := newService(t, j, nil)
	start, _ := s.Start(context.Background(), "dev@esb.co.id", Params{From: "2026-11-01", To: "2026-11-30"})
	r := waitDone(t, s, start.ID).Report
	found := false
	for _, e := range r.Excluded {
		if e.Key == "CORE-2" && strings.Contains(e.Reason, "tidak bisa dibaca") {
			found = true
		}
	}
	if !found || len(r.Tickets) != 1 {
		t.Fatalf("excluded = %+v tickets = %d", r.Excluded, len(r.Tickets))
	}
}

func TestReport_JiraSearchFailureFailsTheReportWithAFriendlyMessage(t *testing.T) {
	j := twoTickets()
	j.searchErr = errors.New("500")
	s := newService(t, j, nil)
	start, _ := s.Start(context.Background(), "dev@esb.co.id", Params{From: "2026-11-01", To: "2026-11-30"})
	p := waitDone(t, s, start.ID)
	if p.State != "failed" || !strings.Contains(p.Error, "Jira") || p.Report != nil {
		t.Fatalf("progress = %+v", p)
	}
	if _, err := s.Result("dev@esb.co.id", start.ID); !errors.Is(err, ErrJobNotFinished) {
		t.Fatalf("err = %v", err)
	}
}

func TestStart_ValidatesRangeAndBoard(t *testing.T) {
	s := newService(t, twoTickets(), nil)
	ctx := context.Background()
	for _, p := range []Params{
		{From: "2026-11-30", To: "2026-11-01"},
		{From: "2026-13-01", To: "2026-12-01"},
		{From: "2025-01-01", To: "2026-12-31"},
		{From: "", To: ""},
	} {
		if _, err := s.Start(ctx, "dev@esb.co.id", p); !errors.Is(err, ErrInvalidRange) {
			t.Errorf("%+v: err = %v", p, err)
		}
	}
	for _, id := range []int64{40, 999} {
		if _, err := s.Start(ctx, "dev@esb.co.id", Params{From: "2026-11-01", To: "2026-11-30", BoardID: id}); !errors.Is(err, ErrBoardUnknown) {
			t.Errorf("board %d: err = %v", id, err)
		}
	}
}

func TestJobs_ArePrivateOnePerUserAndCanBeCancelled(t *testing.T) {
	j := twoTickets()
	j.gate = make(chan struct{})
	s := newService(t, j, nil)
	ctx := context.Background()
	first, _ := s.Start(ctx, "dev@esb.co.id", Params{From: "2026-11-01", To: "2026-11-30"})
	again, _ := s.Start(ctx, "dev@esb.co.id", Params{From: "2026-11-01", To: "2026-11-30"})
	if again.ID != first.ID {
		t.Fatal("a second start while one runs must return the running one")
	}
	if _, err := s.Progress("other@esb.co.id", first.ID); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("another user must not see the report: %v", err)
	}
	if err := s.Cancel("other@esb.co.id", first.ID); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("another user must not cancel it: %v", err)
	}
	if err := s.Cancel("dev@esb.co.id", first.ID); err != nil {
		t.Fatal(err)
	}
	p := waitDone(t, s, first.ID)
	if p.State != "cancelled" || p.Report != nil {
		t.Fatalf("progress = %+v", p)
	}
	if next, _ := s.Start(ctx, "dev@esb.co.id", Params{From: "2026-11-01", To: "2026-11-30"}); next.ID == first.ID {
		t.Fatal("after a cancel a new report starts")
	}
	close(j.gate)
}

func TestReport_NeedsALinkedJiraAccount(t *testing.T) {
	s := newService(t, twoTickets(), nil)
	s.d.Users = fakeUsers{none: true}
	start, _ := s.Start(context.Background(), "dev@esb.co.id", Params{From: "2026-11-01", To: "2026-11-30"})
	if p := waitDone(t, s, start.ID); p.State != "failed" || !strings.Contains(p.Error, "Akun Jira") {
		t.Fatalf("progress = %+v", p)
	}
}

func TestPercentOf_GrowsThroughTheSteps(t *testing.T) {
	steps := []Progress{
		{Step: 0}, {Step: 1}, {Step: 2, Total: 10, Done: 0}, {Step: 2, Total: 10, Done: 5}, {Step: 2, Total: 10, Done: 10}, {Step: 3}, {Step: 4}, {State: "done"},
	}
	last := -1
	for _, p := range steps {
		v := percentOf(p)
		if v < last || v > 100 {
			t.Fatalf("percent went from %d to %d at %+v", last, v, p)
		}
		last = v
	}
	if last != 100 {
		t.Fatalf("done = %d", last)
	}
}

func serveReport(s *Service, method, path, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api/dashboard", func(c *gin.Context) {
		c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{UserID: 1, Email: "dev@esb.co.id"})
	})
	Routes(g, NewHandler(s))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	return w
}

func TestHandler_StartPollAndDownloadThePDF(t *testing.T) {
	s := newService(t, twoTickets(), nil)
	w := serveReport(s, http.MethodPost, "/api/dashboard/reports/jobs", `{"from":"2026-11-01","to":"2026-11-30","boardId":26}`)
	if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), `"state":"running"`) {
		t.Fatalf("start → %d %s", w.Code, w.Body.String())
	}
	id := strings.Split(strings.Split(w.Body.String(), `"id":"`)[1], `"`)[0]
	if w := serveReport(s, http.MethodGet, "/api/dashboard/reports/jobs/"+id+"/pdf", ""); w.Code != http.StatusConflict && w.Code != http.StatusOK {
		t.Fatalf("early pdf → %d", w.Code)
	}
	waitDone(t, s, id)
	if w := serveReport(s, http.MethodGet, "/api/dashboard/reports/jobs/"+id, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"done"`) || !strings.Contains(w.Body.String(), `"tickets"`) {
		t.Fatalf("progress → %d %s", w.Code, w.Body.String())
	}
	pdf := serveReport(s, http.MethodGet, "/api/dashboard/reports/jobs/"+id+"/pdf", "")
	if pdf.Code != 200 || pdf.Header().Get("Content-Type") != "application/pdf" || !strings.HasPrefix(pdf.Body.String(), "%PDF-") ||
		!strings.Contains(pdf.Header().Get("Content-Disposition"), "Laporan-Nudge-Dev-One-2026-11-01_2026-11-30.pdf") {
		t.Fatalf("pdf → %d %v", pdf.Code, pdf.Header())
	}
	if w := serveReport(s, http.MethodPost, "/api/dashboard/reports/jobs", `{"from":"2026-11-30","to":"2026-11-01"}`); w.Code != 400 || !strings.Contains(w.Body.String(), "invalid_range") {
		t.Fatalf("bad range → %d %s", w.Code, w.Body.String())
	}
	if w := serveReport(s, http.MethodPost, "/api/dashboard/reports/jobs", `{"from":"2026-11-01","to":"2026-11-30","boardId":40}`); w.Code != 400 || !strings.Contains(w.Body.String(), "board_unknown") {
		t.Fatalf("unregistered board → %d %s", w.Code, w.Body.String())
	}
	if w := serveReport(s, http.MethodGet, "/api/dashboard/reports/jobs/nope", ""); w.Code != 404 {
		t.Fatalf("unknown → %d", w.Code)
	}
	if w := serveReport(s, http.MethodDelete, "/api/dashboard/reports/jobs/"+id, ""); w.Code != http.StatusNoContent {
		t.Fatalf("cancel → %d", w.Code)
	}
}
