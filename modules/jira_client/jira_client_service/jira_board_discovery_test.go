package jira_client_service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nudge/config"
	"nudge/modules/jira_client/jira_client_dto"
)

func issue(t *testing.T, key string, sprints string) jira_client_dto.Issue {
	t.Helper()
	var is jira_client_dto.Issue
	raw := `{"key":"` + key + `","fields":{"` + jira_client_dto.SprintFieldID + `":` + sprints + `}}`
	if err := json.Unmarshal([]byte(raw), &is); err != nil {
		t.Fatal(err)
	}
	return is
}

func TestGroupByBoard_CountsTicketsPerActiveSprintBoard(t *testing.T) {
	issues := []jira_client_dto.Issue{
		issue(t, "NEW-1", `[{"id":5,"name":"Sprint 1","state":"active","boardId":40}]`),
		issue(t, "NEW-2", `[{"id":4,"name":"Old","state":"closed","boardId":40},{"id":5,"name":"Sprint 1","state":"active","boardId":40}]`),
		issue(t, "CORE-9", `[{"id":7,"name":"Core 24","state":"active","boardId":26}]`),
		issue(t, "X-1", `null`),
		issue(t, "Y-1", `[{"id":3,"name":"Future","state":"future","boardId":50}]`),
	}
	got := groupByBoard(issues)
	if len(got) != 2 {
		t.Fatalf("boards = %+v, want 2 (the board of a closed/future-only or sprint-less ticket is not listed)", got)
	}
	if got[0].BoardID != 40 || got[0].TicketCount != 2 || got[0].ProjectKey != "NEW" || got[0].SprintName != "Sprint 1" {
		t.Fatalf("first = %+v", got[0])
	}
	if got[1].BoardID != 26 || got[1].TicketCount != 1 {
		t.Fatalf("second = %+v", got[1])
	}
}

func TestOpenBoardsOf_AsksJiraAcrossEveryProjectForTheAssignee(t *testing.T) {
	var jql string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jql = r.URL.Query().Get("jql")
		_, _ = w.Write([]byte(`{"issues":[{"key":"NEW-1","fields":{"` + jira_client_dto.SprintFieldID + `":[{"id":5,"name":"S1","state":"active","boardId":40}]}}]}`))
	}))
	defer srv.Close()

	c := NewBoardDiscoveryClient(&config.AppConfig{JiraBaseURL: srv.URL})
	got, err := c.OpenBoardsOf(context.Background(), &jira_client_dto.Credential{Email: "a@esb.co.id", APIToken: "t"}, `acc"1`)
	if err != nil || len(got) != 1 || got[0].BoardID != 40 {
		t.Fatalf("got = %+v, err = %v", got, err)
	}
	if !strings.Contains(jql, `assignee = "acc1"`) || !strings.Contains(jql, "openSprints()") || strings.Contains(jql, "project") {
		t.Fatalf("jql = %q: must be the assignee's open-sprint tickets in every project, with quotes stripped", jql)
	}
	if none, err := c.OpenBoardsOf(context.Background(), nil, " "); err != nil || none != nil {
		t.Fatalf("a user without a Jira account id has no boards: %v %v", none, err)
	}
}
