package time_report

import (
	"bytes"

	"encoding/json"
	"github.com/go-pdf/fpdf"
	"os"
	"strings"
	"testing"
	"time"

	"nudge/modules/business_hours/business_hours_dto"
	"nudge/modules/jira_client/jira_client_dto"
)

var coreRule = jira_client_dto.TrackingRule{ProjectKey: "CORE", TrackedStatuses: []string{"Code Process", "CODE REVIEW"}, DoneStatus: "Code Completed", ReviewStatus: "CODE REVIEW"}

func change(at time.Time, from, to string) jira_client_dto.ChangelogEntry {
	return jira_client_dto.ChangelogEntry{Created: at.Format(jiraTime), Items: []jira_client_dto.ChangelogEntryItem{{Field: "status", FromString: from, ToString: to}}}
}

func TestTrackedSegments_FollowTheStatusHistoryAndKeepAnOpenOneToNow(t *testing.T) {
	now := at(2026, 11, 27, 10, 0)
	hist := []jira_client_dto.ChangelogEntry{
		change(at(2026, 11, 24, 9, 0), "To Do", "Code Process"),
		change(at(2026, 11, 24, 15, 0), "Code Process", "CODE REVIEW"),
		{Created: at(2026, 11, 24, 15, 30).Format(jiraTime), Items: []jira_client_dto.ChangelogEntryItem{{Field: "assignee", ToString: "x"}}}, // not a status change
		change(at(2026, 11, 25, 10, 0), "CODE REVIEW", "To Do"),
		change(at(2026, 11, 26, 9, 0), "To Do", "Code Process"),
	}
	segs := trackedSegments(hist, coreRule, now)
	if len(segs) != 3 {
		t.Fatalf("segments = %+v", segs)
	}
	if segs[0].status != "Code Process" || !segs[0].end.Equal(at(2026, 11, 24, 15, 0)) || segs[1].status != "CODE REVIEW" || segs[1].open {
		t.Fatalf("segments = %+v", segs)
	}
	if !segs[2].open || !segs[2].end.Equal(now) {
		t.Fatalf("the ticket is still in Code Process: the last segment runs to now, got %+v", segs[2])
	}
}

func issueWith(key string, sp float64, created time.Time) jira_client_dto.Issue {
	raw := `{"summary":"Ticket ` + key + `","status":{"name":"Code Completed"},"issuetype":{"name":"Task"},"created":"` + created.Format(jiraTime) + `",
	  "customfield_10020":[{"id":7,"name":"Core Sprint 24","state":"active","boardId":26,"startDate":"2026-11-23T02:00:00.000Z","endDate":"2026-12-04T10:00:00.000Z"}],
	  "customfield_10016":` + jsonNum(sp) + `}`
	var f jira_client_dto.IssueFields
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		panic(err)
	}
	return jira_client_dto.Issue{Key: key, Fields: f}
}

func jsonNum(v float64) string { b, _ := json.Marshal(v); return string(b) }

func withSPRule(t *testing.T) {
	t.Helper()
	jira_client_dto.SetRules([]jira_client_dto.TrackingRule{{ProjectKey: "CORE", TrackedStatuses: coreRule.TrackedStatuses, DoneStatus: "Code Completed", ReviewStatus: "CODE REVIEW", StoryPointFields: []string{"customfield_10016"}, StoryPointWriteField: "customfield_10016", StoryPointWriteFieldSubtask: "customfield_10016"}})
}

func TestBuildTicket_ScoresAgainstTheEstimateWithEveryDeduction(t *testing.T) {
	withSPRule(t)
	defer jira_client_dto.SetRules(nil)
	// 4 SP. Code Process Tue 24 Nov 08:00 → Thu 26 Nov 10:00 with a DSU on Thursdays.
	hist := []jira_client_dto.ChangelogEntry{
		change(at(2026, 11, 24, 8, 0), "To Do", "Code Process"),
		change(at(2026, 11, 26, 10, 0), "Code Process", "Code Completed"),
	}
	cal := Calendar{Windows: []business_hours_dto.ExcludedWindow{{Kind: "dsu", Weekday: time.Thursday, StartMinute: 8*60 + 45, Minutes: 30}}}
	tk, reason := buildTicket(ticketInput{issue: issueWith("CORE-1", 4, at(2026, 11, 23, 10, 0)), histories: hist, rule: coreRule, cal: cal, url: "u"}, at(2026, 11, 27, 9, 0), 12)
	if reason != "" {
		t.Fatalf("reason = %q", reason)
	}
	// Tue 08:30–17:30 (8 h) + Wed (8 h) + Thu 08:30–10:00 minus the 30 min DSU (1 h) = 17 h.
	if tk.CountedSeconds != 17*3600 {
		t.Fatalf("counted = %d", tk.CountedSeconds)
	}
	if tk.RawSeconds != 50*3600 || tk.RawSeconds != tk.CountedSeconds+tk.DeductedSeconds {
		t.Fatalf("raw = %d deducted = %d", tk.RawSeconds, tk.DeductedSeconds)
	}
	if dedOf(tk.Breakdown, "slot:Daily Stand Up") != 1800 || dedOf(tk.Breakdown, CatLunch) != 2*3600 {
		t.Fatalf("deductions = %+v", tk.Deductions)
	}
	if tk.EstimateSeconds != 4*3600 || tk.ScorePercent != 425 || tk.DeviationPercent != 325 || tk.Within {
		t.Fatalf("estimate %d score %v deviation %v within %v", tk.EstimateSeconds, tk.ScorePercent, tk.DeviationPercent, tk.Within)
	}
	if len(tk.Sprints) != 1 || tk.Sprints[0] != "Core Sprint 24" || tk.Running || len(tk.StatusTimes) != 1 {
		t.Fatalf("ticket = %+v", tk)
	}
	if tk.Deductions[0].Category != CatOutside {
		t.Fatalf("the fixed categories come first: %+v", tk.Deductions)
	}
}

func TestBuildTicket_ExplainsWhyATicketIsNotScored(t *testing.T) {
	withSPRule(t)
	defer jira_client_dto.SetRules(nil)
	never := []jira_client_dto.ChangelogEntry{change(at(2026, 11, 24, 8, 0), "To Do", "Code Completed")}
	if _, reason := buildTicket(ticketInput{issue: issueWith("CORE-2", 4, at(2026, 11, 23, 10, 0)), histories: never, rule: coreRule}, at(2026, 11, 27, 9, 0), 12); !strings.Contains(reason, "Belum pernah masuk status yang dihitung") {
		t.Fatalf("reason = %q", reason)
	}
	worked := []jira_client_dto.ChangelogEntry{change(at(2026, 11, 24, 9, 0), "To Do", "Code Process"), change(at(2026, 11, 24, 11, 0), "Code Process", "Code Completed")}
	tk, reason := buildTicket(ticketInput{issue: issueWith("CORE-3", 0, at(2026, 11, 23, 10, 0)), histories: worked, rule: coreRule}, at(2026, 11, 27, 9, 0), 12)
	if !strings.Contains(reason, "Belum ada Story Point") || tk.CountedSeconds != 2*3600 {
		t.Fatalf("reason = %q counted = %d", reason, tk.CountedSeconds)
	}
}

func TestSummarize_TotalsDeviationAchievementAndBand(t *testing.T) {
	mk := func(key string, est, counted int64, within bool) Ticket {
		return Ticket{Key: key, EstimateSeconds: est, Within: within, Breakdown: Breakdown{RawSeconds: counted + 3600, CountedSeconds: counted, Deductions: []Deduction{{Key: CatLunch, Label: labelLunch, Category: CatLunch, Seconds: 3600}}}}
	}
	tickets := []Ticket{mk("A", 10*3600, 9*3600, true), mk("B", 10*3600, 13*3600, false)}
	s, ds := summarize(tickets, []Excluded{{Key: "C"}}, 12)
	if s.TicketCount != 3 || s.ScoredCount != 2 || s.WithinCount != 1 || s.WithinPercent != 50 {
		t.Fatalf("counts = %+v", s)
	}
	// 22 h of 20 h: score 110%, deviation 10% (inside 12%): achievement 100%.
	if s.ScorePercent != 110 || s.AggregateDeviationPercent != 10 || s.AchievementPercent != 100 || !s.Within {
		t.Fatalf("summary = %+v", s)
	}
	if len(ds) != 1 || ds[0].Seconds != 7200 || s.DeductedSeconds != 7200 || s.RawSeconds != 24*3600 {
		t.Fatalf("deductions = %+v summary = %+v", ds, s)
	}
	// 30 h of 20 h: deviation 50% → achievement 12 / 50 = 24%.
	s, _ = summarize([]Ticket{mk("A", 20*3600, 30*3600, false)}, nil, 12)
	if s.AchievementPercent != 24 || s.Within {
		t.Fatalf("summary = %+v", s)
	}
	if s, _ = summarize(nil, nil, 12); s.ScorePercent != 0 || s.AchievementPercent != 0 {
		t.Fatalf("an empty report must not invent a score: %+v", s)
	}
}

func sampleReport() *Report {
	tk := func(key, title string, est, counted int64, within bool, running bool) Ticket {
		return Ticket{
			Key: key, Summary: title, Type: "Task", Status: "Code Completed", Created: "2026-11-03T10:00:00+07:00", Sprints: []string{"Core Sprint 23 2026"},
			StoryPoints: float64(est) / 3600, EstimateSeconds: est, Running: running,
			Breakdown: Breakdown{RawSeconds: counted + 15*3600 + 1800, CountedSeconds: counted, Deductions: []Deduction{
				{Key: CatOutside, Label: labelOutside, Category: CatOutside, Seconds: 12 * 3600},
				{Key: CatLunch, Label: labelLunch, Category: CatLunch, Seconds: 2 * 3600},
				{Key: "slot:Daily Stand Up", Label: labelDSU, Category: CatSlot, Seconds: 1800},
				{Key: CatWeekend, Label: labelWeekend, Category: CatWeekend, Seconds: 3600},
			}},
			DeductedSeconds:  15*3600 + 1800,
			StatusTimes:      []StatusTime{{Status: "Code Process", RawSeconds: counted + 10*3600, CountedSeconds: counted - 3600}, {Status: "CODE REVIEW", RawSeconds: 5*3600 + 1800, CountedSeconds: 3600}},
			DeviationPercent: deviation(est, counted), ScorePercent: float64(counted) / float64(est) * 100, Within: within,
		}
	}
	r := &Report{
		GeneratedAt: "2026-11-27T09:15:00+07:00", UserName: "Yusuf Wijaya", UserEmail: "yusuf.wijaya@esb.co.id", BoardID: 26, BoardName: "ESB Core Board",
		Projects: []string{"CORE"}, From: "2026-11-01", To: "2026-11-27", CapPercent: 12,
		Statuses: map[string][]string{"CORE": {"Code Process", "CODE REVIEW"}},
		Tickets: []Ticket{
			tk("CORE-11361", "[HOTFIX] Perhitungan tarif depresiasi Double Declining di go-core-api membuang desimal 💥", 4*3600, 4*3600+1200, true, false),
			tk("CORE-11374", "[HOTFIX] Upload Simple Manufacturing di go-core-api memproses validasi dan penyimpanan", 4*3600, 6*3600, false, true),
			tk("CORE-9549", "[Revamp] Master Project — dengan judul yang sangat panjang sekali supaya terpotong rapi di tabel ringkasan", 17*3600, 18*3600+1500, true, false),
		},
		Excluded: []Excluded{{Key: "CORE-8253", Summary: "Retouch – VAT Revamp", Reason: "Belum ada Story Point, jadi tidak ada estimasi untuk dibandingkan", CountedSeconds: 3 * 3600}},
	}
	r.Summary, r.Deductions = summarize(r.Tickets, r.Excluded, r.CapPercent)
	return r
}

func TestRenderPDF_ProducesAValidDocumentEvenForAnEmptyReport(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderPDF(&buf, sampleReport()); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) || buf.Len() < 3000 {
		t.Fatalf("not a PDF (%d bytes)", buf.Len())
	}
	if out := os.Getenv("NUDGE_REPORT_SAMPLE"); out != "" {
		_ = os.WriteFile(out, buf.Bytes(), 0o644)
	}

	empty := &Report{GeneratedAt: "2026-11-27T09:15:00+07:00", UserName: "Dev", BoardName: "Semua board", From: "2026-11-01", To: "2026-11-02", CapPercent: 12}
	empty.Summary, empty.Deductions = summarize(nil, nil, 12)
	buf.Reset()
	if err := RenderPDF(&buf, empty); err != nil || !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) {
		t.Fatalf("empty report: err = %v", err)
	}
}

func TestSafeTranslator_DropsWhatTheFontCannotShow(t *testing.T) {
	tr := safeTranslator(fpdfTranslator())
	if got := tr("membuang desimal 💥 lagi"); got != tr("membuang desimal lagi") {
		t.Fatalf("got %q: an emoji must be dropped, not shown as a dot", got)
	}
	if got := tr("a − b ≤ c"); got != tr("a - b <= c") {
		t.Fatalf("got %q", got)
	}
	if got := tr("Retouch – VAT · 12%"); len(got) != len("Retouch - VAT . 12%") {
		t.Fatalf("en dash and middle dot must stay single characters: %q", got)
	}
}

func TestFileName(t *testing.T) {
	if got := FileName(&Report{UserName: "Yusuf Wijaya", From: "2026-11-01", To: "2026-11-27"}); got != "Laporan-Nudge-Yusuf-Wijaya-2026-11-01_2026-11-27.pdf" {
		t.Fatalf("got %q", got)
	}
	if got := FileName(&Report{UserName: "  ", From: "a", To: "b"}); got != "Laporan-Nudge-user-a_b.pdf" {
		t.Fatalf("got %q", got)
	}
}

func TestHMAndFormats(t *testing.T) {
	cases := map[int64]string{0: "0m", 59: "1m", 45 * 60: "45m", 3 * 3600: "3j", 3*3600 + 5*60: "3j 05m", -(2*3600 + 30*60): "-2j 30m"}
	for in, want := range cases {
		if got := hm(in); got != want {
			t.Errorf("hm(%d) = %q, want %q", in, got, want)
		}
	}
	if pct(94.126) != "94,13%" || pct1(12) != "12%" || pct1(12.5) != "12,5%" || hours(45000) != "12,50 jam" {
		t.Fatalf("formats: %s %s %s %s", pct(94.126), pct1(12), pct1(12.5), hours(45000))
	}
}

func fpdfTranslator() func(string) string {
	return fpdf.New("P", "mm", "A4", "").UnicodeTranslatorFromDescriptor("")
}
