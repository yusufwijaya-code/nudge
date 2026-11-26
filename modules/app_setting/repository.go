package app_setting

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Row is one stored setting.
type Row struct {
	Value       string
	UpdatedBy   string
	UpdatedDate time.Time
}

// Repository is the ms_app_setting key/value table.
type Repository interface {
	// Get returns nil when the setting was never saved.
	Get(ctx context.Context, key string) (*Row, error)
	Set(ctx context.Context, key, value, editor string) error
}

type repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) Repository { return &repository{db: db} }

func (r *repository) Get(ctx context.Context, key string) (*Row, error) {
	var row Row
	err := r.db.QueryRowContext(ctx, `SELECT settingValue, updatedBy, updatedDate FROM ms_app_setting WHERE settingKey = ?`, key).
		Scan(&row.Value, &row.UpdatedBy, &row.UpdatedDate)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *repository) Set(ctx context.Context, key, value, editor string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO ms_app_setting (settingKey, settingValue, updatedBy) VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE settingValue = VALUES(settingValue), updatedBy = VALUES(updatedBy)`, key, value, editor)
	return err
}
