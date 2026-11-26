package onboarding

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/board/board_dto"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/jira_client/jira_client_service"
	"nudge/modules/registration/registration_dto"
	"nudge/modules/tracking_rule"
)

func str(s string) *string { return &s }

var core = tracking_rule.RuleView{
	ProjectKey: "CORE", IsActive: true,
	TrackedStatuses: []string{"Code Process", "CODE REVIEW"}, DoneStatus: "Code Completed", ReviewStatus: str("CODE REVIEW"),
	StoryPointFields:     []tracking_rule.FieldRef{{ID: "customfield_10016", Name: "Story Points"}, {ID: "customfield_10020", Name: "Story Point Estimate"}},
	StoryPointWriteField: "customfield_10020", StoryPointWriteFieldSubtask: "customfield_10016",
}

func TestSuggest_CopiesTheBestFittingRuleAndKeepsOnlyWhatTheProjectHas(t *testing.T) {
	l := tracking_rule.Lookup{
		Statuses:     []string{"To Do", "code process", "Code Completed"},
		NumberFields: []jira_client_dto.NumberField{{ID: "customfield_10016", Name: "Story Points"}},
	}
	other := tracking_rule.RuleView{ProjectKey: "OMS", IsActive: true, TrackedStatuses: []string{"In Progress"}, DoneStatus: "Done"}
	def, from := Suggest(l, []tracking_rule.RuleView{other, core})

	if from != "CORE" {
		t.Fatalf("template = %q, want CORE", from)
	}
	if strings.Join(def.TrackedStatuses, ",") != "code process" || def.DoneStatus != "Code Completed" {
		t.Fatalf("statuses = %v / %q (Jira's spelling, only those the project has)", def.TrackedStatuses, def.DoneStatus)
	}
	if def.ReviewStatus != "" {
		t.Fatalf("review = %q, want none: CODE REVIEW is not a status of this project", def.ReviewStatus)
	}
	if strings.Join(def.StoryPointFields, ",") != "customfield_10016" || def.StoryPointWriteField != "customfield_10016" || def.StoryPointWriteFieldSubtask != "customfield_10016" {
		t.Fatalf("SP = %v / %q / %q", def.StoryPointFields, def.StoryPointWriteField, def.StoryPointWriteFieldSubtask)
	}
}

func TestSuggest_NoFittingRuleStillGuessesStoryPointFields(t *testing.T) {
	l := tracking_rule.Lookup{
		Statuses:     []string{"Open", "Closed"},
		NumberFields: []jira_client_dto.NumberField{{ID: "customfield_1", Name: "Votes"}, {ID: "customfield_2", Name: "Story Points", StoryPointLike: true}},
	}
	def, from := Suggest(l, []tracking_rule.RuleView{core})
	if from != "" || len(def.TrackedStatuses) != 0 || def.DoneStatus != "" {
		t.Fatalf("def = %+v from %q, want nothing copied", def, from)
	}
	if strings.Join(def.StoryPointFields, ",") != "customfield_2" || def.StoryPointWriteField != "customfield_2" {
		t.Fatalf("SP = %+v", def)
	}
}

func TestSuggest_InactiveRulesAreNotTemplates(t *testing.T) {
	off := core
	off.IsActive = false
	_, from := Suggest(tracking_rule.Lookup{Statuses: []string{"Code Process", "Code Completed"}}, []tracking_rule.RuleView{off})
	if from != "" {
		t.Fatalf("template = %q", from)
	}
}

type fake struct {
	hits       []jira_client_service.OpenBoard
	discoverN  int
	tracked    map[string]bool
	created    []tracking_rule.RuleInput
	createErr  error
	syncErr    error
	synced     []string
	lookupN    int
	user       *registration_dto.User
	boardNames map[int64]board_dto.Board
}

func (f *fake) FindUserByEmail(context.Context, string) (*registration_dto.User, bool, error) {
	return f.user, f.user != nil, nil
}
func (f *fake) ResolveCredential(context.Context, string) (*jira_client_dto.Credential, error) {
	return nil, nil
}
func (f *fake) OpenBoardsOf(context.Context, *jira_client_dto.Credential, string) ([]jira_client_service.OpenBoard, error) {
	f.discoverN++
	return f.hits, nil
}
func (f *fake) EnsureKnown(_ context.Context, id int64) (board_dto.Board, error) {
	return f.boardNames[id], nil
}
func (f *fake) List(context.Context) ([]tracking_rule.RuleView, error) {
	return []tracking_rule.RuleView{core}, nil
}
func (f *fake) Lookup(_ context.Context, key string, _ bool) (tracking_rule.Lookup, error) {
	f.lookupN++
	return tracking_rule.Lookup{ProjectKey: key, ProjectName: "Proj " + key, Statuses: []string{"To Do", "Code Process", "Code Completed"},
		NumberFields: []jira_client_dto.NumberField{{ID: "customfield_10016", Name: "Story Points"}}}, nil
}
func (f *fake) Create(_ context.Context, in tracking_rule.RuleInput, _ string) (tracking_rule.RuleView, error) {
	if f.createErr != nil {
		return tracking_rule.RuleView{}, f.createErr
	}
	f.created = append(f.created, in)
	f.tracked[in.ProjectKey] = true
	return tracking_rule.RuleView{ProjectKey: in.ProjectKey}, nil
}
func (f *fake) RunForEmail(_ context.Context, email string) (int, int, error) {
	f.synced = append(f.synced, email)
	return 3, 1, f.syncErr
}

func newFake() *fake {
	return &fake{
		user:    &registration_dto.User{UserID: 1, Email: "dev@esb.co.id", JiraAccountID: "acc-1"},
		tracked: map[string]bool{"CORE": true},
		hits: []jira_client_service.OpenBoard{
			{BoardID: 26, ProjectKey: "CORE", SprintName: "Core Sprint 24", TicketCount: 2},
			{BoardID: 40, ProjectKey: "NEW", SprintName: "New Sprint 1", TicketCount: 1},
		},
		boardNames: map[int64]board_dto.Board{26: {BoardID: 26, Name: "ESB Core Board", ProjectKey: "CORE"}, 40: {BoardID: 40, Name: "New Board", ProjectKey: "NEW"}},
	}
}

func (f *fake) svc() *Service {
	return NewService(Deps{Users: f, Creds: f, Discover: f, Boards: f, Rules: f, Sync: f, IsTracked: func(k string) bool { return f.tracked[k] }})
}

func TestBoards_MarksWhichBoardsAreRegistered(t *testing.T) {
	f := newFake()
	boards, err := f.svc().Boards(context.Background(), "dev@esb.co.id", false)
	if err != nil || len(boards) != 2 {
		t.Fatalf("boards = %+v, err = %v", boards, err)
	}
	by := map[int64]Board{}
	for _, b := range boards {
		by[b.BoardID] = b
	}
	if !by[26].Registered || by[40].Registered || by[40].ProjectKey != "NEW" || by[40].Name != "New Board" {
		t.Fatalf("boards = %+v", by)
	}
}

func TestBoards_AreCachedBrieflyAndFreshBypassesIt(t *testing.T) {
	f := newFake()
	s := f.svc()
	_, _ = s.Boards(context.Background(), "dev@esb.co.id", false)
	_, _ = s.Boards(context.Background(), "dev@esb.co.id", false)
	if f.discoverN != 1 {
		t.Fatalf("Jira asked %d times, want 1 (cached)", f.discoverN)
	}
	_, _ = s.Boards(context.Background(), "dev@esb.co.id", true)
	if f.discoverN != 2 {
		t.Fatalf("fresh must ask Jira again, asked %d times", f.discoverN)
	}
}

func TestSetup_RegistersTheBoardAndSyncsTheUserFromJira(t *testing.T) {
	f := newFake()
	res, err := f.svc().Setup(context.Background(), "dev@esb.co.id", 40, tracking_rule.RuleInput{
		ProjectKey: "HACKED", TrackedStatuses: []string{"Code Process"}, DoneStatus: "Code Completed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.created) != 1 || f.created[0].ProjectKey != "NEW" {
		t.Fatalf("created = %+v: the project must come from the board, not the body", f.created)
	}
	if !res.Synced || res.Added != 3 || res.Updated != 1 || len(f.synced) != 1 || f.synced[0] != "dev@esb.co.id" {
		t.Fatalf("result = %+v, synced = %v", res, f.synced)
	}
}

func TestSetup_SyncFailureKeepsTheSavedBoard(t *testing.T) {
	f := newFake()
	f.syncErr = errors.New("jira down")
	res, err := f.svc().Setup(context.Background(), "dev@esb.co.id", 40, tracking_rule.RuleInput{})
	if err != nil || res.Synced || len(f.created) != 1 {
		t.Fatalf("res = %+v, err = %v, created = %d", res, err, len(f.created))
	}
}

func TestSetup_RejectsABoardThatIsNotTheUsersAndOneAlreadyRegistered(t *testing.T) {
	f := newFake()
	s := f.svc()
	if _, err := s.Setup(context.Background(), "dev@esb.co.id", 999, tracking_rule.RuleInput{}); !errors.Is(err, ErrBoardNotYours) {
		t.Fatalf("err = %v, want ErrBoardNotYours", err)
	}
	if _, err := s.Setup(context.Background(), "dev@esb.co.id", 26, tracking_rule.RuleInput{}); !errors.Is(err, ErrAlreadyRegistered) {
		t.Fatalf("err = %v, want ErrAlreadyRegistered", err)
	}
	if len(f.created) != 0 {
		t.Fatalf("created = %+v", f.created)
	}
}

func TestSetup_ARaceWithAnotherSaveReportsAlreadyRegistered(t *testing.T) {
	f := newFake()
	f.createErr = tracking_rule.ErrRuleExists
	if _, err := f.svc().Setup(context.Background(), "dev@esb.co.id", 40, tracking_rule.RuleInput{}); !errors.Is(err, ErrAlreadyRegistered) {
		t.Fatalf("err = %v", err)
	}
}

func TestSetup_ValidationErrorsPassThroughAndNothingSyncs(t *testing.T) {
	f := newFake()
	f.createErr = &tracking_rule.ValidationError{Fields: map[string]string{"doneStatus": "wajib"}}
	_, err := f.svc().Setup(context.Background(), "dev@esb.co.id", 40, tracking_rule.RuleInput{})
	var v *tracking_rule.ValidationError
	if !errors.As(err, &v) || len(f.synced) != 0 {
		t.Fatalf("err = %v, synced = %v", err, f.synced)
	}
}

func TestForm_OfARegisteredBoardIsRefusedAndOfANewOneIsPrefilled(t *testing.T) {
	f := newFake()
	s := f.svc()
	if _, err := s.Form(context.Background(), "dev@esb.co.id", 26); !errors.Is(err, ErrAlreadyRegistered) {
		t.Fatalf("err = %v", err)
	}
	form, err := s.Form(context.Background(), "dev@esb.co.id", 40)
	if err != nil || form.Board.ProjectKey != "NEW" || form.TemplateProject != "CORE" || len(form.Defaults.TrackedStatuses) != 1 || form.ProjectName != "Proj NEW" {
		t.Fatalf("form = %+v, err = %v", form, err)
	}
}

func serve(api API, method, path, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api", func(c *gin.Context) {
		c.Set(auth_middleware.CurrentUserKey, auth_dto.Session{UserID: 1, Email: "dev@esb.co.id"})
	})
	Routes(g, NewHandler(api))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	return w
}

func TestHandler_SetupMapsErrorsAndNeverTrustsTheBodyForTheScheduleFields(t *testing.T) {
	f := newFake()
	s := f.svc()
	w := serve(s, http.MethodPost, "/api/onboarding/boards/40/setup", `{"trackedStatuses":["Code Process"],"doneStatus":"Code Completed","excluded":{"dsu":[{"weekday":1}]},"createFields":{"1":["x"]}}`)
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"synced":true`) {
		t.Fatalf("→ %d %s", w.Code, w.Body.String())
	}
	if f.created[0].Excluded != nil || f.created[0].CreateFields != nil {
		t.Fatalf("created = %+v: onboarding must not set the schedule or create fields", f.created[0])
	}
	if w := serve(s, http.MethodPost, "/api/onboarding/boards/26/setup", `{}`); w.Code != http.StatusConflict {
		t.Fatalf("registered → %d", w.Code)
	}
	if w := serve(s, http.MethodPost, "/api/onboarding/boards/999/setup", `{}`); w.Code != http.StatusNotFound {
		t.Fatalf("not yours → %d", w.Code)
	}
	if w := serve(s, http.MethodPost, "/api/onboarding/boards/x/setup", `{}`); w.Code != http.StatusBadRequest {
		t.Fatalf("bad id → %d", w.Code)
	}
	f.createErr = &tracking_rule.ValidationError{Fields: map[string]string{"doneStatus": "wajib"}}
	f2 := newFake()
	f2.createErr = f.createErr
	if w := serve(f2.svc(), http.MethodPost, "/api/onboarding/boards/40/setup", `{}`); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"doneStatus":"wajib"`) {
		t.Fatalf("validation → %d %s", w.Code, w.Body.String())
	}
}

func TestHandler_BoardsListsThem(t *testing.T) {
	w := serve(newFake().svc(), http.MethodGet, "/api/onboarding/boards", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"registered":false`) || !strings.Contains(w.Body.String(), `"registered":true`) {
		t.Fatalf("→ %d %s", w.Code, w.Body.String())
	}
}
