package jira_account

import (
	"context"
	"testing"

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
