package time_report

import (
	"math"
	"sort"
	"strings"
	"time"

	"nudge/modules/jira_client/jira_client_dto"
)

// StatusTime is a ticket's time in one counted status.
type StatusTime struct {
	Status         string `json:"status"`
	RawSeconds     int64  `json:"rawSeconds"`
	CountedSeconds int64  `json:"countedSeconds"`
}

// Ticket is one scored ticket of the report.
type Ticket struct {
	Key         string   `json:"key"`
	Summary     string   `json:"summary"`
	Type        string   `json:"type"`
	TypeIconURL string   `json:"typeIconUrl,omitempty"`
	Status      string   `json:"status"`
	URL         string   `json:"url"`
	Created     string   `json:"created"` // RFC3339
	Sprints     []string `json:"sprints"`
	StoryPoints float64  `json:"storyPoints"`

	EstimateSeconds int64 `json:"estimateSeconds"`
	Breakdown
	DeductedSeconds int64        `json:"deductedSeconds"`
	StatusTimes     []StatusTime `json:"statusTimes"`
	// FirstStart / LastEnd: when the time in the counted statuses began and ended
	// (RFC3339); Running = it is still in a counted status right now.
	FirstStart string `json:"firstStart,omitempty"`
	LastEnd    string `json:"lastEnd,omitempty"`
	Running    bool   `json:"running"`

	// DeviationPercent = |actual − estimate| ÷ estimate × 100; ScorePercent = actual ÷
	// estimate × 100 (the cascading key result); Within = deviation ≤ the cap.
	DeviationPercent float64 `json:"deviationPercent"`
	ScorePercent     float64 `json:"scorePercent"`
	Within           bool    `json:"within"`
}

// Excluded is a ticket of the range that is not scored, and why.
type Excluded struct {
	Key            string `json:"key"`
	Summary        string `json:"summary"`
	Status         string `json:"status"`
	URL            string `json:"url"`
	Reason         string `json:"reason"`
	CountedSeconds int64  `json:"countedSeconds"`
}

// Summary is the report over every scored ticket.
type Summary struct {
	TicketCount int `json:"ticketCount"` // every ticket in the range
	ScoredCount int `json:"scoredCount"`
	WithinCount int `json:"withinCount"`

	EstimateSeconds int64 `json:"estimateSeconds"`
	RawSeconds      int64 `json:"rawSeconds"`
	DeductedSeconds int64 `json:"deductedSeconds"`
	CountedSeconds  int64 `json:"countedSeconds"`

	AggregateDeviationPercent float64 `json:"aggregateDeviationPercent"`
	ScorePercent              float64 `json:"scorePercent"`
	AchievementPercent        float64 `json:"achievementPercent"`
	WithinPercent             float64 `json:"withinPercent"`
	// Within: the total of the range is inside the band (the key result is met).
	Within bool `json:"within"`
}

// Report is the whole Laporan.
type Report struct {
	GeneratedAt string   `json:"generatedAt"` // RFC3339
	UserName    string   `json:"userName"`
	UserEmail   string   `json:"userEmail"`
	BoardID     int64    `json:"boardId"` // 0 = every board
	BoardName   string   `json:"boardName"`
	Projects    []string `json:"projects"`
	From        string   `json:"from"` // YYYY-MM-DD, the created date range
	To          string   `json:"to"`
	CapPercent  float64  `json:"capPercent"`
	// Statuses: the counted statuses per project, for the method notes.
	Statuses map[string][]string `json:"statuses"`

	Summary    Summary     `json:"summary"`
	Deductions []Deduction `json:"deductions"` // totals over the scored tickets
	Tickets    []Ticket    `json:"tickets"`
	Excluded   []Excluded  `json:"excluded"`
	// Truncated: the range had more tickets than one report reads.
	Truncated bool `json:"truncated"`
	// SyncWarning: the sync of Nudge's own data with Jira could not run (the report
	// itself is still read from Jira).
	SyncWarning string `json:"syncWarning,omitempty"`
}

// segment is a stretch in one counted status.
type segment struct {
	status     string
	start, end time.Time
	open       bool
}

const jiraTime = "2006-01-02T15:04:05.000-0700"

// trackedSegments reads the status history: every stretch the ticket spent in a
// counted status, the last one up to now when it is still there. The same rule as
// the time calculation: time starts when the ticket moves into a counted status.
func trackedSegments(histories []jira_client_dto.ChangelogEntry, rule jira_client_dto.TrackingRule, now time.Time) []segment {
	type change struct {
		at time.Time
		to string
	}
	var changes []change
	for _, h := range histories {
		at, err := time.Parse(jiraTime, h.Created)
		if err != nil {
			continue
		}
		for _, item := range h.Items {
			if item.Field == "status" {
				changes = append(changes, change{at, item.ToString})
			}
		}
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].at.Before(changes[j].at) })

	var out []segment
	for i, c := range changes {
		if !rule.IsTracked(c.to) {
			continue
		}
		if i+1 < len(changes) {
			out = append(out, segment{status: c.to, start: c.at, end: changes[i+1].at})
		} else if now.After(c.at) {
			out = append(out, segment{status: c.to, start: c.at, end: now, open: true})
		}
	}
	return out
}

// order: the fixed categories first, then the board slots by name.
func sortDeductions(ds []Deduction) {
	rank := map[string]int{CatOutside: 0, CatLunch: 1, CatWeekend: 2, CatHoliday: 3, CatLeave: 4, CatSlot: 5}
	sort.SliceStable(ds, func(i, j int) bool {
		if rank[ds[i].Category] != rank[ds[j].Category] {
			return rank[ds[i].Category] < rank[ds[j].Category]
		}
		return ds[i].Label < ds[j].Label
	})
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func deviation(estimate, actual int64) float64 {
	if estimate <= 0 {
		return 0
	}
	return math.Abs(float64(actual-estimate)) / float64(estimate) * 100
}

// achievement is the sprint report's formula: 100% while the deviation is within the
// cap, else cap ÷ deviation × 100.
func achievement(capPercent, deviationPercent float64) float64 {
	if deviationPercent <= capPercent {
		return 100
	}
	return capPercent / deviationPercent * 100
}

// sprintStartOn picks the sprint a day belongs to (its start..end), for the board
// slots that only apply in week N of a sprint.
func sprintStartOn(sprints []jira_client_dto.SprintRef) func(time.Time) time.Time {
	type span struct{ start, end time.Time }
	var spans []span
	for _, s := range sprints {
		start, err1 := time.Parse(time.RFC3339, s.StartDate)
		end, err2 := time.Parse(time.RFC3339, s.EndDate)
		if err1 != nil || err2 != nil {
			continue
		}
		spans = append(spans, span{start.In(wib), end.In(wib)})
	}
	return func(day time.Time) time.Time {
		d := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, wib)
		for _, s := range spans {
			from := time.Date(s.start.Year(), s.start.Month(), s.start.Day(), 0, 0, 0, 0, wib)
			if !d.Before(from) && !d.After(s.end) {
				return s.start
			}
		}
		return time.Time{}
	}
}

// ticketInput is one ticket and what was read about it.
type ticketInput struct {
	issue     jira_client_dto.Issue
	histories []jira_client_dto.ChangelogEntry
	rule      jira_client_dto.TrackingRule
	cal       Calendar
	url       string
}

// buildTicket scores one ticket; a reason when it cannot be scored.
func buildTicket(in ticketInput, now time.Time, capPercent float64) (Ticket, string) {
	f := in.issue.Fields
	t := Ticket{
		Key: in.issue.Key, Summary: f.Summary, Type: f.IssueType.Name, TypeIconURL: f.IssueType.IconURL,
		Status: f.Status.Name, URL: in.url, Sprints: []string{},
	}
	if created, err := time.Parse(jiraTime, f.Created); err == nil {
		t.Created = created.In(wib).Format(time.RFC3339)
	}
	sprints := f.Sprints()
	for _, s := range sprints {
		t.Sprints = append(t.Sprints, s.Name)
	}
	if sp := in.issue.StoryPoints(); sp != nil {
		t.StoryPoints = *sp
	}
	t.EstimateSeconds = int64(math.Round(t.StoryPoints * 3600))

	cal := in.cal
	cal.SprintStartOn = sprintStartOn(sprints)
	byStatus := map[string]*Breakdown{}
	var order []string
	for _, seg := range trackedSegments(in.histories, in.rule, now) {
		var piece Breakdown
		cal.Measure(seg.start, seg.end, &piece)
		t.Breakdown.Merge(piece)
		if byStatus[seg.status] == nil {
			byStatus[seg.status] = &Breakdown{}
			order = append(order, seg.status)
		}
		byStatus[seg.status].Merge(piece)
		if t.FirstStart == "" {
			t.FirstStart = seg.start.In(wib).Format(time.RFC3339)
		}
		t.LastEnd = seg.end.In(wib).Format(time.RFC3339)
		t.Running = seg.open
	}
	if t.Deductions == nil {
		t.Deductions = []Deduction{}
	}
	sortDeductions(t.Deductions)
	t.DeductedSeconds = t.Breakdown.DeductedSeconds()
	for _, st := range order {
		b := byStatus[st]
		t.StatusTimes = append(t.StatusTimes, StatusTime{Status: st, RawSeconds: b.RawSeconds, CountedSeconds: b.CountedSeconds})
	}
	if t.StatusTimes == nil {
		t.StatusTimes = []StatusTime{}
	}

	switch {
	case len(order) == 0:
		return t, "Belum pernah masuk status yang dihitung (" + strings.Join(in.rule.TrackedStatuses, ", ") + ")"
	case t.EstimateSeconds <= 0:
		return t, "Belum ada Story Point, jadi tidak ada estimasi untuk dibandingkan"
	}
	t.DeviationPercent = round2(deviation(t.EstimateSeconds, t.CountedSeconds))
	t.ScorePercent = round2(float64(t.CountedSeconds) / float64(t.EstimateSeconds) * 100)
	t.Within = deviation(t.EstimateSeconds, t.CountedSeconds) <= capPercent
	return t, ""
}

// summarize puts the tickets together: the scored ones make the totals.
func summarize(tickets []Ticket, excluded []Excluded, capPercent float64) (Summary, []Deduction) {
	s := Summary{TicketCount: len(tickets) + len(excluded), ScoredCount: len(tickets)}
	var all Breakdown
	for _, t := range tickets {
		s.EstimateSeconds += t.EstimateSeconds
		all.Merge(t.Breakdown)
		if t.Within {
			s.WithinCount++
		}
	}
	s.RawSeconds, s.CountedSeconds, s.DeductedSeconds = all.RawSeconds, all.CountedSeconds, all.DeductedSeconds()
	if s.EstimateSeconds > 0 {
		dev := deviation(s.EstimateSeconds, s.CountedSeconds)
		s.AggregateDeviationPercent = round2(dev)
		s.ScorePercent = round2(float64(s.CountedSeconds) / float64(s.EstimateSeconds) * 100)
		s.AchievementPercent = round2(achievement(capPercent, dev))
		s.Within = dev <= capPercent
	}
	if s.ScoredCount > 0 {
		s.WithinPercent = round2(float64(s.WithinCount) / float64(s.ScoredCount) * 100)
	}
	ds := all.Deductions
	if ds == nil {
		ds = []Deduction{}
	}
	sortDeductions(ds)
	return s, ds
}
