package jira_client_service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"nudge/base/helpers/curl_helper"
	"nudge/config"
	"nudge/modules/jira_client/jira_client_dto"
)

// OpenBoard is a board whose active sprint holds tickets assigned to someone,
// whether or not its project has a tracking rule (Aturan Board).
type OpenBoard struct {
	BoardID     int64
	ProjectKey  string // of the first ticket seen; the board's own project wins when known
	SprintID    int64
	SprintName  string
	TicketCount int
}

// BoardDiscoveryClient finds the boards a person works on, across every Jira
// project (the dashboard only looks at projects that already have a rule).
type BoardDiscoveryClient interface {
	OpenBoardsOf(ctx context.Context, cred *jira_client_dto.Credential, accountID string) ([]OpenBoard, error)
}

type boardDiscoveryClient struct{ base *jiraClientServiceImpl }

func NewBoardDiscoveryClient(cfg *config.AppConfig) BoardDiscoveryClient {
	return &boardDiscoveryClient{base: &jiraClientServiceImpl{cfg: cfg}}
}

func (c *boardDiscoveryClient) OpenBoardsOf(ctx context.Context, cred *jira_client_dto.Credential, accountID string) ([]OpenBoard, error) {
	if strings.TrimSpace(accountID) == "" {
		return nil, nil
	}
	jql := fmt.Sprintf(`assignee = "%s" AND sprint in openSprints() ORDER BY updated DESC`, strings.ReplaceAll(accountID, `"`, ``))
	params := url.Values{}
	params.Set("jql", jql)
	params.Set("fields", jira_client_dto.SprintFieldID)
	params.Set("maxResults", "100")
	res, err := curl_helper.CurlRequest[jira_client_dto.SearchResponse](
		ctx, c.base.cfg.JiraBaseURL+"/rest/api/3/search/jql?"+params.Encode(), c.base.authHeadersFor(cred), nil, http.MethodGet, 15000, true,
	)
	if err != nil {
		return nil, fmt.Errorf("jira_client: OpenBoardsOf(%s) failed: %w", accountID, err)
	}
	return groupByBoard(res.Issues), nil
}

// groupByBoard counts the tickets per active-sprint board, in first-seen order.
func groupByBoard(issues []jira_client_dto.Issue) []OpenBoard {
	var out []OpenBoard
	at := map[int64]int{}
	for _, issue := range issues {
		sprint, ok := issue.Fields.ActiveSprint()
		if !ok {
			continue
		}
		i, seen := at[sprint.BoardID]
		if !seen {
			i = len(out)
			at[sprint.BoardID] = i
			out = append(out, OpenBoard{BoardID: sprint.BoardID, ProjectKey: jira_client_dto.ProjectKeyOf(issue.Key), SprintID: sprint.ID, SprintName: sprint.Name})
		}
		out[i].TicketCount++
	}
	return out
}
