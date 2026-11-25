package sprint_backfill_service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/business_hours/business_hours_service"
	"nudge/modules/issue_session/issue_session_dto"
	"nudge/modules/issue_session/issue_session_repository"
	"nudge/modules/jira_account/jira_account_holder"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/registration/registration_dto"
)

// jiraSource is the slice of jira_client_service.JiraAgileClient the
// backfill needs.
type jiraSource interface {
	SearchActiveSprintIssuesByAssignee(ctx context.Context, cred *jira_client_dto.Credential, accountID string) ([]jira_client_dto.Issue, error)
	SearchIssuesByKeysForAssignee(ctx context.Context, cred *jira_client_dto.Credential, keys []string, accountID string) ([]jira_client_dto.Issue, error)
	GetChangelog(ctx context.Context, cred *jira_client_dto.Credential, issueKey string) ([]jira_client_dto.ChangelogEntry, error)
}

// userDirectory is the slice of registration_repository the backfill needs.
type userDirectory interface {
	FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error)
	FindAll(ctx context.Context) ([]registration_dto.User, error)
}

// credentialResolver picks the Jira credential for a user (personal API key
// or the bot's); nil means always the bot's.
type credentialResolver interface {
	ResolveCredential(ctx context.Context, email string) (*jira_client_dto.Credential, error)
}

type sprintBackfillServiceImpl struct {
	jira          jiraSource
	users         userDirectory
	credentials   credentialResolver
	sessions      issue_session_repository.SessionBackfillRepository
	businessHours business_hours_service.BusinessHoursService
	calendar      business_hours_dto.CalendarSource
	now           func() time.Time
	// recalcLocks keeps one recalculation per user at a time.
	recalcLocks sync.Map
	// sprintStart is when a sprint began (nil = unknown), for the "week N of the
	// sprint" excluded slots of a project-wide recalculation.
	sprintStart func(ctx context.Context, sprintID int64) (time.Time, bool)
}

// Option tunes the service.
type Option func(*sprintBackfillServiceImpl)

// WithSprintStart supplies the start date of a sprint by id.
func WithSprintStart(fn func(ctx context.Context, sprintID int64) (time.Time, bool)) Option {
	return func(s *sprintBackfillServiceImpl) { s.sprintStart = fn }
}

func NewSprintBackfillService(
	jira jiraSource,
	users userDirectory,
	credentials credentialResolver,
	sessions issue_session_repository.SessionBackfillRepository,
	businessHours business_hours_service.BusinessHoursService,
	calendar business_hours_dto.CalendarSource,
	opts ...Option,
) SprintBackfillService {
	s := &sprintBackfillServiceImpl{
		jira: jira, users: users, credentials: credentials, sessions: sessions,
		businessHours: businessHours, calendar: calendar, now: time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *sprintBackfillServiceImpl) RunForEmail(ctx context.Context, email string) (int, int, error) {
	user, cred, err := s.userAndCredential(ctx, email)
	if err != nil || user == nil {
		return 0, 0, err
	}
	return s.runForUser(ctx, *user, cred)
}

// userAndCredential resolves a registered user (nil when unknown) and the
// Jira credential to act with (the bot's when theirs fails).
func (s *sprintBackfillServiceImpl) userAndCredential(ctx context.Context, email string) (*registration_dto.User, *jira_client_dto.Credential, error) {
	user, found, err := s.users.FindUserByEmail(ctx, email)
	if err != nil || !found || user == nil {
		return nil, nil, err
	}
	var cred *jira_client_dto.Credential
	if s.credentials != nil {
		if c, err := s.credentials.ResolveCredential(ctx, email); err == nil {
			cred = c
		} else {
			log.Printf("sprint_backfill: credential of %s failed, using the bot's: %v", email, err)
		}
	}
	return user, cred, nil
}

func (s *sprintBackfillServiceImpl) RunAll(ctx context.Context) int {
	if !jira_account_holder.Configured() {
		jira_account_holder.NoteMissing("sprint_backfill")
		return 0
	}
	users, err := s.users.FindAll(ctx)
	if err != nil {
		log.Printf("sprint_backfill: listing users failed: %v", err)
		return 0
	}
	total, totalUpdated := 0, 0
	for _, user := range users {
		if !user.FlagActive || user.JiraAccountID == "" {
			continue
		}
		// The bot's own credential: an automatic run must not depend on a
		// user's personal API key.
		added, updated, err := s.runForUser(ctx, user, nil)
		if err != nil {
			log.Printf("sprint_backfill: %s failed: %v", user.Email, err)
		}
		total += added
		totalUpdated += updated
	}
	if total > 0 || totalUpdated > 0 {
		log.Printf("sprint_backfill: added %d, recalculated %d session(s) from Jira changelogs", total, totalUpdated)
	}
	return total
}

// runForUser covers the user's active-sprint tickets plus their own
// subtasks/linked items that are in an active sprint too (AC3.1): tickets
// without a session are backfilled, the others recalculated.
func (s *sprintBackfillServiceImpl) runForUser(ctx context.Context, user registration_dto.User, cred *jira_client_dto.Credential) (int, int, error) {
	issues, err := s.userIssues(ctx, user, cred)
	if err != nil {
		return 0, 0, err
	}
	added := 0
	fresh := make(map[string]bool)
	for _, issue := range issues {
		ok, err := s.backfillIssue(ctx, user, cred, issue)
		if err != nil {
			log.Printf("sprint_backfill: %s skipped: %v", issue.Key, err)
			continue
		}
		if ok {
			added++
			fresh[issue.Key] = true
		}
	}

	unlock := s.lockRecalc(user.Email)
	defer unlock()
	return added, s.recalculateIssues(ctx, user, cred, issues, fresh), nil
}

// lockRecalc keeps one recalculation per user at a time.
func (s *sprintBackfillServiceImpl) lockRecalc(email string) func() {
	lock, _ := s.recalcLocks.LoadOrStore(email, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// userIssues = the user's active-sprint tickets plus their own subtasks /
// linked items (the related ones are filtered per ticket by the callers).
func (s *sprintBackfillServiceImpl) userIssues(ctx context.Context, user registration_dto.User, cred *jira_client_dto.Credential) ([]jira_client_dto.Issue, error) {
	issues, err := s.jira.SearchActiveSprintIssuesByAssignee(ctx, cred, user.JiraAccountID)
	if err != nil {
		return nil, fmt.Errorf("active sprint search: %w", err)
	}

	seen := make(map[string]bool, len(issues))
	for _, issue := range issues {
		seen[issue.Key] = true
	}
	var relatedKeys []string
	for _, issue := range issues {
		for _, rel := range issue.Relations() {
			if !seen[rel.Key] {
				seen[rel.Key] = true
				relatedKeys = append(relatedKeys, rel.Key)
			}
		}
	}
	if len(relatedKeys) > 0 {
		related, err := s.jira.SearchIssuesByKeysForAssignee(ctx, cred, relatedKeys, user.JiraAccountID)
		if err != nil {
			log.Printf("sprint_backfill: related items of %s skipped: %v", user.Email, err)
		} else {
			issues = append(issues, related...)
		}
	}
	return issues, nil
}

func (s *sprintBackfillServiceImpl) RecalculateForEmail(ctx context.Context, email string) (int, error) {
	unlock := s.lockRecalc(email)
	defer unlock()

	user, cred, err := s.userAndCredential(ctx, email)
	if err != nil || user == nil {
		return 0, err
	}
	issues, err := s.userIssues(ctx, *user, cred)
	if err != nil {
		return 0, err
	}
	return s.recalculateIssues(ctx, *user, cred, issues, nil), nil
}

// RecalculateProject rewrites the time of every session of the project, in
// every sprint, from Jira's changelog, with the project's excluded slots (Daily
// Stand Up, Sprint Planning...) left out. A "week N of the sprint" slot counts
// from that session's own sprint; a session without a known sprint only gets
// the every-week slots. Only finished runs are counted: a run still going stays
// with the poller, which uses the same calendar from now on.
func (s *sprintBackfillServiceImpl) RecalculateProject(ctx context.Context, projectKey string) int {
	if !jira_account_holder.Configured() {
		jira_account_holder.NoteMissing("sprint_backfill")
		return 0
	}
	sessions, err := s.sessions.ListSessionsByProject(ctx, projectKey)
	if err != nil {
		log.Printf("sprint_backfill: listing the sessions of %s failed: %v", projectKey, err)
		return 0
	}
	starts := map[int64]time.Time{}
	sprintStart := func(id int64) time.Time {
		if start, seen := starts[id]; seen {
			return start
		}
		var start time.Time
		if s.sprintStart != nil {
			if t, ok := s.sprintStart(ctx, id); ok {
				start = t
			}
		}
		starts[id] = start
		return start
	}

	updated := 0
	for _, ss := range sessions {
		if ctx.Err() != nil {
			break
		}
		cal := s.calendar.ForEmail(ctx, ss.AssigneeEmail).ForProject(projectKey)
		// The active sprint's start is wrong for an older sprint: use the session's own.
		cal = cal.WithSprintStart(time.Time{})
		if ss.SprintID != nil {
			cal = cal.WithSprintStart(sprintStart(*ss.SprintID))
		}
		histories, err := s.jira.GetChangelog(ctx, nil, ss.IssueKey)
		if err != nil {
			log.Printf("sprint_backfill: recalculating %s skipped: %v", ss.IssueKey, err)
			continue
		}
		seconds, found := jira_client_dto.TrackedSeconds(histories, jira_client_dto.RuleForIssue(ss.IssueKey), func(start, end time.Time) int64 {
			return s.businessHours.ElapsedBusinessSeconds(start, end, cal)
		})
		if !found {
			continue
		}
		if err := s.sessions.UpdateAccumulatedSeconds(ctx, ss.IssueKey, seconds); err != nil {
			log.Printf("sprint_backfill: recalculating %s failed: %v", ss.IssueKey, err)
			continue
		}
		updated++
	}
	return updated
}

// recalculateIssues rewrites accumulatedSeconds of user's existing
// active-sprint sessions among issues from the changelog (skip: keys left
// alone, e.g. just backfilled). Returns how many were rewritten.
func (s *sprintBackfillServiceImpl) recalculateIssues(ctx context.Context, user registration_dto.User, cred *jira_client_dto.Credential, issues []jira_client_dto.Issue, skip map[string]bool) int {
	userCal := s.calendar.ForEmail(ctx, user.Email)

	updated := 0
	for _, issue := range issues {
		if skip[issue.Key] {
			continue
		}
		if _, inSprint := issue.Fields.ActiveSprint(); !inSprint {
			continue // AC4.3: sessions outside the active sprint stay as they are
		}
		if a := issue.Fields.Assignee; a != nil && a.AccountID != "" && a.AccountID != user.JiraAccountID {
			continue
		}
		exists, err := s.sessions.SessionExists(ctx, issue.Key)
		if err != nil || !exists {
			continue // no session yet: the backfill's job, not ours
		}
		histories, err := s.jira.GetChangelog(ctx, cred, issue.Key)
		if err != nil {
			log.Printf("sprint_backfill: recalculating %s skipped: %v", issue.Key, err)
			continue
		}
		// The project's Daily Stand Up / Sprint Planning slots do not count.
		cal := userCal.ForProject(jira_client_dto.ProjectKeyOf(issue.Key))
		elapsed := func(start, end time.Time) int64 { return s.businessHours.ElapsedBusinessSeconds(start, end, cal) }
		seconds, found := jira_client_dto.TrackedSeconds(histories, jira_client_dto.RuleForIssue(issue.Key), elapsed)
		if !found {
			continue
		}
		if err := s.sessions.UpdateAccumulatedSeconds(ctx, issue.Key, seconds); err != nil {
			log.Printf("sprint_backfill: recalculating %s failed: %v", issue.Key, err)
			continue
		}
		updated++
	}
	return updated
}

// backfillIssue applies AC3.2–AC3.5 to one ticket.
func (s *sprintBackfillServiceImpl) backfillIssue(ctx context.Context, user registration_dto.User, cred *jira_client_dto.Credential, issue jira_client_dto.Issue) (bool, error) {
	sprint, inSprint := issue.Fields.ActiveSprint()
	if !inSprint {
		return false, nil
	}
	if a := issue.Fields.Assignee; a != nil && a.AccountID != "" && a.AccountID != user.JiraAccountID {
		return false, nil
	}
	sp := issue.StoryPoints()
	if sp == nil || *sp <= 0 {
		return false, nil
	}
	rule := jira_client_dto.RuleForIssue(issue.Key)
	if rule.IsTracked(issue.Fields.Status.Name) {
		return false, nil // in progress right now: the poller's job
	}
	exists, err := s.sessions.SessionExists(ctx, issue.Key)
	if err != nil || exists {
		return false, err
	}

	histories, err := s.jira.GetChangelog(ctx, cred, issue.Key)
	if err != nil {
		return false, err
	}
	cal := s.calendar.ForEmail(ctx, user.Email).ForProject(jira_client_dto.ProjectKeyOf(issue.Key))
	seconds, found := jira_client_dto.TrackedSeconds(histories, rule, func(start, end time.Time) int64 {
		return s.businessHours.ElapsedBusinessSeconds(start, end, cal)
	})
	if !found {
		return false, nil
	}

	boardID, sprintID := sprint.BoardID, sprint.ID
	return s.sessions.InsertClosedSession(ctx, issue_session_dto.IssueSession{
		IssueKey:           issue.Key,
		ProjectKey:         jira_client_dto.ProjectKeyOf(issue.Key),
		StoryPoints:        *sp,
		AccumulatedSeconds: seconds,
		AssigneeEmail:      user.Email,
		IssueSummary:       issue.Fields.Summary,
		SprintName:         sprint.Name,
		BoardID:            &boardID,
		SprintID:           &sprintID,
	})
}

// dailyAt is when the automatic backfill runs every day (WIB).
const dailyHour, dailyMinute = 7, 0

var wib = time.FixedZone("WIB", 7*3600)

// nextDailyRun returns the next 07:00 WIB strictly after now.
func nextDailyRun(now time.Time) time.Time {
	local := now.In(wib)
	next := time.Date(local.Year(), local.Month(), local.Day(), dailyHour, dailyMinute, 0, 0, wib)
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func (s *sprintBackfillServiceImpl) RunDaily(ctx context.Context) {
	s.RunAll(ctx)
	for {
		timer := time.NewTimer(time.Until(nextDailyRun(s.now())))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			s.RunAll(ctx)
		}
	}
}
