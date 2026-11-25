package jira_account

import (
	"context"
	"testing"
	"time"

	"nudge/base/testutil"
)

func TestRepository_SavesOneAccountAndOverwritesIt(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	// Uses the shared local jira_db: leave the table as found.
	prev, err := repo.Find(ctx)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Delete(ctx)
		if prev != nil {
			_ = repo.Save(ctx, *prev)
		}
	})
	if err := repo.Delete(ctx); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if rec, err := repo.Find(ctx); err != nil || rec != nil {
		t.Fatalf("empty table: Find = %+v, %v; want nil", rec, err)
	}

	if err := repo.Save(ctx, Record{Email: "a@esb.co.id", TokenEncrypted: []byte("sealed-1"), UpdatedBy: "admin1@esb.co.id"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := repo.Save(ctx, Record{Email: "b@esb.co.id", TokenEncrypted: []byte("sealed-2"), UpdatedBy: "admin2@esb.co.id"}); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	rec, err := repo.Find(ctx)
	if err != nil || rec == nil {
		t.Fatalf("Find = %+v, %v", rec, err)
	}
	if rec.Email != "b@esb.co.id" || string(rec.TokenEncrypted) != "sealed-2" || rec.UpdatedBy != "admin2@esb.co.id" || rec.UpdatedDate.IsZero() {
		t.Errorf("record = %+v, want the second save to replace the first", rec)
	}

	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ms_jira_account`).Scan(&rows); err != nil || rows != 1 {
		t.Errorf("rows = %d, %v; want exactly one central account", rows, err)
	}

	if err := repo.Delete(ctx); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if rec, _ := repo.Find(ctx); rec != nil {
		t.Errorf("Find after Delete = %+v, want nil", rec)
	}
}

func TestRepository_ExpiryIsOneDayThatCanBeReplacedAndForgotten(t *testing.T) {
	db := testutil.OpenTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	prev, err := repo.FindExpiry(ctx)
	if err != nil {
		t.Fatalf("FindExpiry: %v", err)
	}
	t.Cleanup(func() { _ = repo.SaveExpiry(ctx, prev, "test-cleanup") })
	if err := repo.SaveExpiry(ctx, nil, "t"); err != nil {
		t.Fatalf("SaveExpiry(nil): %v", err)
	}
	if day, err := repo.FindExpiry(ctx); err != nil || day != nil {
		t.Fatalf("empty: FindExpiry = %v, %v; want nil", day, err)
	}

	first := time.Date(2027, 10, 26, 0, 0, 0, 0, time.FixedZone("WIB", 7*3600))
	second := time.Date(2028, 1, 5, 0, 0, 0, 0, time.FixedZone("WIB", 7*3600))
	if err := repo.SaveExpiry(ctx, &first, "admin1@esb.co.id"); err != nil {
		t.Fatalf("SaveExpiry: %v", err)
	}
	if err := repo.SaveExpiry(ctx, &second, "admin2@esb.co.id"); err != nil {
		t.Fatalf("second SaveExpiry: %v", err)
	}
	day, err := repo.FindExpiry(ctx)
	if err != nil || day == nil || day.Format("2006-01-02") != "2028-01-05" {
		t.Errorf("FindExpiry = %v, %v; want the second date", day, err)
	}
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ms_jira_account_expiry`).Scan(&rows); err != nil || rows != 1 {
		t.Errorf("rows = %d, %v; want exactly one", rows, err)
	}
}
