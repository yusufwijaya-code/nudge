package time_report

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"nudge/modules/board/board_dto"
	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/jira_client/jira_client_dto"
	"nudge/modules/jira_client/jira_client_service"
	"nudge/modules/registration/registration_dto"
	"nudge/modules/user_leave/user_leave_dto"
)

const (
	// MaxRangeDays bounds one report's date range.
	MaxRangeDays = 366
	// MaxTickets bounds one report (the range is cut there, and says so).
	MaxTickets = 300
	// changelogWorkers read the status histories side by side.
	changelogWorkers = 4
	// jobTTL is how long a finished report stays available (the PDF of it).
	jobTTL = 30 * time.Minute
	// jobTimeout stops a report that hangs.
	jobTimeout = 4 * time.Minute
)

var (
	ErrInvalidRange   = errors.New("time_report: invalid date range")
	ErrBoardUnknown   = errors.New("time_report: board unknown or not registered")
	ErrNoJiraAccount  = errors.New("time_report: the user has no Jira account id")
	ErrJobNotFound    = errors.New("time_report: report not found")
	ErrJobNotFinished = errors.New("time_report: report not finished")
)

type Users interface {
	FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error)
}
type Credentials interface {
	ResolveCredential(ctx context.Context, email string) (*jira_client_dto.Credential, error)
}
type Boards interface {
	Get(ctx context.Context, boardID int64) (board_dto.Board, bool, error)
}
type Leaves interface {
	List(ctx context.Context, email string) ([]user_leave_dto.Leave, error)
}

// Syncer is sprint_backfill: Nudge's own data brought in line with Jira first.
type Syncer interface {
	RunForEmail(ctx context.Context, email string) (added, updated int, err error)
}

type Deps struct {
	Users    Users
	Creds    Credentials
	Jira     jira_client_service.ReportClient
	Boards   Boards
	Leaves   Leaves
	Sync     Syncer // may be nil
	National business_hours_dto.HolidayCalendar
	// Cap is the deviation an admin allows (Pengaturan Aplikasi).
	Cap func() float64
	// RuleFor / Schedule / ActiveProjects default to the live registries.
	RuleFor        func(projectKey string) jira_client_dto.TrackingRule
	Schedule       func(projectKey string) business_hours_dto.Schedule
	ActiveProjects func() []string
	JiraBaseURL    string
	Now            func() time.Time
	// Lifetime ends the running reports on shutdown.
	Lifetime context.Context
}

// Params is what the user asked for.
type Params struct {
	From    string `json:"from"` // YYYY-MM-DD, inclusive: the ticket's created date
	To      string `json:"to"`
	BoardID int64  `json:"boardId"` // 0 = every board
}

// The steps the progress shows, in order.
var stepNames = []string{
	"Sinkron data Nudge dengan Jira",
	"Mengambil tiket dari Jira",
	"Membaca riwayat status tiket",
	"Menghitung waktu, pengurang, dan deviasi",
	"Menyusun laporan",
}

// Progress is what the page polls.
type Progress struct {
	ID      string   `json:"id"`
	State   string   `json:"state"` // running | done | failed | cancelled
	Step    int      `json:"step"`
	Steps   []string `json:"steps"`
	Done    int      `json:"done"`
	Total   int      `json:"total"`
	Current string   `json:"current,omitempty"`
	Percent int      `json:"percent"`
	Error   string   `json:"error,omitempty"`
	Params  Params   `json:"params"`
	Report  *Report  `json:"report,omitempty"`
}

type job struct {
	mu       sync.Mutex
	progress Progress
	email    string
	cancel   context.CancelFunc
	finished time.Time
}

func (j *job) set(fn func(p *Progress)) {
	j.mu.Lock()
	defer j.mu.Unlock()
	fn(&j.progress)
	j.progress.Percent = percentOf(j.progress)
}

func (j *job) snapshot(withReport bool) Progress {
	j.mu.Lock()
	defer j.mu.Unlock()
	p := j.progress
	p.Steps = append([]string(nil), p.Steps...)
	if !withReport {
		p.Report = nil
	}
	return p
}

// percentOf weighs the steps: reading the histories is most of the work.
func percentOf(p Progress) int {
	if p.State == "done" {
		return 100
	}
	bounds := []float64{0, 12, 22, 90, 97, 100}
	if p.Step < 0 || p.Step >= len(bounds)-1 {
		return 0
	}
	from, to := bounds[p.Step], bounds[p.Step+1]
	frac := 0.0
	if p.Total > 0 {
		frac = float64(p.Done) / float64(p.Total)
	}
	return int(from + (to-from)*frac)
}

type Service struct {
	d    Deps
	mu   sync.Mutex
	jobs map[string]*job
}

func NewService(d Deps) *Service {
	if d.RuleFor == nil {
		d.RuleFor = jira_client_dto.RuleFor
	}
	if d.Schedule == nil {
		d.Schedule = business_hours_dto.ScheduleFor
	}
	if d.ActiveProjects == nil {
		d.ActiveProjects = jira_client_dto.ActiveProjectKeys
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Lifetime == nil {
		d.Lifetime = context.Background()
	}
	if d.Cap == nil {
		d.Cap = func() float64 { return 12 }
	}
	return &Service{d: d, jobs: map[string]*job{}}
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// validRange checks the dates and returns the day after `to` (Jira's exclusive end).
func validRange(p Params) (string, error) {
	from, err1 := time.ParseInLocation("2006-01-02", p.From, wib)
	to, err2 := time.ParseInLocation("2006-01-02", p.To, wib)
	if err1 != nil || err2 != nil || to.Before(from) || to.Sub(from) > MaxRangeDays*24*time.Hour {
		return "", ErrInvalidRange
	}
	return to.AddDate(0, 0, 1).Format("2006-01-02"), nil
}

// Start begins a report for email. A report already running for the user is
// returned as it is (one at a time per user).
func (s *Service) Start(ctx context.Context, email string, p Params) (Progress, error) {
	if _, err := validRange(p); err != nil {
		return Progress{}, err
	}
	if p.BoardID > 0 {
		b, found, err := s.d.Boards.Get(ctx, p.BoardID)
		if err != nil {
			return Progress{}, err
		}
		if !found || !jira_client_dto.IsTrackedProject(b.ProjectKey) {
			return Progress{}, ErrBoardUnknown
		}
	}

	s.mu.Lock()
	now := s.d.Now()
	for id, j := range s.jobs {
		j.mu.Lock()
		state, fin, owner := j.progress.State, j.finished, j.email
		j.mu.Unlock()
		if state != "running" && now.Sub(fin) > jobTTL {
			delete(s.jobs, id)
			continue
		}
		if owner == email && state == "running" {
			s.mu.Unlock()
			return j.snapshot(false), nil
		}
	}
	runCtx, cancel := context.WithTimeout(s.d.Lifetime, jobTimeout)
	j := &job{email: email, cancel: cancel, progress: Progress{ID: newID(), State: "running", Steps: stepNames, Params: p}}
	s.jobs[j.progress.ID] = j
	s.mu.Unlock()

	go func() {
		defer cancel()
		report, err := s.run(runCtx, j, email, p)
		j.set(func(pr *Progress) {
			switch {
			case pr.State == "cancelled":
			case err != nil:
				pr.State = "failed"
				pr.Error = friendly(err)
			default:
				pr.State, pr.Step, pr.Report = "done", len(stepNames)-1, report
			}
		})
		j.mu.Lock()
		j.finished = s.d.Now()
		j.mu.Unlock()
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("time_report: report of %s failed: %v", email, err)
		}
	}()
	return j.snapshot(false), nil
}

func friendly(err error) string {
	switch {
	case errors.Is(err, ErrNoJiraAccount):
		return "Akun Jira kamu belum terhubung ke Nudge."
	case errors.Is(err, context.DeadlineExceeded):
		return "Jira terlalu lama menjawab. Coba rentang tanggal yang lebih pendek."
	case errors.Is(err, context.Canceled):
		return "Laporan dibatalkan."
	}
	return "Gagal membaca data dari Jira. Coba lagi sebentar lagi."
}

func (s *Service) find(email, id string) (*job, error) {
	s.mu.Lock()
	j := s.jobs[id]
	s.mu.Unlock()
	if j == nil || j.email != email {
		return nil, ErrJobNotFound
	}
	return j, nil
}

// Progress is the report's state; the report itself once it is done.
func (s *Service) Progress(email, id string) (Progress, error) {
	j, err := s.find(email, id)
	if err != nil {
		return Progress{}, err
	}
	return j.snapshot(true), nil
}

// Cancel stops a running report.
func (s *Service) Cancel(email, id string) error {
	j, err := s.find(email, id)
	if err != nil {
		return err
	}
	j.set(func(p *Progress) {
		if p.State == "running" {
			p.State, p.Error = "cancelled", "Laporan dibatalkan."
		}
	})
	j.cancel()
	return nil
}

// Result is the finished report (for the PDF).
func (s *Service) Result(email, id string) (*Report, error) {
	j, err := s.find(email, id)
	if err != nil {
		return nil, err
	}
	p := j.snapshot(true)
	if p.State != "done" || p.Report == nil {
		return nil, ErrJobNotFinished
	}
	return p.Report, nil
}

func (s *Service) cancelled(j *job) bool {
	return j.snapshot(false).State == "cancelled"
}

// run does the work, step by step.
func (s *Service) run(ctx context.Context, j *job, email string, p Params) (*Report, error) {
	toExclusive, _ := validRange(p)
	user, found, err := s.d.Users.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if !found || user == nil || strings.TrimSpace(user.JiraAccountID) == "" {
		return nil, ErrNoJiraAccount
	}
	now := s.d.Now()
	capPercent := s.d.Cap()
	report := &Report{
		GeneratedAt: now.In(wib).Format(time.RFC3339), UserName: user.UserName, UserEmail: user.Email,
		BoardID: p.BoardID, BoardName: "Semua board", From: p.From, To: p.To, CapPercent: capPercent,
		Statuses: map[string][]string{}, Tickets: []Ticket{}, Excluded: []Excluded{},
	}

	// 1. Nudge's own data in line with Jira (best effort: the report reads Jira anyway).
	j.set(func(pr *Progress) { pr.Step = 0 })
	if s.d.Sync != nil {
		if _, _, err := s.d.Sync.RunForEmail(ctx, email); err != nil {
			report.SyncWarning = "Sinkron data Nudge dengan Jira tidak berhasil; laporan tetap dihitung langsung dari Jira."
			log.Printf("time_report: sync of %s failed: %v", email, err)
		}
	}
	if ctx.Err() != nil || s.cancelled(j) {
		return nil, context.Canceled
	}

	// 2. The tickets created in the range.
	j.set(func(pr *Progress) { pr.Step = 1 })
	projects := s.d.ActiveProjects()
	if p.BoardID > 0 {
		b, found, err := s.d.Boards.Get(ctx, p.BoardID)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, ErrBoardUnknown
		}
		report.BoardName, projects = b.Name, []string{b.ProjectKey}
	}
	report.Projects = append([]string{}, projects...)
	for _, pk := range projects {
		report.Statuses[pk] = append([]string{}, s.d.RuleFor(pk).TrackedStatuses...)
	}
	cred, err := s.d.Creds.ResolveCredential(ctx, email)
	if err != nil {
		cred = nil // the bot's account
	}
	issues, truncated, err := s.d.Jira.IssuesCreatedBetween(ctx, cred, user.JiraAccountID, projects, p.From, toExclusive, MaxTickets)
	if err != nil {
		return nil, err
	}
	report.Truncated = truncated

	// 3. Every ticket's status history, a few side by side.
	j.set(func(pr *Progress) { pr.Step, pr.Total, pr.Done = 2, len(issues), 0 })
	histories := make([][]jira_client_dto.ChangelogEntry, len(issues))
	errs := make([]error, len(issues))
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < changelogWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				if ctx.Err() != nil {
					errs[i] = ctx.Err()
					continue
				}
				histories[i], errs[i] = s.d.Jira.FullChangelog(ctx, cred, issues[i].Key)
				key := issues[i].Key
				j.set(func(pr *Progress) { pr.Done++; pr.Current = key })
			}
		}()
	}
	for i := range issues {
		if s.cancelled(j) {
			break
		}
		next <- i
	}
	close(next)
	wg.Wait()
	if ctx.Err() != nil || s.cancelled(j) {
		return nil, context.Canceled
	}

	// 4. Time, deductions and deviation per ticket.
	j.set(func(pr *Progress) { pr.Step, pr.Total, pr.Done, pr.Current = 3, len(issues), 0, "" })
	national := s.d.National.Holidays
	leave := map[string]bool{}
	if leaves, err := s.d.Leaves.List(ctx, email); err == nil {
		for _, l := range leaves {
			leave[l.Date] = true
		}
	} else {
		log.Printf("time_report: leave of %s not read: %v", email, err)
	}
	base := strings.TrimRight(s.d.JiraBaseURL, "/")
	for i, issue := range issues {
		pk := jira_client_dto.ProjectKeyOf(issue.Key)
		url := base + "/browse/" + issue.Key
		if errs[i] != nil {
			report.Excluded = append(report.Excluded, Excluded{Key: issue.Key, Summary: issue.Fields.Summary, Status: issue.Fields.Status.Name, URL: url,
				Reason: "Riwayat status tidak bisa dibaca dari Jira"})
			continue
		}
		in := ticketInput{
			issue: issue, histories: histories[i], rule: s.d.RuleFor(pk), url: url,
			cal: Calendar{National: national, Leave: leave, Windows: s.d.Schedule(pk).Windows},
		}
		t, reason := buildTicket(in, now, capPercent)
		if reason != "" {
			report.Excluded = append(report.Excluded, Excluded{Key: t.Key, Summary: t.Summary, Status: t.Status, URL: url, Reason: reason, CountedSeconds: t.CountedSeconds})
		} else {
			report.Tickets = append(report.Tickets, t)
		}
		j.set(func(pr *Progress) { pr.Done = i + 1; pr.Current = issue.Key })
	}

	// 5. Totals.
	j.set(func(pr *Progress) { pr.Step, pr.Total, pr.Done, pr.Current = 4, 0, 0, "" })
	sort.SliceStable(report.Tickets, func(a, b int) bool { return report.Tickets[a].Created < report.Tickets[b].Created })
	report.Summary, report.Deductions = summarize(report.Tickets, report.Excluded, capPercent)
	return report, nil
}

// FileName is the PDF's name: Laporan-Nudge-<name>-<from>_<to>.pdf.
func FileName(r *Report) string {
	name := strings.Map(func(c rune) rune {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			return c
		}
		return '-'
	}, strings.TrimSpace(r.UserName))
	name = strings.Trim(name, "-")
	if name == "" {
		name = "user"
	}
	return fmt.Sprintf("Laporan-Nudge-%s-%s_%s.pdf", name, r.From, r.To)
}
