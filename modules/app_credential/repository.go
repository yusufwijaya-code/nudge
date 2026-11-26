package app_credential

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Row is one stored credential.
type Row struct {
	Key            string
	ValueEncrypted []byte // nil = no value kept here
	ExpiresAt      *time.Time
	UpdatedBy      string
	UpdatedDate    time.Time
}

// Repository is the ms_app_credential table.
type Repository interface {
	List(ctx context.Context) (map[string]Row, error)
	// Get returns nil when the key has no row.
	Get(ctx context.Context, key string) (*Row, error)
	// SaveValue stores the encrypted value and, with it, the expiry day (nil = unknown).
	SaveValue(ctx context.Context, key string, sealed []byte, expiresAt *time.Time, editor string) error
	// SaveExpiry changes only the expiry day, creating the row when needed.
	SaveExpiry(ctx context.Context, key string, expiresAt *time.Time, editor string) error
	// ClearValue forgets the stored value but keeps the row (and its expiry).
	ClearValue(ctx context.Context, key, editor string) error
}

type repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) Repository { return &repository{db: db} }

const cols = `credKey, valueEncrypted, expiresAt, updatedBy, updatedDate`

func scan(sc interface{ Scan(...any) error }) (Row, error) {
	var r Row
	var exp sql.NullTime
	if err := sc.Scan(&r.Key, &r.ValueEncrypted, &exp, &r.UpdatedBy, &r.UpdatedDate); err != nil {
		return Row{}, err
	}
	if exp.Valid {
		t := exp.Time
		r.ExpiresAt = &t
	}
	return r, nil
}

func (r *repository) List(ctx context.Context) (map[string]Row, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+cols+` FROM ms_app_credential`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Row{}
	for rows.Next() {
		row, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out[row.Key] = row
	}
	return out, rows.Err()
}

func (r *repository) Get(ctx context.Context, key string) (*Row, error) {
	row, err := scan(r.db.QueryRowContext(ctx, `SELECT `+cols+` FROM ms_app_credential WHERE credKey = ?`, key))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func day(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.In(time.FixedZone("WIB", 7*3600)).Format("2006-01-02")
}

func (r *repository) SaveValue(ctx context.Context, key string, sealed []byte, expiresAt *time.Time, editor string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO ms_app_credential (credKey, valueEncrypted, expiresAt, updatedBy) VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE valueEncrypted = VALUES(valueEncrypted), expiresAt = VALUES(expiresAt), updatedBy = VALUES(updatedBy)`,
		key, sealed, day(expiresAt), editor)
	return err
}

func (r *repository) SaveExpiry(ctx context.Context, key string, expiresAt *time.Time, editor string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO ms_app_credential (credKey, expiresAt, updatedBy) VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE expiresAt = VALUES(expiresAt), updatedBy = VALUES(updatedBy)`,
		key, day(expiresAt), editor)
	return err
}

func (r *repository) ClearValue(ctx context.Context, key, editor string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ms_app_credential SET valueEncrypted = NULL, updatedBy = ? WHERE credKey = ?`, editor, key)
	return err
}
