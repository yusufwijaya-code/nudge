package jira_account

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
)

type fakeAPI struct {
	status    Status
	testWho   Identity
	testErr   error
	saveErr   error
	resetErr  error
	gotEmail  string
	gotToken  string
	gotEditor string
}

func (f *fakeAPI) Status(context.Context) (Status, error) { return f.status, nil }
func (f *fakeAPI) Test(context.Context) (Identity, error) { return f.testWho, f.testErr }
func (f *fakeAPI) Save(_ context.Context, email, token, editor string) (Identity, error) {
	f.gotEmail, f.gotToken, f.gotEditor = email, token, editor
	return Identity{DisplayName: "Bot"}, f.saveErr
}
func (f *fakeAPI) Reset(context.Context) error { return f.resetErr }

func serve(api *fakeAPI, method, path, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	auth := func(c *gin.Context) {
		c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{UserID: 1, Email: "admin@esb.co.id"})
	}
	AdminRoutes(r.Group("/api/dashboard/admin", auth), NewHandler(api))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestStatus_ReturnsTheAccountWithoutAnyToken(t *testing.T) {
	api := &fakeAPI{status: Status{Source: "env", Email: "yusuf.wijaya@esb.co.id", TokenHint: "••••••••abcd", EncryptionReady: true}}
	w := serve(api, http.MethodGet, "/api/dashboard/admin/jira-account", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d", w.Code)
	}
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["source"] != "env" || got["email"] != "yusuf.wijaya@esb.co.id" || got["tokenHint"] != "••••••••abcd" {
		t.Errorf("body = %v", got)
	}
	for k := range got {
		if strings.Contains(strings.ToLower(k), "token") && k != "tokenHint" {
			t.Errorf("response exposes a %q field", k)
		}
	}
}

func TestSave_PassesTheEditorAndMapsEveryOutcome(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
	}{
		{"saved", nil, http.StatusOK},
		{"empty input", ErrInvalidInput, http.StatusBadRequest},
		{"no encryption key", ErrEncryptionKeyNotConfigured, http.StatusServiceUnavailable},
		{"Jira says no", ErrRejected, http.StatusUnprocessableEntity},
		{"Jira unreachable", context.DeadlineExceeded, http.StatusBadGateway},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			api := &fakeAPI{saveErr: c.err}
			w := serve(api, http.MethodPut, "/api/dashboard/admin/jira-account", `{"email":"bot@esb.co.id","apiToken":"tok"}`)
			if w.Code != c.code {
				t.Errorf("code = %d, want %d (%s)", w.Code, c.code, w.Body.String())
			}
			if api.gotEmail != "bot@esb.co.id" || api.gotToken != "tok" || api.gotEditor != "admin@esb.co.id" {
				t.Errorf("service got %q / %q / %q", api.gotEmail, api.gotToken, api.gotEditor)
			}
			if strings.Contains(w.Body.String(), "tok\"") && c.err != nil {
				t.Errorf("an error response must not echo the token: %s", w.Body.String())
			}
		})
	}
}

func TestSave_RejectsABodyThatIsNotJSON(t *testing.T) {
	if w := serve(&fakeAPI{}, http.MethodPut, "/api/dashboard/admin/jira-account", `not json`); w.Code != http.StatusBadRequest {
		t.Errorf("code = %d", w.Code)
	}
}

// A Jira-side problem is data for the page, not an HTTP error.
func TestTest_ReportsTheAccountOrTheReason(t *testing.T) {
	ok := serve(&fakeAPI{testWho: Identity{DisplayName: "Yusuf Wijaya", EmailAddress: "yusuf.wijaya@esb.co.id", Active: true}}, http.MethodPost, "/api/dashboard/admin/jira-account/test", "")
	if ok.Code != http.StatusOK || !strings.Contains(ok.Body.String(), `"ok":true`) || !strings.Contains(ok.Body.String(), "Yusuf Wijaya") {
		t.Errorf("ok response = %d %s", ok.Code, ok.Body.String())
	}
	bad := serve(&fakeAPI{testErr: ErrRejected}, http.MethodPost, "/api/dashboard/admin/jira-account/test", "")
	if bad.Code != http.StatusOK || !strings.Contains(bad.Body.String(), `"ok":false`) || !strings.Contains(bad.Body.String(), "menolak") {
		t.Errorf("rejected response = %d %s", bad.Code, bad.Body.String())
	}
}

func TestReset_ReturnsNoContent(t *testing.T) {
	if w := serve(&fakeAPI{}, http.MethodDelete, "/api/dashboard/admin/jira-account", ""); w.Code != http.StatusNoContent {
		t.Errorf("code = %d", w.Code)
	}
}
