package jira_account

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"nudge/config"
	"nudge/modules/jira_account/jira_account_holder"
	"nudge/modules/jira_credential/jira_credential_crypto"
	"nudge/modules/jira_credential/jira_credential_dto"
)

type fakeRepo struct {
	expiry     *time.Time
	expiryBy   string
	rec        *Record
	saveCalls  int
	deleteErr  error
	saveErr    error
	findErr    error
	deleteCall int
}

func (f *fakeRepo) Find(context.Context) (*Record, error) { return f.rec, f.findErr }
func (f *fakeRepo) Save(_ context.Context, r Record) error {
	f.saveCalls++
	if f.saveErr != nil {
		return f.saveErr
	}
	r.UpdatedDate = time.Date(2026, 11, 25, 21, 0, 0, 0, time.UTC)
	f.rec = &r
	return nil
}
func (f *fakeRepo) FindExpiry(context.Context) (*time.Time, error) { return f.expiry, nil }
func (f *fakeRepo) SaveExpiry(_ context.Context, day *time.Time, editor string) error {
	f.expiry, f.expiryBy = day, editor
	return nil
}
func (f *fakeRepo) Delete(context.Context) error {
	f.deleteCall++
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.rec = nil
	return nil
}

type fakeChecker struct {
	who     Identity
	err     error
	gotMail string
	gotTok  string
}

func (f *fakeChecker) WhoAmI(_ context.Context, email, token string) (Identity, error) {
	f.gotMail, f.gotTok = email, token
	return f.who, f.err
}

// 32 bytes, base64 — the shape CREDENTIAL_ENCRYPTION_KEY has.
var testKey = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

func newService(t *testing.T, key string, repo *fakeRepo, checker *fakeChecker) *Service {
	t.Helper()
	jira_account_holder.SetOverride(nil)
	t.Cleanup(func() { jira_account_holder.SetOverride(nil) })
	cfg := &config.AppConfig{CredentialEncryptionKey: key, JiraEmail: "env@esb.co.id", JiraAPIToken: "env-token-1234"}
	return NewService(cfg, repo, checker)
}

func TestSave_StoresTheTokenEncryptedAndSwitchesEveryClientAtOnce(t *testing.T) {
	repo, checker := &fakeRepo{}, &fakeChecker{who: Identity{DisplayName: "Bot Nudge", EmailAddress: "bot@esb.co.id", Active: true}}
	svc := newService(t, testKey, repo, checker)

	who, err := svc.Save(context.Background(), "  bot@esb.co.id ", " new-secret-token ", "", "admin@esb.co.id")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if who.DisplayName != "Bot Nudge" {
		t.Errorf("identity = %+v", who)
	}
	if checker.gotMail != "bot@esb.co.id" || checker.gotTok != "new-secret-token" {
		t.Errorf("verified %q / %q, want the trimmed input", checker.gotMail, checker.gotTok)
	}
	if strings.Contains(string(repo.rec.TokenEncrypted), "new-secret-token") {
		t.Error("the token must not be stored in clear text")
	}
	if repo.rec.UpdatedBy != "admin@esb.co.id" {
		t.Errorf("updatedBy = %q", repo.rec.UpdatedBy)
	}
	email, token := jira_account_holder.Current("env@esb.co.id", "env-token-1234")
	if email != "bot@esb.co.id" || token != "new-secret-token" {
		t.Errorf("credential in force = %q / %q, want the saved one at once", email, token)
	}
}

// A typo must not cut the bot off: Jira's verdict comes before anything is
// stored or switched.
func TestSave_NeverStoresOrSwitchesToACredentialJiraRejects(t *testing.T) {
	repo, checker := &fakeRepo{}, &fakeChecker{err: ErrRejected}
	svc := newService(t, testKey, repo, checker)

	if _, err := svc.Save(context.Background(), "bot@esb.co.id", "wrong", "", "admin@esb.co.id"); !errors.Is(err, ErrRejected) {
		t.Fatalf("err = %v, want ErrRejected", err)
	}
	if repo.saveCalls != 0 {
		t.Error("a rejected credential must not be saved")
	}
	if email, token := jira_account_holder.Current("env@esb.co.id", "env-token-1234"); email != "env@esb.co.id" || token != "env-token-1234" {
		t.Errorf("credential in force changed to %q / %q after a rejected save", email, token)
	}
}

func TestSave_RejectsEmptyOrMalformedInputAndAMissingKey(t *testing.T) {
	svc := newService(t, testKey, &fakeRepo{}, &fakeChecker{})
	for _, c := range [][2]string{{"", "t"}, {"bot@esb.co.id", ""}, {"not-an-email", "t"}, {"  ", "  "}} {
		if _, err := svc.Save(context.Background(), c[0], c[1], "", "a"); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("Save(%q, %q) err = %v, want ErrInvalidInput", c[0], c[1], err)
		}
	}
	noKey := newService(t, "", &fakeRepo{}, &fakeChecker{})
	if _, err := noKey.Save(context.Background(), "bot@esb.co.id", "t", "", "a"); !errors.Is(err, ErrEncryptionKeyNotConfigured) {
		t.Errorf("err = %v, want ErrEncryptionKeyNotConfigured", err)
	}
}

func TestLoad_InstallsTheSavedAccountAtStart(t *testing.T) {
	var key [32]byte
	copy(key[:], "0123456789abcdef0123456789abcdef")
	sealed, _ := jira_credential_crypto.Encrypt([]byte("stored-token"), key)
	repo := &fakeRepo{rec: &Record{Email: "bot@esb.co.id", TokenEncrypted: sealed, UpdatedBy: "admin@esb.co.id"}}
	svc := newService(t, testKey, repo, &fakeChecker{})

	if err := svc.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if email, token := jira_account_holder.Current("env@esb.co.id", "env-token-1234"); email != "bot@esb.co.id" || token != "stored-token" {
		t.Errorf("credential in force = %q / %q", email, token)
	}
}

// A changed CREDENTIAL_ENCRYPTION_KEY must not stop the service from booting
// or leave it without a Jira credential.
func TestLoad_FallsBackToEnvWhenTheSavedTokenCannotBeDecrypted(t *testing.T) {
	repo := &fakeRepo{rec: &Record{Email: "bot@esb.co.id", TokenEncrypted: []byte("garbage-not-a-sealed-token-at-all"), UpdatedBy: "x"}}
	svc := newService(t, testKey, repo, &fakeChecker{})

	if err := svc.Load(context.Background()); err != nil {
		t.Fatalf("Load must not fail: %v", err)
	}
	if email, _ := jira_account_holder.Current("env@esb.co.id", "env-token-1234"); email != "env@esb.co.id" {
		t.Errorf("credential in force = %q, want the env fallback", email)
	}
}

func TestLoad_WithNothingSavedKeepsTheEnvCredential(t *testing.T) {
	svc := newService(t, testKey, &fakeRepo{}, &fakeChecker{})
	if err := svc.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if email, _ := jira_account_holder.Current("env@esb.co.id", "env-token-1234"); email != "env@esb.co.id" {
		t.Errorf("credential in force = %q", email)
	}
}

func TestReset_GoesBackToTheEnvPair(t *testing.T) {
	repo := &fakeRepo{}
	svc := newService(t, testKey, repo, &fakeChecker{who: Identity{}})
	if _, err := svc.Save(context.Background(), "bot@esb.co.id", "tok", "", "a"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := svc.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if repo.rec != nil {
		t.Error("the saved row must be deleted")
	}
	if email, token := jira_account_holder.Current("env@esb.co.id", "env-token-1234"); email != "env@esb.co.id" || token != "env-token-1234" {
		t.Errorf("credential in force = %q / %q, want env", email, token)
	}
}

func TestStatus_ShowsTheSourceAndOnlyTheLastFourOfTheToken(t *testing.T) {
	repo := &fakeRepo{}
	svc := newService(t, testKey, repo, &fakeChecker{})

	st, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Source != "env" || st.Email != "env@esb.co.id" || st.TokenHint != "••••••••1234" || !st.EncryptionReady || st.UpdatedBy != "" {
		t.Errorf("env status = %+v", st)
	}

	if _, err := svc.Save(context.Background(), "bot@esb.co.id", "abcdefgh-WXYZ", "", "admin@esb.co.id"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	st, _ = svc.Status(context.Background())
	if st.Source != "database" || st.Email != "bot@esb.co.id" || st.TokenHint != "••••••••WXYZ" || st.UpdatedBy != "admin@esb.co.id" || st.UpdatedDate == nil {
		t.Errorf("database status = %+v", st)
	}
	if strings.Contains(st.TokenHint, "abcdefgh") {
		t.Error("the hint must not reveal more than the last characters")
	}
}

func TestTest_ChecksTheCredentialInForce(t *testing.T) {
	checker := &fakeChecker{who: Identity{DisplayName: "Yusuf Wijaya"}}
	svc := newService(t, testKey, &fakeRepo{}, checker)

	who, err := svc.Test(context.Background())
	if err != nil || who.DisplayName != "Yusuf Wijaya" {
		t.Fatalf("Test = %+v, %v", who, err)
	}
	if checker.gotMail != "env@esb.co.id" || checker.gotTok != "env-token-1234" {
		t.Errorf("checked %q / %q, want the env pair", checker.gotMail, checker.gotTok)
	}
}

func TestSave_StoresTheExpiryAndANewTokenNeverKeepsTheOldDate(t *testing.T) {
	repo := &fakeRepo{}
	svc := newService(t, testKey, repo, &fakeChecker{})

	if _, err := svc.Save(context.Background(), "bot@esb.co.id", "tok-1", "2027-10-26", "admin@esb.co.id"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if repo.expiry == nil || repo.expiry.Format("2006-01-02") != "2027-10-26" || repo.expiryBy != "admin@esb.co.id" {
		t.Fatalf("expiry = %v by %q, want 2027-10-26 by admin", repo.expiry, repo.expiryBy)
	}
	st, _ := svc.Status(context.Background())
	if st.ExpiresAt != "2027-10-26" {
		t.Errorf("status expiresAt = %q", st.ExpiresAt)
	}

	// The next token is a different token: without a date, the old one goes.
	if _, err := svc.Save(context.Background(), "bot@esb.co.id", "tok-2", "", "admin@esb.co.id"); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	if repo.expiry != nil {
		t.Errorf("expiry = %v, want it forgotten with the old token", repo.expiry)
	}
}

func TestSave_RejectsABadExpiryBeforeStoringAnything(t *testing.T) {
	repo := &fakeRepo{}
	checker := &fakeChecker{}
	svc := newService(t, testKey, repo, checker)
	for _, bad := range []string{"26-10-2027", "2027-13-40", "soon", "2099-01-01", "2019-01-01"} {
		if _, err := svc.Save(context.Background(), "bot@esb.co.id", "tok", bad, "a"); !errors.Is(err, jira_credential_dto.ErrInvalidExpiry) {
			t.Errorf("Save(expiry %q) err = %v, want ErrInvalidExpiry", bad, err)
		}
	}
	if repo.saveCalls != 0 || checker.gotMail != "" {
		t.Error("a bad date must be refused before Jira is asked or anything is stored")
	}
}

func TestSetExpiry_ChangesOnlyTheDateAndAcceptsAnAlreadyPastOne(t *testing.T) {
	repo := &fakeRepo{}
	svc := newService(t, testKey, repo, &fakeChecker{})

	if err := svc.SetExpiry(context.Background(), "2026-11-23", "admin@esb.co.id"); err != nil {
		t.Fatalf("SetExpiry: %v", err)
	}
	if repo.expiry == nil || repo.saveCalls != 0 {
		t.Errorf("expiry = %v, token saves = %d; want the date alone", repo.expiry, repo.saveCalls)
	}
	if err := svc.SetExpiry(context.Background(), "", "admin@esb.co.id"); err != nil || repo.expiry != nil {
		t.Errorf("clearing: err = %v, expiry = %v", err, repo.expiry)
	}
	if err := svc.SetExpiry(context.Background(), "nope", "a"); !errors.Is(err, jira_credential_dto.ErrInvalidExpiry) {
		t.Errorf("err = %v", err)
	}
}

func TestReset_ForgetsTheExpiryOfTheRemovedToken(t *testing.T) {
	repo := &fakeRepo{}
	svc := newService(t, testKey, repo, &fakeChecker{})
	if _, err := svc.Save(context.Background(), "bot@esb.co.id", "tok", "2027-10-26", "a"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := svc.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if repo.expiry != nil {
		t.Errorf("expiry = %v after Reset, want none", repo.expiry)
	}
}

type fakeExpiring struct {
	records []jira_credential_dto.ExpiringRecord
	within  time.Duration
}

func (f *fakeExpiring) ListExpiring(_ context.Context, within time.Duration) ([]jira_credential_dto.ExpiringRecord, error) {
	f.within = within
	return f.records, nil
}

func TestExpiringUsers_ListsThePersonalTokensWithinTheWarningWindow(t *testing.T) {
	src := &fakeExpiring{records: []jira_credential_dto.ExpiringRecord{
		{UserID: 3, UserName: "Keva", Email: "keva.galih@esb.co.id", ExpiresAt: time.Date(2026, 11, 30, 0, 0, 0, 0, time.FixedZone("WIB", 7*3600))},
	}}
	svc := newService(t, testKey, &fakeRepo{}, &fakeChecker{}).WithExpiringUsers(src)

	users, err := svc.ExpiringUsers(context.Background())
	if err != nil || len(users) != 1 {
		t.Fatalf("ExpiringUsers = %+v, %v", users, err)
	}
	if users[0].Email != "keva.galih@esb.co.id" || users[0].Name != "Keva" || users[0].ExpiresAt != "2026-11-30" {
		t.Errorf("user = %+v", users[0])
	}
	if src.within != ExpiryWarnDays*24*time.Hour {
		t.Errorf("window = %v, want %d days", src.within, ExpiryWarnDays)
	}
	if none, err := newService(t, testKey, &fakeRepo{}, &fakeChecker{}).ExpiringUsers(context.Background()); err != nil || len(none) != 0 {
		t.Errorf("without a source: %+v, %v", none, err)
	}
}
