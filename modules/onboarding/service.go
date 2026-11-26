// Package onboarding is step 2 of joining Nudge: the user's Jira boards are
// checked against the registered ones (a board is registered when its project
// has an active Aturan Board), and a board that is not gets set up right there
// and then synced from Jira (Tech Spec "Pisah Login dan Registrasi").
package onboarding

import (
	"context"
	"errors"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"nudge/modules/board/board_dto"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/jira_client/jira_client_service"
	"nudge/modules/registration/registration_dto"
	"nudge/modules/tracking_rule"
)

var (
	// ErrBoardNotYours: the board is not one the user has tickets on (right now).
	ErrBoardNotYours = errors.New("onboarding: board is not one of the user's boards")
	// ErrAlreadyRegistered: the board's project already has an Aturan Board.
	ErrAlreadyRegistered = errors.New("onboarding: board already registered")
	// ErrNotRegistered: the user has no account (should not happen behind the session).
	ErrNotRegistered = errors.New("onboarding: user not found")
)

const (
	boardsTTL   = 45 * time.Second
	syncTimeout = 60 * time.Second
)

type Users interface {
	FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error)
}
type Credentials interface {
	ResolveCredential(ctx context.Context, email string) (*jira_client_dto.Credential, error)
}
type BoardLookup interface {
	EnsureKnown(ctx context.Context, boardID int64) (board_dto.Board, error)
}

// Rules is the slice of tracking_rule.Service this module uses.
type Rules interface {
	List(ctx context.Context) ([]tracking_rule.RuleView, error)
	Lookup(ctx context.Context, projectKey string, fresh bool) (tracking_rule.Lookup, error)
	Create(ctx context.Context, in tracking_rule.RuleInput, editor string) (tracking_rule.RuleView, error)
}

// Syncer is sprint_backfill_service: it rebuilds the user's time from Jira.
type Syncer interface {
	RunForEmail(ctx context.Context, email string) (added, updated int, err error)
}

type Deps struct {
	Users     Users
	Creds     Credentials
	Discover  jira_client_service.BoardDiscoveryClient
	Boards    BoardLookup
	Rules     Rules
	Sync      Syncer
	IsTracked func(projectKey string) bool // default: jira_client_dto.IsTrackedProject
	Now       func() time.Time
}

// Board is one board of the user and whether it is registered.
type Board struct {
	BoardID     int64  `json:"boardId"`
	Name        string `json:"name"`
	ProjectKey  string `json:"projectKey"`
	SprintName  string `json:"sprintName"`
	TicketCount int    `json:"ticketCount"`
	Registered  bool   `json:"registered"`
}

type cached struct {
	boards []Board
	at     time.Time
}

type Service struct {
	d     Deps
	mu    sync.Mutex
	cache map[string]cached
}

func NewService(d Deps) *Service {
	if d.IsTracked == nil {
		d.IsTracked = jira_client_dto.IsTrackedProject
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d: d, cache: map[string]cached{}}
}

// Boards lists the boards with tickets of email in an active sprint, registered
// or not; fresh skips the short cache. A board whose project has no rule yet is
// Registered=false.
func (s *Service) Boards(ctx context.Context, email string, fresh bool) ([]Board, error) {
	key := strings.ToLower(email)
	s.mu.Lock()
	if c, ok := s.cache[key]; ok && !fresh && s.d.Now().Sub(c.at) < boardsTTL {
		s.mu.Unlock()
		return c.boards, nil
	}
	s.mu.Unlock()

	user, found, err := s.d.Users.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if !found || user == nil {
		return nil, ErrNotRegistered
	}
	cred, err := s.d.Creds.ResolveCredential(ctx, email)
	if err != nil {
		log.Printf("onboarding: credential of %s failed, using the bot's: %v", email, err)
		cred = nil
	}
	hits, err := s.d.Discover.OpenBoardsOf(ctx, cred, user.JiraAccountID)
	if err != nil {
		return nil, err
	}
	out := make([]Board, 0, len(hits))
	for _, h := range hits {
		b := Board{BoardID: h.BoardID, Name: "Board #" + strconv.FormatInt(h.BoardID, 10), ProjectKey: h.ProjectKey, SprintName: h.SprintName, TicketCount: h.TicketCount}
		if known, err := s.d.Boards.EnsureKnown(ctx, h.BoardID); err != nil {
			log.Printf("onboarding: board %d lookup failed: %v", h.BoardID, err)
		} else {
			b.Name = known.Name
			if known.ProjectKey != "" {
				b.ProjectKey = known.ProjectKey
			}
		}
		b.Registered = s.d.IsTracked(b.ProjectKey)
		out = append(out, b)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	s.mu.Lock()
	s.cache[key] = cached{boards: out, at: s.d.Now()}
	s.mu.Unlock()
	return out, nil
}

func (s *Service) forget() {
	s.mu.Lock()
	s.cache = map[string]cached{}
	s.mu.Unlock()
}

// boardOf finds one of the user's boards.
func (s *Service) boardOf(ctx context.Context, email string, boardID int64, fresh bool) (Board, error) {
	boards, err := s.Boards(ctx, email, fresh)
	if err != nil {
		return Board{}, err
	}
	for _, b := range boards {
		if b.BoardID == boardID {
			return b, nil
		}
	}
	return Board{}, ErrBoardNotYours
}

// Form is what the setup step shows: the project's Jira data and a suggestion.
type Form struct {
	Board        Board                         `json:"board"`
	ProjectName  string                        `json:"projectName"`
	Statuses     []string                      `json:"statuses"`
	NumberFields []jira_client_dto.NumberField `json:"numberFields"`
	Defaults     Defaults                      `json:"defaults"`
	// TemplateProject: the existing rule the defaults were copied from ("" = none fit).
	TemplateProject string `json:"templateProject"`
}

// Form loads the setup step of a board that is not registered yet.
func (s *Service) Form(ctx context.Context, email string, boardID int64) (Form, error) {
	b, err := s.boardOf(ctx, email, boardID, false)
	if err != nil {
		return Form{}, err
	}
	if b.Registered {
		return Form{}, ErrAlreadyRegistered
	}
	l, err := s.d.Rules.Lookup(ctx, b.ProjectKey, false)
	if err != nil {
		return Form{}, err
	}
	rules, err := s.d.Rules.List(ctx)
	if err != nil {
		log.Printf("onboarding: listing the rules for suggestions failed: %v", err)
	}
	def, from := Suggest(l, rules)
	return Form{Board: b, ProjectName: l.ProjectName, Statuses: l.Statuses, NumberFields: l.NumberFields, Defaults: def, TemplateProject: from}, nil
}

// Result is what saving a board reports.
type Result struct {
	ProjectKey string `json:"projectKey"`
	// Synced: the user's tickets were rebuilt from Jira before answering.
	Synced  bool `json:"synced"`
	Added   int  `json:"added"`
	Updated int  `json:"updated"`
}

// Setup registers the board's project with the user's choices and syncs the
// user's tickets from Jira right away (the board is new, so there is no history).
func (s *Service) Setup(ctx context.Context, email string, boardID int64, in tracking_rule.RuleInput) (Result, error) {
	b, err := s.boardOf(ctx, email, boardID, true)
	if err != nil {
		return Result{}, err
	}
	if b.Registered {
		return Result{}, ErrAlreadyRegistered
	}
	in.ProjectKey = b.ProjectKey
	if _, err := s.d.Rules.Create(ctx, in, email); err != nil {
		if errors.Is(err, tracking_rule.ErrRuleExists) {
			s.forget()
			return Result{}, ErrAlreadyRegistered
		}
		return Result{}, err
	}
	s.forget()

	res := Result{ProjectKey: b.ProjectKey}
	// The saved row must not be lost if the visitor closes the tab mid-sync.
	syncCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), syncTimeout)
	defer cancel()
	added, updated, err := s.d.Sync.RunForEmail(syncCtx, email)
	if err != nil {
		log.Printf("onboarding: syncing %s after registering %s failed: %v", email, b.ProjectKey, err)
		return res, nil
	}
	res.Synced, res.Added, res.Updated = true, added, updated
	return res, nil
}
