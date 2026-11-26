package app_setting

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"nudge/modules/app_setting/app_setting_holder"
	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
)

type fakeRepo struct {
	row   *Row
	saved []string
}

func (f *fakeRepo) Get(context.Context, string) (*Row, error) { return f.row, nil }
func (f *fakeRepo) Set(_ context.Context, key, value, editor string) error {
	f.saved = append(f.saved, key+"="+value+"@"+editor)
	f.row = &Row{Value: value, UpdatedBy: editor, UpdatedDate: time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)}
	return nil
}

// newService keeps the cap in a local variable so tests never touch the shared holder.
func newService(repo *fakeRepo) (*Service, *float64) {
	cap := app_setting_holder.DefaultDeviationCapPercent
	s := NewService(repo)
	s.apply = func(v float64) {
		if v <= 0 {
			v = app_setting_holder.DefaultDeviationCapPercent
		}
		cap = v
	}
	s.current = func() float64 { return cap }
	return s, &cap
}

func TestDefaultIsTwelvePercent(t *testing.T) {
	s, cap := newService(&fakeRepo{})
	got, err := s.Get(context.Background())
	if err != nil || got.DeviationCapPercent != 12 || got.DefaultPercent != 12 || got.UpdatedDate != nil || *cap != 12 {
		t.Fatalf("got = %+v, err = %v", got, err)
	}
}

func TestSetDeviationCap_SavesAppliesAndRoundsToTwoDecimals(t *testing.T) {
	repo := &fakeRepo{}
	s, cap := newService(repo)
	got, err := s.SetDeviationCap(context.Background(), 15.256, "boss@esb.co.id")
	if err != nil {
		t.Fatal(err)
	}
	if *cap != 15.26 || got.DeviationCapPercent != 15.26 || got.UpdatedBy != "boss@esb.co.id" || got.UpdatedDate == nil {
		t.Fatalf("cap = %v, got = %+v", *cap, got)
	}
	if len(repo.saved) != 1 || repo.saved[0] != "deviation_cap_percent=15.26@boss@esb.co.id" {
		t.Fatalf("saved = %v", repo.saved)
	}
}

func TestSetDeviationCap_RejectsOutOfRangeValuesAndKeepsTheCap(t *testing.T) {
	repo := &fakeRepo{}
	s, cap := newService(repo)
	for _, v := range []float64{0, -5, 0.99, 100.01, 1e9} {
		_, err := s.SetDeviationCap(context.Background(), v, "boss@esb.co.id")
		if _, ok := err.(*ValidationError); !ok {
			t.Errorf("%v: err = %v, want a ValidationError", v, err)
		}
	}
	if len(repo.saved) != 0 || *cap != 12 {
		t.Fatalf("saved = %v, cap = %v: an invalid value must change nothing", repo.saved, *cap)
	}
	for _, v := range []float64{1, 100} {
		if _, err := s.SetDeviationCap(context.Background(), v, "boss@esb.co.id"); err != nil {
			t.Errorf("%v must be accepted: %v", v, err)
		}
	}
}

func TestLoad_AppliesTheStoredValueAndFallsBackOnABadOne(t *testing.T) {
	repo := &fakeRepo{row: &Row{Value: "20"}}
	s, cap := newService(repo)
	if err := s.Load(context.Background()); err != nil || *cap != 20 {
		t.Fatalf("cap = %v, err = %v", *cap, err)
	}
	repo.row = &Row{Value: "abc"}
	if err := s.Load(context.Background()); err != nil || *cap != 12 {
		t.Fatalf("a bad stored value must fall back to 12, cap = %v, err = %v", *cap, err)
	}
	repo.row = &Row{Value: "500"}
	_ = s.Load(context.Background())
	if *cap != 12 {
		t.Fatalf("an out-of-range stored value must fall back to 12, cap = %v", *cap)
	}
}

func TestHolder_DefaultThenSetThenReset(t *testing.T) {
	defer app_setting_holder.SetDeviationCapPercent(0)
	if app_setting_holder.DeviationCapPercent() != 12 {
		t.Fatal("default must be 12")
	}
	app_setting_holder.SetDeviationCapPercent(7.5)
	if app_setting_holder.DeviationCapPercent() != 7.5 {
		t.Fatal("set value must be returned")
	}
	app_setting_holder.SetDeviationCapPercent(-1)
	if app_setting_holder.DeviationCapPercent() != 12 {
		t.Fatal("a non-positive value must put the default back")
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

func TestHandler_GetAndPut(t *testing.T) {
	s, cap := newService(&fakeRepo{})
	if w := serve(s, http.MethodGet, "/api/app-settings", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"deviationCapPercent":12`) {
		t.Fatalf("get → %d %s", w.Code, w.Body.String())
	}
	w := serve(s, http.MethodPut, "/api/app-settings/deviation", `{"deviationCapPercent":18}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"deviationCapPercent":18`) || *cap != 18 {
		t.Fatalf("put → %d %s, cap = %v", w.Code, w.Body.String(), *cap)
	}
	if w := serve(s, http.MethodPut, "/api/app-settings/deviation", `{"deviationCapPercent":0}`); w.Code != 400 || !strings.Contains(w.Body.String(), "antara 1% dan 100%") {
		t.Fatalf("invalid → %d %s", w.Code, w.Body.String())
	}
	if w := serve(s, http.MethodPut, "/api/app-settings/deviation", `{}`); w.Code != 400 {
		t.Fatalf("missing value → %d", w.Code)
	}
}
