// Package auto_done moves a ticket to its board's done status by itself once its
// time budget is used up exactly (Tech Spec "Geser Otomatis ke Selesai"). It is
// switched on per user in Pengaturan > Umum and only works with the user's own
// Jira API key, so Jira records the move under their name.
package auto_done

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/business_hours/business_hours_service"
	"nudge/modules/issue_session/issue_session_dto"
	"nudge/modules/issue_session/issue_session_service"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/move_fields"
	"nudge/modules/user_dashboard/user_dashboard_dto"
	"nudge/modules/user_dashboard/user_dashboard_service"
)

const (
	// Every cycle looks at the running tickets of the users who switched this on.
	cycle = 30 * time.Second
	// A ticket that is not eligible right now (in review, no personal key, not
	// its own...) is looked at again only after this long, so Jira is not asked
	// about it every cycle.
	recheck = 5 * time.Minute

	OutcomeMoved  = "moved"
	OutcomeFailed = "failed"
)

// ErrCredentialRequired: the feature needs the user's own Jira API key.
var ErrCredentialRequired = errors.New("auto_done: a personal Jira API key is required")

type Sessions interface {
	GetActiveSessions(ctx context.Context) ([]issue_session_dto.IssueSession, error)
}
type Credentials interface {
	ResolveCredential(ctx context.Context, email string) (*jira_client_dto.Credential, error)
}
type Dashboard interface {
	ListTransitions(ctx context.Context, email, issueKey string) (user_dashboard_dto.TicketTransitions, error)
	Transition(ctx context.Context, email, issueKey, transitionID string) (string, error)
}
type Notifier interface {
	SendPlainTextDM(ctx context.Context, email, text string) error
}

type Deps struct {
	Store     Store
	Sessions  Sessions
	Creds     Credentials
	Dashboard Dashboard
	Notifier  Notifier
	Calendar  business_hours_dto.CalendarSource
	Hours     business_hours_service.BusinessHoursService
	// RuleFor is the project's tracking rule; defaults to jira_client_dto.RuleFor.
	RuleFor func(projectKey string) jira_client_dto.TrackingRule
	Now     func() time.Time
}

type Service struct {
	d    Deps
	mu   sync.Mutex
	skip map[string]time.Time // issueKey|run → look again after
}

func NewService(d Deps) *Service {
	if d.RuleFor == nil {
		d.RuleFor = jira_client_dto.RuleFor
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d: d, skip: map[string]time.Time{}}
}

// Status is what Pengaturan shows.
type Status struct {
	// Enabled is the user's choice; Active = Enabled and a personal key exists.
	Enabled       bool `json:"enabled"`
	HasCredential bool `json:"hasCredential"`
	Active        bool `json:"active"`
}

func (s *Service) Status(ctx context.Context, userID int64, email string) (Status, error) {
	on, err := s.d.Store.IsEnabled(ctx, userID)
	if err != nil {
		return Status{}, err
	}
	cred, err := s.d.Creds.ResolveCredential(ctx, email)
	if err != nil {
		return Status{}, err
	}
	st := Status{Enabled: on, HasCredential: cred != nil}
	st.Active = st.Enabled && st.HasCredential
	return st, nil
}

// SetEnabled switches the feature; turning it on needs the user's own Jira API
// key (ErrCredentialRequired otherwise).
func (s *Service) SetEnabled(ctx context.Context, userID int64, email string, enabled bool) (Status, error) {
	if enabled {
		cred, err := s.d.Creds.ResolveCredential(ctx, email)
		if err != nil {
			return Status{}, err
		}
		if cred == nil {
			return Status{}, ErrCredentialRequired
		}
	}
	if err := s.d.Store.SetEnabled(ctx, userID, enabled); err != nil {
		return Status{}, err
	}
	return s.Status(ctx, userID, email)
}

// Run checks every cycle until ctx ends.
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(cycle)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.RunOnce(ctx)
		}
	}
}

func (s *Service) skipped(key string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if until, ok := s.skip[key]; ok {
		if now.Before(until) {
			return true
		}
		delete(s.skip, key)
	}
	return false
}

func (s *Service) skipFor(key string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.skip) > 5000 { // never grows without bound
		s.skip = map[string]time.Time{}
	}
	s.skip[key] = now.Add(recheck)
}

func runStart(sess issue_session_dto.IssueSession) time.Time {
	if sess.CurrentSessionStartedDate != nil && !sess.CurrentSessionStartedDate.IsZero() {
		return sess.CurrentSessionStartedDate.UTC().Truncate(time.Second)
	}
	return time.Unix(0, 0).UTC()
}

// RunOnce moves every ticket of an opted-in user whose time is used up. It
// returns how many were moved.
func (s *Service) RunOnce(ctx context.Context) int {
	enabled, err := s.d.Store.EnabledEmails(ctx)
	if err != nil {
		log.Printf("auto_done: listing the users failed: %v", err)
		return 0
	}
	if len(enabled) == 0 {
		return 0
	}
	sessions, err := s.d.Sessions.GetActiveSessions(ctx)
	if err != nil {
		log.Printf("auto_done: listing the running tickets failed: %v", err)
		return 0
	}
	now := s.d.Now()
	moved := 0
	for _, sess := range sessions {
		if !enabled[sess.AssigneeEmail] || sess.StoryPoints <= 0 {
			continue
		}
		budget := int64(sess.StoryPoints * 3600)
		cal := s.d.Calendar.ForEmail(ctx, sess.AssigneeEmail)
		elapsed := issue_session_service.CalculateElapsedSeconds(sess, s.d.Hours, cal)
		if elapsed < budget {
			continue
		}
		start := runStart(sess)
		key := sess.IssueKey + "|" + start.Format(time.RFC3339)
		if s.skipped(key, now) {
			continue
		}
		if s.handle(ctx, sess, start, elapsed, budget) {
			moved++
		}
		s.skipFor(key, now)
	}
	return moved
}

// handle tries one ticket whose time is used up; true when it was moved.
func (s *Service) handle(ctx context.Context, sess issue_session_dto.IssueSession, start time.Time, elapsed, budget int64) bool {
	email, issue := sess.AssigneeEmail, sess.IssueKey
	// Only with the user's own key: the move is recorded in Jira under their name.
	cred, err := s.d.Creds.ResolveCredential(ctx, email)
	if err != nil || cred == nil {
		return false
	}
	rule := s.d.RuleFor(sess.ProjectKey)
	done := rule.DoneStatus
	if done == "" {
		return false
	}
	trs, err := s.d.Dashboard.ListTransitions(ctx, email, issue)
	if err != nil {
		if !errors.Is(err, user_dashboard_service.ErrNotOwnTicket) {
			log.Printf("auto_done: transitions of %s failed: %v", issue, err)
		}
		return false
	}
	current := trs.CurrentStatus
	// Only a ticket being worked on: one already done, or waiting in review (its
	// time still counts, but it is out of the assignee's hands), stays as it is.
	if strings.EqualFold(current, done) || !rule.IsTracked(current) || (rule.ReviewStatus != "" && strings.EqualFold(current, rule.ReviewStatus)) {
		return false
	}

	ok, err := s.d.Store.Claim(ctx, issue, start, email, done)
	if err != nil {
		log.Printf("auto_done: claiming %s failed: %v", issue, err)
		return false
	}
	if !ok {
		return false
	}

	var transitionID string
	for _, tr := range trs.Transitions {
		if strings.EqualFold(tr.ToStatus, done) {
			transitionID = tr.ID
			break
		}
	}
	if transitionID == "" {
		s.fail(ctx, sess, start, done, fmt.Sprintf("tidak ada jalur langsung dari %s ke %s di workflow Jira", current, done))
		return false
	}
	status, err := s.d.Dashboard.Transition(ctx, email, issue, transitionID)
	var fr *move_fields.FieldsRequiredError
	switch {
	case errors.As(err, &fr):
		s.fail(ctx, sess, start, done, fmt.Sprintf("Jira meminta field yang belum terisi (%s)", fr.Error()))
		return false
	case err != nil:
		log.Printf("auto_done: moving %s to %s failed: %v", issue, done, err)
		s.fail(ctx, sess, start, done, "Jira menolak perpindahan statusnya")
		return false
	}
	if err := s.d.Store.Finish(ctx, issue, start, OutcomeMoved, "digeser ke "+status); err != nil {
		log.Printf("auto_done: recording %s failed: %v", issue, err)
	}
	log.Printf("auto_done: %s moved to %s for %s (time %ds of %ds)", issue, status, email, elapsed, budget)
	s.tell(ctx, email, fmt.Sprintf("✅ *%s* otomatis digeser ke *%s*\n⏱ Waktunya sudah habis tepat di budget %s (%s). Kamu bisa mematikan geser otomatis di *Pengaturan › Umum*.",
		issue, status, budgetText(budget), sess.IssueSummary))
	return true
}

func (s *Service) fail(ctx context.Context, sess issue_session_dto.IssueSession, start time.Time, done, reason string) {
	if err := s.d.Store.Finish(ctx, sess.IssueKey, start, OutcomeFailed, reason); err != nil {
		log.Printf("auto_done: recording %s failed: %v", sess.IssueKey, err)
	}
	s.tell(ctx, sess.AssigneeEmail, fmt.Sprintf("⚠️ *%s* belum bisa digeser otomatis ke *%s*: %s.\nWaktunya sudah habis, silakan geser manual di Papan Tiket.", sess.IssueKey, done, reason))
}

func (s *Service) tell(ctx context.Context, email, text string) {
	if s.d.Notifier == nil {
		return
	}
	if err := s.d.Notifier.SendPlainTextDM(ctx, email, text); err != nil {
		log.Printf("auto_done: DM to %s failed: %v", email, err)
	}
}

// budgetText writes a budget in hours and minutes ("4 jam", "1 jam 30 menit").
func budgetText(seconds int64) string {
	h, m := seconds/3600, (seconds%3600)/60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%d jam %d menit", h, m)
	case h > 0:
		return fmt.Sprintf("%d jam", h)
	default:
		return fmt.Sprintf("%d menit", m)
	}
}
