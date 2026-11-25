package jira_client_service

import (
	"context"
	"fmt"
	"net/http"

	"nudge/base/helpers/curl_helper"
	"nudge/config"
	"nudge/modules/jira_client/jira_client_dto"
)

// SprintClient reads one sprint (its dates) by id with the bot credential.
// Kept apart from JiraAgileClient so that interface's test fakes stay valid.
type SprintClient interface {
	GetSprint(ctx context.Context, sprintID int64) (jira_client_dto.SprintRef, error)
}

type sprintClient struct{ base *jiraClientServiceImpl }

func NewSprintClient(cfg *config.AppConfig) SprintClient {
	return &sprintClient{base: &jiraClientServiceImpl{cfg: cfg}}
}

func (c *sprintClient) GetSprint(ctx context.Context, sprintID int64) (jira_client_dto.SprintRef, error) {
	u := fmt.Sprintf("%s/rest/agile/1.0/sprint/%d", c.base.cfg.JiraBaseURL, sprintID)
	res, err := curl_helper.CurlRequest[jira_client_dto.SprintRef](ctx, u, c.base.authHeaders(), nil, http.MethodGet, 10000, true)
	if err != nil {
		return jira_client_dto.SprintRef{}, fmt.Errorf("jira_client: GetSprint(%d) failed: %w", sprintID, err)
	}
	return *res, nil
}
