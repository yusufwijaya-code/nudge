package auto_done

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/business_hours/business_hours_service"
	"nudge/modules/issue_session/issue_session_dto"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/move_fields"
	"nudge/modules/user_dashboard/user_dashboard_dto"
	"nudge/modules/user_dashboard/user_dashboard_service"
)

var wib = time.FixedZone("WIB", 7*3600)

type fakeStore struct {
	enabled  map[string]bool
	claims   map[string]bool
	finished map[string]string
	set      map[int64]bool
}

func (f *fakeStore) EnabledEmails(context.Context) (map[string]bool, error) { return f.enabled, nil }
func (f *fakeStore) IsEnabled(_ context.Context, id int64) (bool, error)    { return f.set[id], nil }
func (f *fakeStore) SetEnabled(_ context.Context, id int64, on bool) error {
	f.set[id] = on
	return nil
}
func (f *fakeStore) Claim(_ context.Context, key string, run time.Time, _, _ string) (bool, error) {
	k := key + "|" + run.String()
	if f.claims[k] {
		return false, nil
	}
	f.claims[k] = true
	return true, nil
}
func (f *fakeStore) Finish(_ context.Context, key string, _ time.Time, outcome, msg string) error {
	f.finished[key] = outcome + ":" + msg
	return nil
}

type fakeSessions struct {
	list []issue_session_dto.IssueSession
}

func (f fakeSessions) GetActiveSessions(context.Context) ([]issue_session_dto.IssueSession, error) {
	return f.list, nil
}

type fakeCreds struct{ none bool }

func (f fakeCreds) ResolveCredential(context.Context, string) (*jira_client_dto.Credential, error) {
	if f.none {
		return nil, nil
	}
	return &jira_client_dto.Credential{Email: "a@esb.co.id", APIToken: "t"}, nil
}

type fakeDashboard struct {
	current     string
	transitions []user_dashboard_dto.TransitionOption
	listErr     error
	moveErr     error
	listed      int
	moved       []string
}

func (f *fakeDashboard) ListTransitions(context.Context, string, string) (user_dashboard_dto.TicketTransitions, error) {
	f.listed++
	return user_dashboard_dto.TicketTransitions{CurrentStatus: f.current, Transitions: f.transitions}, f.listErr
}
func (f *fakeDashboard) Transition(_ context.Context, _, key, id string) (string, error) {
	if f.moveErr != nil {
		return "", f.moveErr
	}
	f.moved = append(f.moved, key+":"+id)
	return "Code Completed", nil
}

type fakeNotifier struct{ texts []string }

func (f *fakeNotifier) SendPlainTextDM(_ context.Context, _ string, text string) error {
	f.texts = append(f.texts, text)
	return nil
}

// The time a ticket has used comes from AccumulatedSeconds (see used), so the tests do not
// depend on the wall clock; `now` only names a start time the sessions need.
var now = time.Date(2026, 11, 26, 11, 0, 0, 0, wib)

type rig struct {
	store *fakeStore
	dash  *fakeDashboard
	note  *fakeNotifier
	svc   *Service
}

func sessionAt(key string, startedAt time.Time, sp float64) issue_session_dto.IssueSession {
	return issue_session_dto.IssueSession{
		IssueKey: key, ProjectKey: "CORE", StoryPoints: sp, FlagActive: true, AssigneeEmail: "dev@esb.co.id", IssueSummary: "Summary " + key,
		CurrentSessionStartedDate: &startedAt,
	}
}

func newRig(sessions []issue_session_dto.IssueSession) *rig {
	r := &rig{
		store: &fakeStore{enabled: map[string]bool{"dev@esb.co.id": true}, claims: map[string]bool{}, finished: map[string]string{}, set: map[int64]bool{}},
		dash: &fakeDashboard{current: "Code Process", transitions: []user_dashboard_dto.TransitionOption{
			{ID: "11", ToStatus: "Code Review"}, {ID: "31", ToStatus: "Code Completed"},
		}},
		note: &fakeNotifier{},
	}
	r.svc = NewService(Deps{
		Store: r.store, Sessions: fakeSessions{sessions}, Creds: fakeCreds{}, Dashboard: r.dash, Notifier: r.note,
		Calendar: allDays{}, Hours: business_hours_service.NewBusinessHoursService(),
		RuleFor: func(string) jira_client_dto.TrackingRule {
			return jira_client_dto.TrackingRule{ProjectKey: "CORE", TrackedStatuses: []string{"Code Process", "CODE REVIEW"}, DoneStatus: "Code Completed", ReviewStatus: "CODE REVIEW"}
		},
	})
	return r
}

// allDays is a calendar without holidays or leave.
type allDays struct{}

func (allDays) ForEmail(context.Context, string) business_hours_dto.HolidayCalendar {
	return business_hours_dto.HolidayCalendar{}
}

// used makes the ticket's run already finished with the given seconds counted.
func used(sess issue_session_dto.IssueSession, seconds int64) issue_session_dto.IssueSession {
	sess.CurrentSessionStartedDate = nil
	sess.AccumulatedSeconds = seconds
	return sess
}

func TestRunOnce_MovesATicketWhoseTimeIsUpToTheProjectsDoneStatus(t *testing.T) {
	r := newRig([]issue_session_dto.IssueSession{
		used(sessionAt("CORE-1", now, 2), 7200),
		used(sessionAt("CORE-2", now, 2), 3600),
	})
	if n := r.svc.RunOnce(context.Background()); n != 1 {
		t.Fatalf("moved = %d, want 1 (only the ticket at its budget)", n)
	}
	if len(r.dash.moved) != 1 || r.dash.moved[0] != "CORE-1:31" {
		t.Fatalf("moved = %v, want CORE-1 through the transition to Code Completed", r.dash.moved)
	}
	if !strings.HasPrefix(r.store.finished["CORE-1"], "moved:") {
		t.Fatalf("log = %v", r.store.finished)
	}
	if len(r.note.texts) != 1 || !strings.Contains(r.note.texts[0], "CORE-1") || !strings.Contains(r.note.texts[0], "Code Completed") {
		t.Fatalf("DM = %v", r.note.texts)
	}
}

func TestRunOnce_UsesEachProjectsOwnDoneStatus(t *testing.T) {
	r := newRig([]issue_session_dto.IssueSession{used(sessionAt("OMS-1", now, 1), 3600)})
	r.svc.d.RuleFor = func(string) jira_client_dto.TrackingRule {
		return jira_client_dto.TrackingRule{ProjectKey: "OMS", TrackedStatuses: []string{"In Progress"}, DoneStatus: "Done"}
	}
	r.dash.current = "In Progress"
	r.dash.transitions = []user_dashboard_dto.TransitionOption{{ID: "5", ToStatus: "Code Completed"}, {ID: "7", ToStatus: "Done"}}
	if n := r.svc.RunOnce(context.Background()); n != 1 || r.dash.moved[0] != "OMS-1:7" {
		t.Fatalf("moved = %d %v, want the transition to this project's Done (7)", n, r.dash.moved)
	}
}

func TestRunOnce_OnlyForUsersWhoSwitchedItOn(t *testing.T) {
	r := newRig([]issue_session_dto.IssueSession{used(sessionAt("CORE-1", now, 2), 7200)})
	r.store.enabled = map[string]bool{}
	if n := r.svc.RunOnce(context.Background()); n != 0 || r.dash.listed != 0 {
		t.Fatalf("moved = %d, Jira asked %d times", n, r.dash.listed)
	}
}

func TestRunOnce_NeedsThePersonalJiraKey(t *testing.T) {
	r := newRig([]issue_session_dto.IssueSession{used(sessionAt("CORE-1", now, 2), 7200)})
	r.svc.d.Creds = fakeCreds{none: true}
	if n := r.svc.RunOnce(context.Background()); n != 0 || r.dash.listed != 0 || len(r.store.claims) != 0 {
		t.Fatalf("moved = %d, listed = %d, claims = %v: without a personal key nothing may happen", n, r.dash.listed, r.store.claims)
	}
}

func TestRunOnce_LeavesATicketInReviewOrAlreadyDone(t *testing.T) {
	for _, status := range []string{"CODE REVIEW", "Code Completed", "To Do"} {
		r := newRig([]issue_session_dto.IssueSession{used(sessionAt("CORE-1", now, 2), 7200)})
		r.dash.current = status
		if n := r.svc.RunOnce(context.Background()); n != 0 || len(r.dash.moved) != 0 || len(r.store.claims) != 0 {
			t.Fatalf("%s: moved = %d %v claims = %v", status, n, r.dash.moved, r.store.claims)
		}
	}
}

func TestRunOnce_AttemptsOncePerRunAndDoesNotAskJiraEveryCycle(t *testing.T) {
	r := newRig([]issue_session_dto.IssueSession{used(sessionAt("CORE-1", now, 2), 7200)})
	r.dash.current = "CODE REVIEW"
	r.svc.RunOnce(context.Background())
	r.svc.RunOnce(context.Background())
	if r.dash.listed != 1 {
		t.Fatalf("Jira asked %d times in two cycles, want 1 (looked at again only after a while)", r.dash.listed)
	}

	r2 := newRig([]issue_session_dto.IssueSession{used(sessionAt("CORE-1", now, 2), 7200)})
	r2.svc.RunOnce(context.Background())
	r2.svc.skip = map[string]time.Time{} // even when the in-memory memory is gone (a restart)…
	r2.svc.RunOnce(context.Background())
	if len(r2.dash.moved) != 1 {
		t.Fatalf("moved %d times, want once: the stored claim must stop a second attempt", len(r2.dash.moved))
	}
}

func TestRunOnce_ReportsWhyItCouldNotMove(t *testing.T) {
	cases := []struct {
		name string
		prep func(*fakeDashboard)
		want string
	}{
		{"no direct route", func(d *fakeDashboard) {
			d.transitions = []user_dashboard_dto.TransitionOption{{ID: "11", ToStatus: "Code Review"}}
		}, "tidak ada jalur langsung"},
		{"fields required", func(d *fakeDashboard) { d.moveErr = &move_fields.FieldsRequiredError{} }, "field yang belum terisi"},
		{"jira refuses", func(d *fakeDashboard) { d.moveErr = errors.New("boom") }, "Jira menolak"},
	}
	for _, c := range cases {
		r := newRig([]issue_session_dto.IssueSession{used(sessionAt("CORE-1", now, 2), 7200)})
		c.prep(r.dash)
		if n := r.svc.RunOnce(context.Background()); n != 0 {
			t.Fatalf("%s: moved = %d", c.name, n)
		}
		if !strings.HasPrefix(r.store.finished["CORE-1"], "failed:") || !strings.Contains(r.store.finished["CORE-1"], c.want) {
			t.Fatalf("%s: log = %v", c.name, r.store.finished)
		}
		if len(r.note.texts) != 1 || !strings.Contains(r.note.texts[0], "geser manual") {
			t.Fatalf("%s: DM = %v", c.name, r.note.texts)
		}
	}
}

func TestRunOnce_SkipsATicketThatIsNotTheUsers(t *testing.T) {
	r := newRig([]issue_session_dto.IssueSession{used(sessionAt("CORE-1", now, 2), 7200)})
	r.dash.listErr = user_dashboard_service.ErrNotOwnTicket
	if n := r.svc.RunOnce(context.Background()); n != 0 || len(r.store.claims) != 0 || len(r.note.texts) != 0 {
		t.Fatalf("moved = %d, claims = %v, DMs = %v", n, r.store.claims, r.note.texts)
	}
}

func TestSetEnabled_NeedsThePersonalKeyOnlyToTurnOn(t *testing.T) {
	r := newRig(nil)
	r.svc.d.Creds = fakeCreds{none: true}
	if _, err := r.svc.SetEnabled(context.Background(), 1, "dev@esb.co.id", true); !errors.Is(err, ErrCredentialRequired) || r.store.set[1] {
		t.Fatalf("err = %v, set = %v: turning on without a key must be refused", err, r.store.set)
	}
	if st, err := r.svc.SetEnabled(context.Background(), 1, "dev@esb.co.id", false); err != nil || st.Enabled {
		t.Fatalf("turning off must always work: %+v %v", st, err)
	}
	r.svc.d.Creds = fakeCreds{}
	st, err := r.svc.SetEnabled(context.Background(), 1, "dev@esb.co.id", true)
	if err != nil || !st.Enabled || !st.HasCredential || !st.Active {
		t.Fatalf("st = %+v, err = %v", st, err)
	}
	// The key is removed later: the choice is kept but the feature is not active.
	r.svc.d.Creds = fakeCreds{none: true}
	st, _ = r.svc.Status(context.Background(), 1, "dev@esb.co.id")
	if !st.Enabled || st.HasCredential || st.Active {
		t.Fatalf("st = %+v, want enabled but not active without a key", st)
	}
}

func serve(s *Service, method, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api/dashboard", func(c *gin.Context) {
		c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{UserID: 1, Email: "dev@esb.co.id"})
	})
	Routes(g, NewHandler(s))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, "/api/dashboard/settings/auto-done", strings.NewReader(body)))
	return w
}

func TestHandler_GetPutAndTheCredentialGate(t *testing.T) {
	r := newRig(nil)
	if w := serve(r.svc, http.MethodGet, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"hasCredential":true`) {
		t.Fatalf("get → %d %s", w.Code, w.Body.String())
	}
	if w := serve(r.svc, http.MethodPut, `{"enabled":true}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"active":true`) {
		t.Fatalf("put → %d %s", w.Code, w.Body.String())
	}
	if w := serve(r.svc, http.MethodPut, `{}`); w.Code != 400 {
		t.Fatalf("missing → %d", w.Code)
	}
	r.svc.d.Creds = fakeCreds{none: true}
	w := serve(r.svc, http.MethodPut, `{"enabled":true}`)
	if w.Code != http.StatusPreconditionFailed || !strings.Contains(w.Body.String(), "credential_required") || !strings.Contains(w.Body.String(), "Jira API Key Pribadi") {
		t.Fatalf("no key → %d %s", w.Code, w.Body.String())
	}
}
