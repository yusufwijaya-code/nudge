package admin_roster

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"nudge/base/testutil"
	"nudge/modules/admin_roster/admin_roster_holder"
	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
)

const owner = "yusuf.wijaya@esb.co.id"

type fakeRepo struct {
	users   map[int64]string // userID -> email
	admins  map[int64]bool
	findErr error
	setErr  error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{users: map[int64]string{1: owner, 2: "keva.galih@esb.co.id", 3: "eren.dhoheiri@esb.co.id"}, admins: map[int64]bool{}}
}

func (f *fakeRepo) ListAdminEmails(context.Context) ([]string, error) {
	var out []string
	for id, on := range f.admins {
		if on {
			out = append(out, f.users[id])
		}
	}
	return out, nil
}
func (f *fakeRepo) SetAdmin(_ context.Context, id int64, admin bool) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.admins[id] = admin
	return nil
}
func (f *fakeRepo) FindEmail(_ context.Context, id int64) (string, bool, error) {
	email, ok := f.users[id]
	return email, ok, f.findErr
}

func reset(t *testing.T) {
	t.Helper()
	admin_roster_holder.SetOwner(owner)
	admin_roster_holder.SetAdmins(nil)
	t.Cleanup(func() { admin_roster_holder.SetOwner(""); admin_roster_holder.SetAdmins(nil) })
}

func TestSetAdmin_OnlyTheOwnerPromotesAndTheChangeTakesEffectAtOnce(t *testing.T) {
	reset(t)
	repo := newFakeRepo()
	svc := NewService(repo)

	if err := svc.SetAdmin(context.Background(), owner, 2, true); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if !admin_roster_holder.IsAdmin("keva.galih@esb.co.id") {
		t.Error("a promoted user must be an admin at once")
	}
	if admin_roster_holder.IsOwner("keva.galih@esb.co.id") {
		t.Error("a promoted user is not the owner")
	}
	if err := svc.SetAdmin(context.Background(), owner, 2, false); err != nil {
		t.Fatalf("demote: %v", err)
	}
	if admin_roster_holder.IsAdmin("keva.galih@esb.co.id") {
		t.Error("a demoted user must lose the admin role at once")
	}
}

func TestSetAdmin_AnAdminWhoIsNotTheOwnerCannotPromoteOrDemote(t *testing.T) {
	reset(t)
	repo := newFakeRepo()
	svc := NewService(repo)
	_ = svc.SetAdmin(context.Background(), owner, 2, true)

	for _, target := range []int64{3, 2} {
		if err := svc.SetAdmin(context.Background(), "keva.galih@esb.co.id", target, true); !errors.Is(err, ErrOwnerOnly) {
			t.Errorf("target %d: err = %v, want ErrOwnerOnly", target, err)
		}
	}
	if repo.admins[3] {
		t.Error("nothing may be stored for a refused promotion")
	}
}

func TestSetAdmin_TheOwnerIsFixedAndUnknownUsersAreRefused(t *testing.T) {
	reset(t)
	repo := newFakeRepo()
	svc := NewService(repo)

	if err := svc.SetAdmin(context.Background(), owner, 1, false); !errors.Is(err, ErrOwnerFixed) {
		t.Errorf("demoting the owner: err = %v, want ErrOwnerFixed", err)
	}
	if err := svc.SetAdmin(context.Background(), "YUSUF.WIJAYA@ESB.CO.ID", 1, true); !errors.Is(err, ErrOwnerFixed) {
		t.Errorf("promoting the owner: err = %v, want ErrOwnerFixed", err)
	}
	if err := svc.SetAdmin(context.Background(), owner, 99, true); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("unknown user: err = %v, want ErrUserNotFound", err)
	}
	if !admin_roster_holder.IsAdmin(owner) {
		t.Error("the owner stays an admin")
	}
}

func TestLoad_PutsThePromotedAdminsInForce(t *testing.T) {
	reset(t)
	repo := newFakeRepo()
	repo.admins[3] = true
	if err := NewService(repo).Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !admin_roster_holder.IsAdmin("eren.dhoheiri@esb.co.id") || admin_roster_holder.IsAdmin("keva.galih@esb.co.id") {
		t.Error("only the stored admins are in force")
	}
}

// --- handler -----------------------------------------------------------

type fakeAPI struct {
	err    error
	gotID  int64
	gotAdm bool
	gotWho string
	called bool
}

func (f *fakeAPI) SetAdmin(_ context.Context, actor string, id int64, admin bool) error {
	f.called, f.gotWho, f.gotID, f.gotAdm = true, actor, id, admin
	return f.err
}

func serveRoster(api *fakeAPI, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	auth := func(c *gin.Context) {
		c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{UserID: 1, Email: owner})
	}
	AdminRoutes(r.Group("/api/dashboard/admin", auth), NewHandler(api))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/dashboard/admin/users/2/admin", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestHandler_MapsEveryOutcome(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
	}{
		{"promoted", nil, http.StatusOK},
		{"not the owner", ErrOwnerOnly, http.StatusForbidden},
		{"the owner itself", ErrOwnerFixed, http.StatusBadRequest},
		{"no such user", ErrUserNotFound, http.StatusNotFound},
		{"storage failed", errors.New("db down"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			api := &fakeAPI{err: c.err}
			w := serveRoster(api, `{"admin":true}`)
			if w.Code != c.code {
				t.Errorf("code = %d, want %d (%s)", w.Code, c.code, w.Body.String())
			}
			if api.gotWho != owner || api.gotID != 2 || !api.gotAdm {
				t.Errorf("service got %q / %d / %v", api.gotWho, api.gotID, api.gotAdm)
			}
		})
	}
	api := &fakeAPI{}
	if w := serveRoster(api, `{}`); w.Code != http.StatusBadRequest || api.called {
		t.Errorf("a body without admin: code=%d called=%v, want 400 and no change", w.Code, api.called)
	}
	if w := serveRoster(api, `nope`); w.Code != http.StatusBadRequest || api.called {
		t.Errorf("a body that is not JSON: code=%d", w.Code)
	}
}

// --- the owner guard ---------------------------------------------------

func guardedServer(actor string, lookup OwnerEmailLookup) (*gin.Engine, *int) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	reached := 0
	g := r.Group("/api/dashboard/admin", func(c *gin.Context) {
		c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{UserID: 2, Email: actor})
	})
	g.Use(ProtectOwner(lookup))
	ok := func(c *gin.Context) { reached++; c.Status(http.StatusOK) }
	g.GET("/users/:userID", ok)
	g.GET("/users/:userID/settings", ok)
	g.PUT("/users/:userID/settings/notification", ok)
	g.POST("/users/:userID/settings/mute", ok)
	g.DELETE("/users/:userID/credential", ok)
	g.DELETE("/users/:userID", ok)
	g.POST("/users/:userID/active", ok)
	g.POST("/users/:userID/jira-account-id", ok)
	g.POST("/broadcast", ok)
	return r, &reached
}

func lookupFrom(repo *fakeRepo) OwnerEmailLookup { return repo.FindEmail }

func call(r *gin.Engine, method, path string) int {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w.Code
}

func TestProtectOwner_AnAdminCannotChangeTheOwnersAccountInAnyWay(t *testing.T) {
	reset(t)
	r, reached := guardedServer("keva.galih@esb.co.id", lookupFrom(newFakeRepo()))
	for _, c := range [][2]string{
		{"PUT", "/api/dashboard/admin/users/1/settings/notification"},
		{"POST", "/api/dashboard/admin/users/1/settings/mute"},
		{"DELETE", "/api/dashboard/admin/users/1/credential"},
		{"DELETE", "/api/dashboard/admin/users/1"},
		{"POST", "/api/dashboard/admin/users/1/active"},
		{"POST", "/api/dashboard/admin/users/1/jira-account-id"},
	} {
		if code := call(r, c[0], c[1]); code != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403", c[0], c[1], code)
		}
	}
	if *reached != 0 {
		t.Errorf("%d handler(s) ran for a refused change", *reached)
	}
}

func TestProtectOwner_ReadingTheOwnersPageStaysAllowed(t *testing.T) {
	reset(t)
	r, _ := guardedServer("keva.galih@esb.co.id", lookupFrom(newFakeRepo()))
	for _, path := range []string{"/api/dashboard/admin/users/1", "/api/dashboard/admin/users/1/settings"} {
		if code := call(r, "GET", path); code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, code)
		}
	}
}

func TestProtectOwner_OtherUsersAndNonUserRoutesAreUntouched(t *testing.T) {
	reset(t)
	r, reached := guardedServer("keva.galih@esb.co.id", lookupFrom(newFakeRepo()))
	for _, c := range [][2]string{
		{"PUT", "/api/dashboard/admin/users/3/settings/notification"},
		{"DELETE", "/api/dashboard/admin/users/3"},
		{"POST", "/api/dashboard/admin/users/3/active"},
		{"POST", "/api/dashboard/admin/broadcast"},
		{"PUT", "/api/dashboard/admin/users/abc/settings/notification"}, // the handler answers 400 itself
	} {
		if code := call(r, c[0], c[1]); code != http.StatusOK {
			t.Errorf("%s %s = %d, want it to pass the guard", c[0], c[1], code)
		}
	}
	if *reached != 5 {
		t.Errorf("handlers reached = %d, want 5", *reached)
	}
}

func TestProtectOwner_TheOwnerCanChangeHisOwnAccount(t *testing.T) {
	reset(t)
	r, _ := guardedServer("YUSUF.WIJAYA@ESB.CO.ID", lookupFrom(newFakeRepo()))
	if code := call(r, "PUT", "/api/dashboard/admin/users/1/settings/notification"); code != http.StatusOK {
		t.Errorf("the owner changing his own settings = %d, want 200", code)
	}
}

// A guard that cannot tell who the target is must not let the change through.
func TestProtectOwner_FailsClosedWhenTheTargetCannotBeLooked(t *testing.T) {
	reset(t)
	repo := newFakeRepo()
	repo.findErr = errors.New("db down")
	r, reached := guardedServer("keva.galih@esb.co.id", lookupFrom(repo))
	if code := call(r, "DELETE", "/api/dashboard/admin/users/3"); code != http.StatusInternalServerError {
		t.Errorf("lookup failure = %d, want 500", code)
	}
	if *reached != 0 {
		t.Error("the change must not run when the target is unknown")
	}
}

func TestProtectOwner_MatchesTheOwnerWhateverTheLetterCase(t *testing.T) {
	reset(t)
	repo := newFakeRepo()
	repo.users[1] = "Yusuf.Wijaya@ESB.co.id"
	r, _ := guardedServer("keva.galih@esb.co.id", lookupFrom(repo))
	if code := call(r, "DELETE", "/api/dashboard/admin/users/1"); code != http.StatusForbidden {
		t.Errorf("owner stored with other letter case = %d, want 403", code)
	}
}

// --- the real table ----------------------------------------------------

func TestRepository_ReadsAndWritesTheAdminFlagOfRealRows(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	email := "testmysql-roster@esb.co.id"
	res, err := db.Exec(`INSERT INTO ms_user (userName, email, flagActive, createdBy) VALUES ('TestMySQL Roster', ?, 1, 'test')`, email)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	t.Cleanup(func() { db.Exec(`DELETE FROM ms_user WHERE userID = ?`, id) })

	if got, found, err := repo.FindEmail(ctx, id); err != nil || !found || got != email {
		t.Fatalf("FindEmail = %q %v %v", got, found, err)
	}
	if _, found, _ := repo.FindEmail(ctx, -1); found {
		t.Error("an unknown id must not be found")
	}
	has := func() bool {
		emails, err := repo.ListAdminEmails(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range emails {
			if e == email {
				return true
			}
		}
		return false
	}
	if has() {
		t.Error("a new user is not an admin")
	}
	if err := repo.SetAdmin(ctx, id, true); err != nil || !has() {
		t.Errorf("after promoting: err=%v listed=%v", err, has())
	}
	if err := repo.SetAdmin(ctx, id, false); err != nil || has() {
		t.Errorf("after demoting: err=%v listed=%v", err, has())
	}
}
