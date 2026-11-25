package jira_credential_dto

import (
	"fmt"
	"strings"
	"time"
)

// Record is one tr_usercredential row — the encrypted token is kept as raw
// bytes here too (decryption happens in jira_credential_service, never in
// the repository layer).
type Record struct {
	UserCredentialID      int64
	UserID                int64
	JiraEmail             string
	JiraAPITokenEncrypted []byte
	FlagActive            bool
	CreatedDate           time.Time
	// ExpiresAt is the day the token expires, as entered by the user (the
	// token string is opaque and Jira does not report it). nil = unknown.
	ExpiresAt *time.Time
}

// ExpiringRecord is a personal token that is close to, or past, its expiry:
// what the admin banner lists.
type ExpiringRecord struct {
	UserID    int64
	UserName  string
	Email     string
	ExpiresAt time.Time
}

// ExpiryDateLayout is the wire and storage format of an expiry day.
const ExpiryDateLayout = "2006-01-02"

// ErrInvalidExpiry is returned for an expiry that is not a plausible date.
var ErrInvalidExpiry = fmt.Errorf("jira_credential: expiry must be a date as YYYY-MM-DD, within the next 10 years")

// jakarta is the calendar the expiry day is read in (what Atlassian shows the
// user), whatever the server clock says.
var jakarta = time.FixedZone("WIB", 7*3600)

// ParseExpiryDate reads "YYYY-MM-DD" as that day in WIB. An empty string means
// "no expiry known" (nil). Past dates are accepted (an already expired token
// still deserves a banner); dates beyond ten years ahead are typos.
func ParseExpiryDate(raw string, now time.Time) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	day, err := time.ParseInLocation(ExpiryDateLayout, raw, jakarta)
	if err != nil || day.Year() < 2020 || day.After(now.In(jakarta).AddDate(10, 0, 0)) {
		return nil, ErrInvalidExpiry
	}
	return &day, nil
}

// FormatExpiryDate is the "YYYY-MM-DD" of an expiry day; "" for nil.
func FormatExpiryDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.In(jakarta).Format(ExpiryDateLayout)
}
