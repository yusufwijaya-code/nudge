package jira_account

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"

	"nudge/base/helpers/curl_helper"
)

// Identity is who Jira says a credential belongs to (GET /myself).
type Identity struct {
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
	AccountID    string `json:"accountId"`
	Active       bool   `json:"active"`
}

// ErrRejected means Jira did not accept the credential (401/403).
var ErrRejected = errors.New("jira_account: Jira rejected the credential")

// Checker asks Jira who a credential is.
type Checker interface {
	WhoAmI(ctx context.Context, email, apiToken string) (Identity, error)
}

type jiraChecker struct{ baseURL string }

// NewChecker verifies credentials against baseURL (JIRA_BASE_URL).
func NewChecker(baseURL string) Checker { return &jiraChecker{baseURL: baseURL} }

func (c *jiraChecker) WhoAmI(ctx context.Context, email, apiToken string) (Identity, error) {
	headers := map[string]string{
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+apiToken)),
		"Accept":        "application/json",
	}
	res, err := curl_helper.CurlRequest[Identity](ctx, c.baseURL+"/rest/api/3/myself", headers, nil, http.MethodGet, 10000, true)
	if err != nil {
		var httpErr *curl_helper.HTTPError
		if errors.As(err, &httpErr) && (httpErr.StatusCode == http.StatusUnauthorized || httpErr.StatusCode == http.StatusForbidden) {
			return Identity{}, fmt.Errorf("%w (HTTP %d)", ErrRejected, httpErr.StatusCode)
		}
		return Identity{}, err
	}
	return *res, nil
}
