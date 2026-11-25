// Package jira_account_holder keeps the central Jira account (email + API
// token) in force: the one an admin saved in Admin › Akun Jira, else the
// JIRA_EMAIL / JIRA_API_TOKEN pair from the environment. It has no
// dependencies, so the Jira clients can read it without importing the admin
// module (which depends on them through auth).
package jira_account_holder

import "sync/atomic"

// Credential is an Atlassian email + API token.
type Credential struct {
	Email    string
	APIToken string
}

// override is what an admin saved, read by every Jira client on every request,
// so a change applies at once, without a restart. Same registry style as
// jira_client_dto.SetRules.
var override atomic.Pointer[Credential]

// SetOverride installs the admin-saved credential; nil goes back to the
// environment pair.
func SetOverride(c *Credential) {
	if c == nil {
		override.Store(nil)
		return
	}
	cp := *c
	override.Store(&cp)
}

// Override is the admin-saved credential, nil when none is in force.
func Override() *Credential {
	if c := override.Load(); c != nil {
		cp := *c
		return &cp
	}
	return nil
}

// Current is the credential Jira calls authenticate with: the admin-saved
// one when there is one, else the environment pair.
func Current(envEmail, envToken string) (email, token string) {
	if c := override.Load(); c != nil {
		return c.Email, c.APIToken
	}
	return envEmail, envToken
}
