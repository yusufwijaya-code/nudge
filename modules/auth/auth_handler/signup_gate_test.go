package auth_handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/registration/registration_dto"
)

type fakeGate struct {
	isNew   bool
	err     error
	begun   *auth_dto.GoogleClaims
	forPath string
}

func (f *fakeGate) IsNew(context.Context, string) (bool, error) { return f.isNew, f.err }
func (f *fakeGate) Begin(_ *gin.Context, claims auth_dto.GoogleClaims) string {
	f.begun = &claims
	return f.forPath
}

func callbackWith(t *testing.T, svc *fakeAuthServiceForHandler, gate *fakeGate) *httptest.ResponseRecorder {
	t.Helper()
	handler := NewAuthHandler(svc, testCfg(), WithSignupGate(gate))
	w, c := newTestContext()
	req := httptest.NewRequest("GET", "/auth/google/callback?code=abc&state=s1", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: "s1"})
	c.Request = req
	handler.Callback(c)
	return w
}

// "Masuk" must not register: a visitor with no account is sent to the
// registration page and gets no session and no account.
func TestCallback_NewVisitorGoesToRegistrationInsteadOfBeingRegistered(t *testing.T) {
	svc := &fakeAuthServiceForHandler{
		verifyClaims: &auth_dto.GoogleClaims{Email: "new@esb.co.id", EmailVerified: true, Name: "New"},
		// GetOrCreateUser would register; it must never be reached.
		getOrCreateErr: context.DeadlineExceeded,
		issuedToken:    "tok",
	}
	gate := &fakeGate{isNew: true, forPath: "/register"}
	w := callbackWith(t, svc, gate)

	if w.Code != http.StatusFound || w.Header().Get("Location") != "/nudge/register" {
		t.Fatalf("→ %d %q, want 302 to /nudge/register", w.Code, w.Header().Get("Location"))
	}
	if gate.begun == nil || gate.begun.Email != "new@esb.co.id" {
		t.Fatalf("the verified identity was not handed to the signup step: %+v", gate.begun)
	}
	for _, ck := range w.Result().Cookies() {
		if ck.Name == sessionCookieName {
			t.Fatal("a visitor without an account must not get a session")
		}
	}
}

func TestCallback_ExistingUserStillLogsInWithTheGate(t *testing.T) {
	svc := &fakeAuthServiceForHandler{
		verifyClaims:    &auth_dto.GoogleClaims{Email: "dev@esb.co.id", EmailVerified: true, Name: "Dev"},
		getOrCreateUser: &registration_dto.User{UserID: 5, Email: "dev@esb.co.id", FlagActive: true},
		issuedToken:     "session-token-xyz",
	}
	w := callbackWith(t, svc, &fakeGate{isNew: false})
	if w.Header().Get("Location") != "/nudge/dashboard" {
		t.Fatalf("Location = %q, want /nudge/dashboard", w.Header().Get("Location"))
	}
}

func TestCallback_GateFailureShowsALoginError(t *testing.T) {
	svc := &fakeAuthServiceForHandler{verifyClaims: &auth_dto.GoogleClaims{Email: "dev@esb.co.id", EmailVerified: true}}
	w := callbackWith(t, svc, &fakeGate{err: context.DeadlineExceeded})
	if msg := loginErrorFrom(t, w); msg == "" {
		t.Fatal("want an error message on the login screen")
	}
}
