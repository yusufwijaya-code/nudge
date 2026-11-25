// Package jira_account_holder keeps the central Jira account (email + API
// token) in force: the one an admin saved in Admin › Akun Jira. There is no
// environment fallback; until an account exists nothing can call Jira as the
// bot. It has no dependencies, so the Jira clients can read it without
// importing the admin module (which depends on them through auth).
package jira_account_holder

import (
	"log"
	"sync/atomic"
)

// Credential is an Atlassian email + API token.
type Credential struct {
	Email    string
	APIToken string
}

// override is the account in force, read by every Jira client on every
// request, so a change applies at once, without a restart. Same registry style
// as jira_client_dto.SetRules.
var override atomic.Pointer[Credential]

// missingLogged makes "no account" a single log line instead of one per poll.
var missingLogged atomic.Bool

// SetOverride installs the account in force; nil removes it.
func SetOverride(c *Credential) {
	if c == nil {
		override.Store(nil)
		return
	}
	cp := *c
	override.Store(&cp)
	missingLogged.Store(false)
}

// Override is the account in force, nil when none is set.
func Override() *Credential {
	if c := override.Load(); c != nil {
		cp := *c
		return &cp
	}
	return nil
}

// Current is the credential Jira calls authenticate with as the bot. ok is
// false when no account has been set up: the strings are then empty and Jira
// will answer 401.
func Current() (email, token string, ok bool) {
	if c := override.Load(); c != nil {
		return c.Email, c.APIToken, true
	}
	return "", "", false
}

// Configured reports whether a central account is in force.
func Configured() bool { return override.Load() != nil }

// NoteMissing is what a background loop calls when it skips a cycle for lack of
// an account: it logs once, until an account is set.
func NoteMissing(component string) {
	if missingLogged.CompareAndSwap(false, true) {
		log.Printf("%s: no Jira account is set up yet; Jira polling is paused (an admin sets it in Admin > Akun Jira)", component)
	}
}
