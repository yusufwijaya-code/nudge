package jira_client_service

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"nudge/base/helpers/curl_helper"
	"nudge/config"
	"nudge/modules/jira_client/jira_client_dto"
)

// ReportClient reads what the user's Laporan needs: the tickets created in a date
// range and the full status history of each, always with the given credential.
type ReportClient interface {
	// IssuesCreatedBetween lists accountID's tickets in projects created from `from`
	// up to (not including) `toExclusive`, both "YYYY-MM-DD"; at most max tickets,
	// truncated=true when there were more.
	IssuesCreatedBetween(ctx context.Context, cred *jira_client_dto.Credential, accountID string, projects []string, from, toExclusive string, max int) (issues []jira_client_dto.Issue, truncated bool, err error)
	// FullChangelog is every history entry of the ticket, oldest first.
	FullChangelog(ctx context.Context, cred *jira_client_dto.Credential, issueKey string) ([]jira_client_dto.ChangelogEntry, error)
}

type reportClient struct{ base *jiraClientServiceImpl }

func NewReportClient(cfg *config.AppConfig) ReportClient {
	return &reportClient{base: &jiraClientServiceImpl{cfg: cfg}}
}

var (
	reportDay        = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	reportProjectKey = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,19}$`)
)

func (c *reportClient) IssuesCreatedBetween(ctx context.Context, cred *jira_client_dto.Credential, accountID string, projects []string, from, toExclusive string, max int) ([]jira_client_dto.Issue, bool, error) {
	if !reportDay.MatchString(from) || !reportDay.MatchString(toExclusive) {
		return nil, false, fmt.Errorf("jira_client: IssuesCreatedBetween: bad dates %q..%q", from, toExclusive)
	}
	var keys []string
	for _, p := range projects {
		if reportProjectKey.MatchString(p) {
			keys = append(keys, p)
		}
	}
	if len(keys) == 0 || strings.TrimSpace(accountID) == "" {
		return nil, false, nil
	}
	jql := fmt.Sprintf(`project in (%s) AND assignee = %s AND created >= "%s" AND created < "%s" ORDER BY created ASC`,
		strings.Join(keys, ", "), strconv.Quote(accountID), from, toExclusive)
	fields := append([]string{"summary", "status", "issuetype", "created", "parent", jira_client_dto.SprintFieldID}, jira_client_dto.StoryPointFieldIDs()...)
	body := searchJQLRequest{JQL: jql, Fields: fields, MaxResults: 100}
	url := c.base.cfg.JiraBaseURL + "/rest/api/3/search/jql"

	var out []jira_client_dto.Issue
	for page := 0; page < 50; page++ {
		res, err := curl_helper.CurlRequest[jira_client_dto.SearchResponse](ctx, url, c.base.authHeadersFor(cred), body, http.MethodPost, 15000, true)
		if err != nil {
			return nil, false, fmt.Errorf("jira_client: IssuesCreatedBetween failed: %w", err)
		}
		out = append(out, res.Issues...)
		if len(out) >= max {
			return out[:max], len(out) > max || res.NextPageToken != "", nil
		}
		if res.NextPageToken == "" {
			return out, false, nil
		}
		body.NextPageToken = res.NextPageToken
	}
	return out, true, nil
}

func (c *reportClient) FullChangelog(ctx context.Context, cred *jira_client_dto.Credential, issueKey string) ([]jira_client_dto.ChangelogEntry, error) {
	if !jira_client_dto.IsTrackedProjectKey(issueKey) {
		return nil, fmt.Errorf("jira_client: FullChangelog: invalid issue key %q", issueKey)
	}
	const pageSize = 100
	var out []jira_client_dto.ChangelogEntry
	for startAt := 0; startAt < 5000; startAt += pageSize {
		url := fmt.Sprintf("%s/rest/api/3/issue/%s/changelog?startAt=%d&maxResults=%d", c.base.cfg.JiraBaseURL, issueKey, startAt, pageSize)
		page, err := curl_helper.CurlRequest[changelogPage](ctx, url, c.base.authHeadersFor(cred), nil, http.MethodGet, 15000, true)
		if err != nil {
			return nil, fmt.Errorf("jira_client: FullChangelog(%s) failed: %w", issueKey, err)
		}
		out = append(out, page.Values...)
		if len(page.Values) == 0 || startAt+len(page.Values) >= page.Total {
			break
		}
	}
	return out, nil
}
