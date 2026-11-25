package jira_credential_repository

import (
	"context"
	"database/sql"
	"time"

	"nudge/modules/jira_credential/jira_credential_dto"
)

type jiraCredentialRepositoryImpl struct {
	db *sql.DB
}

func NewJiraCredentialRepository(db *sql.DB) JiraCredentialRepository {
	return &jiraCredentialRepositoryImpl{db: db}
}

func (r *jiraCredentialRepositoryImpl) Upsert(ctx context.Context, userID int64, jiraEmail string, encrypted []byte) error {
	const query = `
		INSERT INTO tr_usercredential (userID, jiraEmail, jiraAPITokenEncrypted, flagActive, createdBy, createdDate)
		VALUES (?, ?, ?, 1, ?, CURRENT_TIMESTAMP)
		ON DUPLICATE KEY UPDATE
			jiraEmail = VALUES(jiraEmail),
			jiraAPITokenEncrypted = VALUES(jiraAPITokenEncrypted),
			flagActive = 1,
			editedBy = VALUES(createdBy),
			editedDate = CURRENT_TIMESTAMP
	`
	// createdBy/editedBy record the Jira email the token is FOR, not an
	// admin actor — same self-service convention as registration_repository.
	_, err := r.db.ExecContext(ctx, query, userID, jiraEmail, encrypted, jiraEmail)
	return err
}

func (r *jiraCredentialRepositoryImpl) FindByUserID(ctx context.Context, userID int64) (*jira_credential_dto.Record, bool, error) {
	const query = `
		SELECT userCredentialID, userID, jiraEmail, jiraAPITokenEncrypted, flagActive, createdDate, expiresAt
		FROM tr_usercredential
		WHERE userID = ? AND flagActive = 1
	`
	row := r.db.QueryRowContext(ctx, query, userID)
	return scanRecord(row)
}

func scanRecord(row *sql.Row) (*jira_credential_dto.Record, bool, error) {
	var rec jira_credential_dto.Record
	var expires sql.NullTime
	err := row.Scan(&rec.UserCredentialID, &rec.UserID, &rec.JiraEmail, &rec.JiraAPITokenEncrypted, &rec.FlagActive, &rec.CreatedDate, &expires)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if expires.Valid {
		rec.ExpiresAt = &expires.Time
	}
	return &rec, true, nil
}

func (r *jiraCredentialRepositoryImpl) DeleteByUserID(ctx context.Context, userID int64) error {
	const query = `DELETE FROM tr_usercredential WHERE userID = ?`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

func (r *jiraCredentialRepositoryImpl) FindAll(ctx context.Context) ([]jira_credential_dto.Record, error) {
	const query = `
		SELECT userCredentialID, userID, jiraEmail, jiraAPITokenEncrypted, flagActive, createdDate, expiresAt
		FROM tr_usercredential
		WHERE flagActive = 1
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []jira_credential_dto.Record
	for rows.Next() {
		var rec jira_credential_dto.Record
		var expires sql.NullTime
		if err := rows.Scan(&rec.UserCredentialID, &rec.UserID, &rec.JiraEmail, &rec.JiraAPITokenEncrypted, &rec.FlagActive, &rec.CreatedDate, &expires); err != nil {
			return nil, err
		}
		if expires.Valid {
			rec.ExpiresAt = &expires.Time
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

func (r *jiraCredentialRepositoryImpl) SetExpiry(ctx context.Context, userID int64, expiresAt *time.Time) error {
	var day any
	if expiresAt != nil {
		day = expiresAt.Format(jira_credential_dto.ExpiryDateLayout)
	}
	_, err := r.db.ExecContext(ctx, `UPDATE tr_usercredential SET expiresAt = ? WHERE userID = ?`, day, userID)
	return err
}

func (r *jiraCredentialRepositoryImpl) ListExpiringBefore(ctx context.Context, cutoff time.Time) ([]jira_credential_dto.ExpiringRecord, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.userID, u.userName, u.email, c.expiresAt
		FROM tr_usercredential c
		JOIN ms_user u ON u.userID = c.userID
		WHERE c.flagActive = 1 AND c.expiresAt IS NOT NULL AND c.expiresAt < ?
		ORDER BY c.expiresAt ASC, u.email ASC`, cutoff.Format(jira_credential_dto.ExpiryDateLayout))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []jira_credential_dto.ExpiringRecord
	for rows.Next() {
		var rec jira_credential_dto.ExpiringRecord
		if err := rows.Scan(&rec.UserID, &rec.UserName, &rec.Email, &rec.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}
