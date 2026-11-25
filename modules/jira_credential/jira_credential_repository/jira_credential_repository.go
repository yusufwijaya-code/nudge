package jira_credential_repository

import (
	"context"
	"time"

	"nudge/modules/jira_credential/jira_credential_dto"
)

type JiraCredentialRepository interface {
	// Upsert creates or overwrites userID's personal credential (spec
	// AC5.5 "reset" — a second call for the same user replaces the first,
	// never adds a second row; uq_tr_usercredential_userid enforces this).
	Upsert(ctx context.Context, userID int64, jiraEmail string, encrypted []byte) error
	FindByUserID(ctx context.Context, userID int64) (*jira_credential_dto.Record, bool, error)
	DeleteByUserID(ctx context.Context, userID int64) error
	// SetExpiry records the day userID's token expires (nil clears it).
	SetExpiry(ctx context.Context, userID int64, expiresAt *time.Time) error
	// ListExpiringBefore returns the active personal tokens that expire before
	// cutoff, soonest first (already-expired ones included).
	ListExpiringBefore(ctx context.Context, cutoff time.Time) ([]jira_credential_dto.ExpiringRecord, error)
	// FindAll is used by the Admin Dashboard (Chapter 10b) to show which
	// users have a personal credential set, without exposing the token.
	FindAll(ctx context.Context) ([]jira_credential_dto.Record, error)
}
