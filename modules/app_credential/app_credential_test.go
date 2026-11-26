package app_credential

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"nudge/config"
	"nudge/modules/app_credential/app_credential_holder"
	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
)

type fakeRepo struct{ rows map[string]Row }

func newRepo() *fakeRepo { return &fakeRepo{rows: map[string]Row{}} }

func (f *fakeRepo) List(context.Context) (map[string]Row, error) {
	out := map[string]Row{}
	for k, v := range f.rows {
		out[k] = v
	}
	return out, nil
}
func (f *fakeRepo) Get(_ context.Context, k string) (*Row, error) {
	if r, ok := f.rows[k]; ok {
		return &r, nil
	}
	return nil, nil
}
func (f *fakeRepo) SaveValue(_ context.Context, k string, sealed []byte, exp *time.Time, by string) error {
	f.rows[k] = Row{Key: k, ValueEncrypted: sealed, ExpiresAt: exp, UpdatedBy: by, UpdatedDate: time.Now()}
	return nil
}
func (f *fakeRepo) SaveExpiry(_ context.Context, k string, exp *time.Time, by string) error {
	r := f.rows[k]
	r.Key, r.ExpiresAt, r.UpdatedBy, r.UpdatedDate = k, exp, by, time.Now()
	f.rows[k] = r
	return nil
}
func (f *fakeRepo) ClearValue(_ context.Context, k, by string) error {
	r := f.rows[k]
	r.ValueEncrypted, r.UpdatedBy = nil, by
	f.rows[k] = r
	return nil
}

var encKey = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

func reset() func() {
	app_credential_holder.SetGeminiAPIKey("")
	app_credential_holder.SetChatWebhookURL("")
	return func() {
		app_credential_holder.SetGeminiAPIKey("")
		app_credential_holder.SetChatWebhookURL("")
	}
}

func newSvc(t *testing.T, repo *fakeRepo, mod func(*config.AppConfig), checks Checks) *Service {
	t.Helper()
	cfg := &config.AppConfig{CredentialEncryptionKey: encKey}
	if mod != nil {
		mod(cfg)
	}
	return NewService(cfg, repo, checks)
}

func okGemini(context.Context, string) error { return nil }

func item(t *testing.T, s *Service, key string) Item {
	t.Helper()
	items, err := s.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Key == key {
			return it
		}
	}
	t.Fatalf("no item %s", key)
	return Item{}
}

func TestLoad_CarriesTheEnvironmentValuesIntoTheDatabaseOnce(t *testing.T) {
	defer reset()()
	repo := newRepo()
	s := newSvc(t, repo, func(c *config.AppConfig) {
		c.GeminiAPIKey = "env-gemini-key-1234"
		c.GoogleChatWebhookURL = "https://chat.googleapis.com/v1/spaces/X/messages?key=k&token=t"
	}, Checks{})
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if app_credential_holder.GeminiAPIKey() != "env-gemini-key-1234" || !strings.HasPrefix(app_credential_holder.ChatWebhookURL(), "https://chat.googleapis.com/") {
		t.Fatal("the carried-over values must be in force")
	}
	row := repo.rows[KeyGemini]
	if len(row.ValueEncrypted) == 0 || strings.Contains(string(row.ValueEncrypted), "env-gemini") || row.UpdatedBy != "env" {
		t.Fatalf("row = %+v: the value must be stored encrypted and marked as carried over", row)
	}
	gem := item(t, s, KeyGemini)
	if gem.Source != "database" || gem.Hint != "••••1234" || !gem.Configured || !gem.Editable {
		t.Fatalf("item = %+v", gem)
	}
}

func TestLoad_ADatabaseValueWinsOverTheEnvironmentAndARemovedOneStaysRemoved(t *testing.T) {
	defer reset()()
	repo := newRepo()
	s := newSvc(t, repo, func(c *config.AppConfig) { c.GeminiAPIKey = "env-key-aaaa" }, Checks{Gemini: okGemini})
	if _, err := s.SetValue(context.Background(), KeyGemini, "db-key-bbbb-1111", "", "boss@esb.co.id"); err != nil {
		t.Fatal(err)
	}
	app_credential_holder.SetGeminiAPIKey("")
	if err := s.Load(context.Background()); err != nil || app_credential_holder.GeminiAPIKey() != "db-key-bbbb-1111" {
		t.Fatalf("holder = %q, err = %v: the stored value must be put in force", app_credential_holder.GeminiAPIKey(), err)
	}
	if _, err := s.Clear(context.Background(), KeyGemini, "boss@esb.co.id"); err != nil {
		t.Fatal(err)
	}
	if err := s.Load(context.Background()); err != nil || app_credential_holder.GeminiAPIKey() != "" {
		t.Fatalf("holder = %q: removing must stick, the environment must not bring it back", app_credential_holder.GeminiAPIKey())
	}
	if it := item(t, s, KeyGemini); it.Configured || it.Source != "none" {
		t.Fatalf("item = %+v", it)
	}
}

func TestLoad_WithoutAnEncryptionKeyTheEnvironmentKeepsWorking(t *testing.T) {
	defer reset()()
	repo := newRepo()
	s := newSvc(t, repo, func(c *config.AppConfig) { c.CredentialEncryptionKey = ""; c.GeminiAPIKey = "env-key-aaaa" }, Checks{})
	if err := s.Load(context.Background()); err != nil || len(repo.rows) != 0 {
		t.Fatalf("err = %v rows = %v: nothing may be stored without the key", err, repo.rows)
	}
	if it := item(t, s, KeyGemini); it.Source != "env" || !it.Configured {
		t.Fatalf("item = %+v: it must still be reported from the environment", it)
	}
	if _, err := s.SetValue(context.Background(), KeyGemini, "new-key-12345", "", "boss@esb.co.id"); !errors.Is(err, ErrEncryptionKeyNotConfigured) {
		t.Fatalf("err = %v", err)
	}
}

func TestSetValue_ChecksTheGeminiKeyStoresItEncryptedAndAppliesAtOnce(t *testing.T) {
	defer reset()()
	repo := newRepo()
	var tried string
	s := newSvc(t, repo, nil, Checks{Gemini: func(_ context.Context, k string) error {
		tried = k
		if k == "bad-key-000000" {
			return errors.New("key ditolak Google (HTTP 400)")
		}
		return nil
	}})
	ctx := context.Background()
	if _, err := s.SetValue(ctx, KeyGemini, "bad-key-000000", "", "boss@esb.co.id"); err == nil || !strings.Contains(err.Error(), "tidak diterima") {
		t.Fatalf("err = %v, want a refusal that names Google's answer", err)
	}
	if len(repo.rows) != 0 || app_credential_holder.GeminiAPIKey() != "" {
		t.Fatal("a refused key must change nothing")
	}
	it, err := s.SetValue(ctx, KeyGemini, "  good-key-987654  ", "2027-05-01", "boss@esb.co.id")
	if err != nil || tried != "good-key-987654" {
		t.Fatalf("err = %v, tried = %q (the value must be trimmed)", err, tried)
	}
	if app_credential_holder.GeminiAPIKey() != "good-key-987654" || it.Hint != "••••7654" || it.ExpiresAt != "2027-05-01" || it.UpdatedBy != "boss@esb.co.id" {
		t.Fatalf("item = %+v, holder = %q", it, app_credential_holder.GeminiAPIKey())
	}
	if strings.Contains(string(repo.rows[KeyGemini].ValueEncrypted), "good-key") {
		t.Fatal("the value must be stored encrypted")
	}
}

func TestSetValue_ValidatesInputAndRefusesMonitoredOnes(t *testing.T) {
	defer reset()()
	s := newSvc(t, newRepo(), nil, Checks{Gemini: okGemini})
	ctx := context.Background()
	for _, bad := range []string{"", "short", "has space inside", strings.Repeat("x", maxSecretLen+1)} {
		var v *ValidationError
		if _, err := s.SetValue(ctx, KeyGemini, bad, "", "x"); !errors.As(err, &v) {
			t.Errorf("%q: err = %v, want a ValidationError", bad, err)
		}
	}
	var v *ValidationError
	if _, err := s.SetValue(ctx, KeyChatWebhook, "https://example.com/hook-abcdef", "", "x"); !errors.As(err, &v) || !strings.Contains(err.Error(), "chat.googleapis.com") {
		t.Fatalf("a webhook outside Google Chat must be refused, got %v", err)
	}
	if _, err := s.SetValue(ctx, KeyGemini, "valid-key-12345", "not-a-date", "x"); !errors.As(err, &v) {
		t.Fatalf("a bad date must be refused, got %v", err)
	}
	if _, err := s.SetValue(ctx, KeyGroupsToken, "whatever-12345", "", "x"); !errors.Is(err, ErrNotEditable) {
		t.Fatalf("a monitored credential cannot be set from the web: %v", err)
	}
	if _, err := s.SetValue(ctx, "nope", "whatever-12345", "", "x"); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("err = %v", err)
	}
	webhook := "https://chat.googleapis.com/v1/spaces/A/messages?key=k&token=t"
	if _, err := s.SetValue(ctx, KeyChatWebhook, webhook, "", "x"); err != nil || app_credential_holder.ChatWebhookURL() != webhook {
		t.Fatalf("a Google Chat webhook must be accepted and applied: %v", err)
	}
}

func TestSetExpiry_WorksForEveryCredentialAndFeedsTheBanner(t *testing.T) {
	defer reset()()
	repo := newRepo()
	s := newSvc(t, repo, nil, Checks{})
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.FixedZone("WIB", 7*3600))
	s.now = func() time.Time { return now }
	ctx := context.Background()
	for key, day := range map[string]string{
		KeyGroupsToken: "2026-11-30", // in 4 days: warns
		KeyChatAppSA:   "2026-11-20", // already past: warns
		KeyChatToken:   "2027-06-01", // far away: no warning
		KeyOAuthSecret: "2026-12-10", // in 14 days (inclusive): warns
		KeyGemini:      "2026-12-11", // in 15 days: no warning yet
	} {
		if _, err := s.SetExpiry(ctx, key, day, "boss@esb.co.id"); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
	}
	got, err := s.Expiring(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, e := range got {
		keys = append(keys, e.Key+"@"+e.ExpiresAt)
	}
	want := []string{KeyChatAppSA + "@2026-11-20", KeyGroupsToken + "@2026-11-30", KeyOAuthSecret + "@2026-12-10"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("expiring = %v, want %v (soonest first, past ones included)", keys, want)
	}
	if _, err := s.SetExpiry(ctx, KeyGroupsToken, "", "boss@esb.co.id"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Expiring(ctx)
	if len(got) != 2 {
		t.Fatalf("clearing a day must take it off the banner: %v", got)
	}
	var v *ValidationError
	if _, err := s.SetExpiry(ctx, KeyGroupsToken, "31/12/2026", "x"); !errors.As(err, &v) {
		t.Fatalf("err = %v", err)
	}
}

func TestMonitoredFilesAreReportedWithoutShowingSecrets(t *testing.T) {
	defer reset()()
	dir := t.TempDir()
	tokenOK := filepath.Join(dir, "groups.json")
	tokenNoRefresh := filepath.Join(dir, "chat.json")
	sa := filepath.Join(dir, "sa.json")
	_ = os.WriteFile(tokenOK, []byte(`{"access_token":"a","refresh_token":"SECRET-REFRESH"}`), 0o600)
	_ = os.WriteFile(tokenNoRefresh, []byte(`{"access_token":"a"}`), 0o600)
	_ = os.WriteFile(sa, []byte(`{"client_email":"bot@proj.iam.gserviceaccount.com","private_key":"-----BEGIN-----SECRET"}`), 0o600)
	s := newSvc(t, newRepo(), func(c *config.AppConfig) {
		c.GoogleGroupsTokenPath, c.GoogleOAuthTokenPath, c.GoogleChatAppCredentialsFile = tokenOK, tokenNoRefresh, sa
		c.GoogleOAuthClientSecret = "oauth-client-secret-wxyz"
	}, Checks{Groups: func(context.Context) error { return nil }})

	g := item(t, s, KeyGroupsToken)
	if !g.Configured || g.Source != "file" || g.Detail != "ada refresh token" || g.Hint != "groups.json" || !g.Testable || g.Editable {
		t.Fatalf("groups = %+v", g)
	}
	if c := item(t, s, KeyChatToken); c.Configured || !strings.Contains(c.Detail, "refresh token") {
		t.Fatalf("a token file without a refresh token is not healthy: %+v", c)
	}
	if a := item(t, s, KeyChatAppSA); !a.Configured || a.Detail != "bot@proj.iam.gserviceaccount.com" {
		t.Fatalf("service account = %+v", a)
	}
	if o := item(t, s, KeyOAuthSecret); !o.Configured || o.Source != "env" || o.Hint != "••••wxyz" {
		t.Fatalf("client secret = %+v", o)
	}
	items, _ := s.List(context.Background())
	for _, it := range items {
		if blob := it.Hint + it.Detail; strings.Contains(blob, "SECRET") || strings.Contains(blob, "oauth-client-secret") {
			t.Fatalf("%s leaks a secret: %q", it.Key, blob)
		}
	}
	if missing := item(t, newSvc(t, newRepo(), func(c *config.AppConfig) { c.GoogleChatAppCredentialsFile = filepath.Join(dir, "gone.json") }, Checks{}), KeyChatAppSA); missing.Configured || !strings.Contains(missing.Detail, "tidak ada") {
		t.Fatalf("a missing file must be reported: %+v", missing)
	}
}

func TestTest_ReportsEachCheckWithoutFailingTheRequest(t *testing.T) {
	defer reset()()
	ctx := context.Background()
	s := newSvc(t, newRepo(), func(c *config.AppConfig) { c.GeminiAPIKey = "env-key-1234" }, Checks{
		Gemini: func(context.Context, string) error { return errors.New("key ditolak Google (HTTP 400)") },
		Groups: func(context.Context) error { return errors.New("invalid_grant") },
	})
	if r, err := s.Test(ctx, KeyGemini); err != nil || r.OK || !strings.Contains(r.Message, "ditolak") {
		t.Fatalf("gemini = %+v %v", r, err)
	}
	if r, err := s.Test(ctx, KeyGroupsToken); err != nil || r.OK || !strings.Contains(r.Message, "invalid_grant") {
		t.Fatalf("groups = %+v %v", r, err)
	}
	if r, _ := s.Test(ctx, KeyOAuthSecret); r.OK || !strings.Contains(r.Message, "tidak bisa diuji") {
		t.Fatalf("secret = %+v", r)
	}
	ok := newSvc(t, newRepo(), func(c *config.AppConfig) { c.GeminiAPIKey = "env-key-1234" }, Checks{Gemini: okGemini, Groups: func(context.Context) error { return nil }})
	if r, _ := ok.Test(ctx, KeyGemini); !r.OK {
		t.Fatalf("gemini = %+v", r)
	}
	if r, _ := ok.Test(ctx, KeyGroupsToken); !r.OK {
		t.Fatalf("groups = %+v", r)
	}
	if r, _ := newSvc(t, newRepo(), nil, Checks{}).Test(ctx, KeyGemini); r.OK || !strings.Contains(r.Message, "Belum ada") {
		t.Fatalf("no key = %+v", r)
	}
	if r, _ := newSvc(t, newRepo(), nil, Checks{}).Test(ctx, KeyGroupsToken); r.OK || !strings.Contains(r.Message, "belum aktif") {
		t.Fatalf("groups off = %+v", r)
	}
}

func TestGeminiCheck_AcceptsOnly200AndSendsTheKeyInTheHeader(t *testing.T) {
	var gotHeader, gotURL string
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader, gotURL = r.Header.Get("x-goog-api-key"), r.URL.String()
		w.WriteHeader(status)
	}))
	defer srv.Close()
	check := geminiCheck(srv.Client(), srv.URL+"/v1beta/models?pageSize=1")
	if err := check(context.Background(), "the-key-1234"); err != nil || gotHeader != "the-key-1234" || strings.Contains(gotURL, "the-key") {
		t.Fatalf("err = %v, header = %q, url = %q: the key must be in the header only", err, gotHeader, gotURL)
	}
	for _, code := range []int{400, 401, 403} {
		status = code
		if err := check(context.Background(), "k"); err == nil || !strings.Contains(err.Error(), "ditolak") {
			t.Errorf("%d: err = %v", code, err)
		}
	}
	status = 500
	if err := check(context.Background(), "k"); err == nil || strings.Contains(err.Error(), "ditolak") {
		t.Errorf("500 is Google's trouble, not a rejected key: %v", err)
	}
}

func serve(s *Service, method, path, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api", func(c *gin.Context) {
		c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{UserID: 1, Email: "boss@esb.co.id"})
	})
	AdminRoutes(g, NewHandler(s))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	return w
}

func TestHandler_RoutesAndErrors(t *testing.T) {
	defer reset()()
	s := newSvc(t, newRepo(), func(c *config.AppConfig) { c.GeminiAPIKey = "env-key-1234" }, Checks{Gemini: okGemini})
	if w := serve(s, http.MethodGet, "/api/app-credentials", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"key":"gemini_api_key"`) || strings.Contains(w.Body.String(), "env-key-1234") {
		t.Fatalf("list → %d %s", w.Code, w.Body.String())
	}
	if w := serve(s, http.MethodGet, "/api/app-credentials/expiring", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatalf("expiring → %d %s", w.Code, w.Body.String())
	}
	if w := serve(s, http.MethodPut, "/api/app-credentials/gemini_api_key", `{"value":"fresh-key-55555","expiresAt":"2027-01-01"}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"hint":"••••5555"`) {
		t.Fatalf("put → %d %s", w.Code, w.Body.String())
	}
	if w := serve(s, http.MethodPut, "/api/app-credentials/groups_oauth_token", `{"value":"whatever-12345"}`); w.Code != 400 {
		t.Fatalf("monitored put → %d", w.Code)
	}
	if w := serve(s, http.MethodPut, "/api/app-credentials/nope/expiry", `{"expiresAt":"2027-01-01"}`); w.Code != 404 {
		t.Fatalf("unknown → %d", w.Code)
	}
	if w := serve(s, http.MethodPut, "/api/app-credentials/groups_oauth_token/expiry", `{"expiresAt":"2027-01-01"}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"expiresAt":"2027-01-01"`) {
		t.Fatalf("expiry → %d %s", w.Code, w.Body.String())
	}
	if w := serve(s, http.MethodPost, "/api/app-credentials/gemini_api_key/test", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("test → %d %s", w.Code, w.Body.String())
	}
	if w := serve(s, http.MethodDelete, "/api/app-credentials/gemini_api_key", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"configured":false`) {
		t.Fatalf("delete → %d %s", w.Code, w.Body.String())
	}
	if w := serve(s, http.MethodPut, "/api/app-credentials/gemini_api_key", `{bad`); w.Code != 400 {
		t.Fatalf("bad body → %d", w.Code)
	}
	noKey := newSvc(t, newRepo(), func(c *config.AppConfig) { c.CredentialEncryptionKey = "" }, Checks{Gemini: okGemini})
	if w := serve(noKey, http.MethodPut, "/api/app-credentials/gemini_api_key", `{"value":"fresh-key-55555"}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("no encryption key → %d", w.Code)
	}
}
