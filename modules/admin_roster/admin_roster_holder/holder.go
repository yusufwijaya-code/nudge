// Package admin_roster_holder knows who may use the admin panel: the owner
// (ADMIN_EMAIL, always) and the users the owner promoted. It has no
// dependencies, so the auth middleware and the /me handler can read it without
// importing the admin_roster module (which depends on them).
package admin_roster_holder

import (
	"strings"
	"sync/atomic"
)

var (
	owner  atomic.Pointer[string]
	admins atomic.Pointer[map[string]bool]
)

func norm(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// SetOwner records the owner's email (the ADMIN_EMAIL of the server).
func SetOwner(email string) {
	e := norm(email)
	owner.Store(&e)
}

// IsOwner: email is the owner. Case-insensitive.
func IsOwner(email string) bool {
	o := owner.Load()
	return o != nil && *o != "" && norm(email) == *o
}

// SetAdmins replaces the promoted admins.
func SetAdmins(emails []string) {
	set := make(map[string]bool, len(emails))
	for _, e := range emails {
		if n := norm(e); n != "" {
			set[n] = true
		}
	}
	admins.Store(&set)
}

// IsPromoted: the owner promoted this user.
func IsPromoted(email string) bool {
	set := admins.Load()
	return set != nil && (*set)[norm(email)]
}

// IsAdmin: the owner, or a user the owner promoted.
func IsAdmin(email string) bool { return IsOwner(email) || IsPromoted(email) }
