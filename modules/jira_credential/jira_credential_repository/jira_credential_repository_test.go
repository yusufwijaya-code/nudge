package jira_credential_repository

import (
	"context"
	"testing"
	"time"

	"nudge/base/testutil"
)

func TestExpiry_IsStoredAsADayReadBackAndListedByUser(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewJiraCredentialRepository(db)
	ctx := context.Background()

	email := "testmysql-expiry@esb.co.id"
	res, err := db.Exec(`INSERT INTO ms_user (userName, email, flagActive, createdBy) VALUES (?, ?, 1, 'test')`, "TestMySQL Expiry", email)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	userID, _ := res.LastInsertId()
	t.Cleanup(func() {
		db.Exec(`DELETE FROM tr_usercredential WHERE userID = ?`, userID)
		db.Exec(`DELETE FROM ms_user WHERE userID = ?`, userID)
	})

	if err := repo.Upsert(ctx, userID, "jira@esb.co.id", []byte("sealed")); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	rec, found, err := repo.FindByUserID(ctx, userID)
	if err != nil || !found || rec.ExpiresAt != nil {
		t.Fatalf("a new credential has an unknown expiry: %+v, %v, %v", rec, found, err)
	}

	day := time.Date(2026, 11, 30, 0, 0, 0, 0, time.FixedZone("WIB", 7*3600))
	if err := repo.SetExpiry(ctx, userID, &day); err != nil {
		t.Fatalf("SetExpiry: %v", err)
	}
	rec, _, _ = repo.FindByUserID(ctx, userID)
	if rec.ExpiresAt == nil || rec.ExpiresAt.Format("2006-01-02") != "2026-11-30" {
		t.Fatalf("ExpiresAt = %v, want 2026-11-30", rec.ExpiresAt)
	}

	inWindow, err := repo.ListExpiringBefore(ctx, day.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("ListExpiringBefore: %v", err)
	}
	var listed bool
	for _, r := range inWindow {
		if r.UserID == userID {
			listed = true
			if r.Email != email || r.UserName != "TestMySQL Expiry" || r.ExpiresAt.Format("2006-01-02") != "2026-11-30" {
				t.Errorf("listed record = %+v", r)
			}
		}
	}
	if !listed {
		t.Error("a token expiring before the cutoff must be listed")
	}
	outside, _ := repo.ListExpiringBefore(ctx, day.AddDate(0, 0, -1))
	for _, r := range outside {
		if r.UserID == userID {
			t.Error("a token expiring after the cutoff must not be listed")
		}
	}

	if err := repo.SetExpiry(ctx, userID, nil); err != nil {
		t.Fatalf("clear: %v", err)
	}
	rec, _, _ = repo.FindByUserID(ctx, userID)
	if rec.ExpiresAt != nil {
		t.Errorf("ExpiresAt after clearing = %v", rec.ExpiresAt)
	}
}
