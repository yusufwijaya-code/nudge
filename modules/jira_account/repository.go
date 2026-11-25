package jira_account

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Record is the stored central account; the token stays encrypted.
type Record struct {
	Email          string
	TokenEncrypted []byte
	UpdatedBy      string
	UpdatedDate    time.Time
}

// Repository is the single-row ms_jira_account table.
type Repository interface {
	// Find returns nil when no account has been saved.
	Find(ctx context.Context) (*Record, error)
	Save(ctx context.Context, r Record) error
	Delete(ctx context.Context) error
}

type repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) Repository { return &repository{db: db} }

func (r *repository) Find(ctx context.Context) (*Record, error) {
	var rec Record
	err := r.db.QueryRowContext(ctx,
		`SELECT jiraEmail, jiraAPITokenEncrypted, updatedBy, updatedDate FROM ms_jira_account WHERE accountSlot = 1`,
	).Scan(&rec.Email, &rec.TokenEncrypted, &rec.UpdatedBy, &rec.UpdatedDate)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *repository) Save(ctx context.Context, rec Record) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO ms_jira_account (accountSlot, jiraEmail, jiraAPITokenEncrypted, updatedBy)
		VALUES (1, ?, ?, ?)
		ON DUPLICATE KEY UPDATE jiraEmail = VALUES(jiraEmail), jiraAPITokenEncrypted = VALUES(jiraAPITokenEncrypted), updatedBy = VALUES(updatedBy)`,
		rec.Email, rec.TokenEncrypted, rec.UpdatedBy)
	return err
}

func (r *repository) Delete(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM ms_jira_account WHERE accountSlot = 1`)
	return err
}
