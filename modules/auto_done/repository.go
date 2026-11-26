package auto_done

import (
	"context"
	"database/sql"
	"time"
)

// Store is where the switch and the attempts live.
type Store interface {
	// EnabledEmails are the active users who switched the feature on.
	EnabledEmails(ctx context.Context) (map[string]bool, error)
	IsEnabled(ctx context.Context, userID int64) (bool, error)
	SetEnabled(ctx context.Context, userID int64, enabled bool) error
	// Claim records that this run of the ticket is being moved; false when it
	// already was (an earlier cycle, another instance, before a restart).
	Claim(ctx context.Context, issueKey string, runStarted time.Time, email, target string) (bool, error)
	Finish(ctx context.Context, issueKey string, runStarted time.Time, outcome, message string) error
}

type repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) Store { return &repository{db: db} }

func (r *repository) EnabledEmails(ctx context.Context) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT email FROM ms_user WHERE autoDoneEnabled = 1 AND flagActive = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		out[e] = true
	}
	return out, rows.Err()
}

func (r *repository) IsEnabled(ctx context.Context, userID int64) (bool, error) {
	var on bool
	err := r.db.QueryRowContext(ctx, `SELECT autoDoneEnabled FROM ms_user WHERE userID = ?`, userID).Scan(&on)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return on, err
}

func (r *repository) SetEnabled(ctx context.Context, userID int64, enabled bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ms_user SET autoDoneEnabled = ? WHERE userID = ?`, enabled, userID)
	return err
}

func (r *repository) Claim(ctx context.Context, issueKey string, runStarted time.Time, email, target string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT IGNORE INTO tr_autodone_log (issueKey, runStarted, assigneeEmail, targetStatus) VALUES (?, ?, ?, ?)`,
		issueKey, runStarted.UTC(), email, target)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (r *repository) Finish(ctx context.Context, issueKey string, runStarted time.Time, outcome, message string) error {
	if len([]rune(message)) > 480 {
		message = string([]rune(message)[:480])
	}
	_, err := r.db.ExecContext(ctx, `UPDATE tr_autodone_log SET outcome = ?, message = ? WHERE issueKey = ? AND runStarted = ?`,
		outcome, message, issueKey, runStarted.UTC())
	return err
}
