package signup

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
	"nudge/modules/registration/registration_dto"
	"nudge/modules/registration/registration_service"
)

func TestTokens_RoundTripExpiryAndForgery(t *testing.T) {
	tk := NewTokens("secret")
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	tk.now = func() time.Time { return now }
	v := tk.Sign(Pending{Email: "a@esb.co.id", Name: "A"}, 15*time.Minute)

	if p, ok := tk.Verify(v); !ok || p.Email != "a@esb.co.id" || p.Name != "A" {
		t.Fatalf("verify = %+v, %v", p, ok)
	}
	tk.now = func() time.Time { return now.Add(16 * time.Minute) }
	if _, ok := tk.Verify(v); ok {
		t.Fatal("an expired token must be rejected")
	}
	tk.now = func() time.Time { return now }
	payload, _, _ := strings.Cut(v, ".")
	if _, ok := tk.Verify(payload + ".AAAA"); ok {
		t.Fatal("a forged signature must be rejected")
	}
	if _, ok := NewTokens("other").Verify(v); ok {
		t.Fatal("a token signed with another key must be rejected")
	}
	if _, ok := tk.Verify("garbage"); ok {
		t.Fatal("garbage must be rejected")
	}
}

type fakes struct {
	calls     []string
	users     map[string]*registration_dto.User
	checkErr  error
	addErr    error
	jiraFound bool
	jiraErr   error
}

func (f *fakes) FindUserByEmail(_ context.Context, email string) (*registration_dto.User, bool, error) {
	u, ok := f.users[email]
	return u, ok, nil
}
func (f *fakes) Check(context.Context) error { f.calls = append(f.calls, "check"); return f.checkErr }
func (f *fakes) AddMember(context.Context, string) error {
	f.calls = append(f.calls, "add")
	return f.addErr
}
func (f *fakes) LookupUserByEmail(context.Context, string) (string, bool, error) {
	f.calls = append(f.calls, "jira")
	return "acc", f.jiraFound, f.jiraErr
}
func (f *fakes) Register(_ context.Context, _, email string) error {
	f.calls = append(f.calls, "register")
	f.users[email] = &registration_dto.User{UserID: 9, Email: email, FlagActive: true}
	return nil
}
func (f *fakes) MarkAdded(context.Context, string) error { f.calls = append(f.calls, "mark"); return nil }

func newFakes() *fakes { return &fakes{users: map[string]*registration_dto.User{}, jiraFound: true} }

func (f *fakes) svc(withGroup bool) *Service {
	d := Deps{Directory: f, Jira: f, Registrar: f, Store: f}
	if withGroup {
		d.Group = f
	}
	return NewService(d)
}

func (f *fakes) did(step string) bool { return strings.Contains(strings.Join(f.calls, ","), step) }

func TestRegister_GroupFirstThenAccount(t *testing.T) {
	f := newFakes()
	u, err := f.svc(true).Register(context.Background(), "Dev", "dev@esb.co.id")
	if err != nil || u == nil {
		t.Fatalf("err = %v", err)
	}
	if got := strings.Join(f.calls, ","); got != "check,jira,add,register,mark" {
		t.Fatalf("order = %s, want check,jira,add,register,mark: the account only after the group accepted", got)
	}
}

func TestRegister_DeadTokenCreatesNothing(t *testing.T) {
	f := newFakes()
	f.checkErr = errors.New("invalid_grant")
	_, err := f.svc(true).Register(context.Background(), "Dev", "dev@esb.co.id")
	if !errors.Is(err, ErrGroupUnavailable) {
		t.Fatalf("err = %v, want ErrGroupUnavailable", err)
	}
	if len(f.users) != 0 || f.did("register") || f.did("add") {
		t.Fatalf("calls = %v, users = %v: nothing may be created or added", f.calls, f.users)
	}
}

func TestRegister_FailedGroupAddCreatesNoAccount(t *testing.T) {
	f := newFakes()
	f.addErr = errors.New("HTTP 403")
	_, err := f.svc(true).Register(context.Background(), "Dev", "dev@esb.co.id")
	if !errors.Is(err, ErrGroupUnavailable) || len(f.users) != 0 {
		t.Fatalf("err = %v, users = %v", err, f.users)
	}
}

func TestRegister_NoJiraAccountIsNotAddedToTheGroup(t *testing.T) {
	f := newFakes()
	f.jiraFound = false
	_, err := f.svc(true).Register(context.Background(), "Dev", "dev@esb.co.id")
	if !errors.Is(err, registration_service.ErrJiraAccountNotFound) {
		t.Fatalf("err = %v", err)
	}
	if f.did("add") {
		t.Fatalf("calls = %v: a person without a Jira account must not join the group", f.calls)
	}
}

func TestRegister_WithoutGroupConfiguredSkipsTheGroup(t *testing.T) {
	f := newFakes()
	if _, err := f.svc(false).Register(context.Background(), "Dev", "dev@esb.co.id"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.calls, ","); got != "jira,register" {
		t.Fatalf("order = %s", got)
	}
}

func TestRegister_ExistingUserIsReturnedAsIs(t *testing.T) {
	f := newFakes()
	f.users["dev@esb.co.id"] = &registration_dto.User{UserID: 1, Email: "dev@esb.co.id", FlagActive: true}
	if _, err := f.svc(true).Register(context.Background(), "Dev", "dev@esb.co.id"); err != nil || len(f.calls) != 0 {
		t.Fatalf("err = %v, calls = %v", err, f.calls)
	}
	f.users["dev@esb.co.id"].FlagActive = false
	if _, err := f.svc(true).Register(context.Background(), "Dev", "dev@esb.co.id"); !errors.Is(err, ErrDeactivated) {
		t.Fatalf("err = %v, want ErrDeactivated", err)
	}
}

type fakeSessions struct{}

func (fakeSessions) IssueSession(context.Context, int64) (string, error) { return "sess-1", nil }

func newHandler(f *fakes, hook Hook) (*Handler, *gin.Engine) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(f.svc(true), NewTokens("k"), fakeSessions{}, "/nudge", "boss@esb.co.id", hook)
	r := gin.New()
	Routes(r.Group("/nudge"), h)
	return h, r
}

func pendingCookie(t *testing.T, h *Handler) *http.Cookie {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	if path := h.Begin(c, auth_dto.GoogleClaims{Email: "dev@esb.co.id", Name: "Dev", Picture: "pic"}); path != "/register" {
		t.Fatalf("Begin → %q", path)
	}
	for _, ck := range w.Result().Cookies() {
		if ck.Name == cookieName {
			return ck
		}
	}
	t.Fatal("no signup cookie")
	return nil
}

func do(r *gin.Engine, method, path string, ck *http.Cookie) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	if ck != nil {
		req.AddCookie(ck)
	}
	r.ServeHTTP(w, req)
	return w
}

func TestHandler_RegisterOpensASessionOnlyAfterTheAccountExists(t *testing.T) {
	f := newFakes()
	var hooked auth_dto.GoogleClaims
	h, r := newHandler(f, func(_ context.Context, _ int64, c auth_dto.GoogleClaims) { hooked = c })
	ck := pendingCookie(t, h)

	if w := do(r, "GET", "/nudge/api/signup/me", ck); w.Code != 200 || !strings.Contains(w.Body.String(), `"email":"dev@esb.co.id"`) {
		t.Fatalf("me → %d %s", w.Code, w.Body.String())
	}
	w := do(r, "POST", "/nudge/api/signup", ck)
	if w.Code != 200 {
		t.Fatalf("register → %d %s", w.Code, w.Body.String())
	}
	var session *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "session_token" {
			session = c
		}
	}
	if session == nil || session.Value != "sess-1" || session.Path != "/nudge" {
		t.Fatalf("session cookie = %+v", session)
	}
	if hooked.Email != "dev@esb.co.id" || hooked.Picture != "pic" {
		t.Fatalf("hook claims = %+v", hooked)
	}
}

func TestHandler_GroupFailureShowsAdminMessageAndNoSession(t *testing.T) {
	f := newFakes()
	f.checkErr = errors.New("invalid_grant")
	h, r := newHandler(f, nil)
	w := do(r, "POST", "/nudge/api/signup", pendingCookie(t, h))

	body := w.Body.String()
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(body, "group_unavailable") || !strings.Contains(body, "boss@esb.co.id") || !strings.Contains(body, "Hubungi admin") {
		t.Fatalf("→ %d %s", w.Code, body)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "session_token" {
			t.Fatal("no session may be opened when the group failed")
		}
	}
	if len(f.users) != 0 {
		t.Fatal("no account may be created when the group failed")
	}
}

func TestHandler_WithoutOrForgedCookieIsUnauthorized(t *testing.T) {
	_, r := newHandler(newFakes(), nil)
	if w := do(r, "POST", "/nudge/api/signup", nil); w.Code != 401 {
		t.Fatalf("no cookie → %d", w.Code)
	}
	if w := do(r, "GET", "/nudge/api/signup/me", &http.Cookie{Name: cookieName, Value: "x.y"}); w.Code != 401 {
		t.Fatalf("forged cookie → %d", w.Code)
	}
}
