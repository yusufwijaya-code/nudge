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
	expiry    *time.Time
	expiryBy  string
	rec       *Record
	saveCalls int
	saveErr   error
	findErr   error
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

// 32 bytes, base64: the shape CREDENTIAL_ENCRYPTION_KEY has.
var testKey = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

func sealed(t *testing.T, token string) []byte {
	t.Helper()
	var key [32]byte
	copy(key[:], "0123456789abcdef0123456789abcdef")
	out, err := jira_credential_crypto.Encrypt([]byte(token), key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	return out
}

func newService(t *testing.T, key string, repo *fakeRepo, checker *fakeChecker) *Service {
	t.Helper()
	jira_account_holder.SetOverride(nil)
	t.Cleanup(func() { jira_account_holder.SetOverride(nil) })
	return NewService(&config.AppConfig{CredentialEncryptionKey: key}, repo, checker)
}

// newLegacyService is a server that still has JIRA_EMAIL / JIRA_API_TOKEN.
func newLegacyService(t *testing.T, key string, repo *fakeRepo) *Service {
	t.Helper()
	jira_account_holder.SetOverride(nil)
	t.Cleanup(func() { jira_account_holder.SetOverride(nil) })
	return NewService(&config.AppConfig{CredentialEncryptionKey: key, LegacyJiraEmail: "legacy@esb.co.id", LegacyJiraAPIToken: "legacy-token-1234"}, repo, &fakeChecker{})
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
	if email, token, ok := jira_account_holder.Current(); !ok || email != "bot@esb.co.id" || token != "new-secret-token" {
		t.Errorf("credential in force = %q / %q / %v, want the saved one at once", email, token, ok)
	}
}

// A typo must not cut the bot off: Jira's verdict comes before anything is
// stored or switched.
func TestSave_NeverStoresOrSwitchesToACredentialJiraRejects(t *testing.T) {
	repo, checker := &fakeRepo{}, &fakeChecker{err: ErrRejected}
	svc := newService(t, testKey, repo, checker)
	jira_account_holder.SetOverride(&jira_account_holder.Credential{Email: "good@esb.co.id", APIToken: "good-token"})

	if _, err := svc.Save(context.Background(), "bot@esb.co.id", "wrong", "", "admin@esb.co.id"); !errors.Is(err, ErrRejected) {
		t.Fatalf("err = %v, want ErrRejected", err)
	}
	if repo.saveCalls != 0 {
		t.Error("a rejected credential must not be saved")
	}
	if email, _, _ := jira_account_holder.Current(); email != "good@esb.co.id" {
		t.Errorf("credential in force changed to %q after a rejected save", email)
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

func TestLoad_PutsTheSavedAccountInForceAtStart(t *testing.T) {
	repo := &fakeRepo{rec: &Record{Email: "bot@esb.co.id", TokenEncrypted: sealed(t, "stored-token"), UpdatedBy: "admin@esb.co.id"}}
	svc := newService(t, testKey, repo, &fakeChecker{})

	if err := svc.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if email, token, ok := jira_account_holder.Current(); !ok || email != "bot@esb.co.id" || token != "stored-token" {
		t.Errorf("credential in force = %q / %q / %v", email, token, ok)
	}
}

// There is no environment fallback: without an account nothing can act as the bot.
func TestLoad_WithNothingSavedAndNoLegacyPairLeavesNoAccountInForce(t *testing.T) {
	svc := newService(t, testKey, &fakeRepo{}, &fakeChecker{})
	if err := svc.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if jira_account_holder.Configured() {
		t.Error("no account must be in force")
	}
	st, _ := svc.Status(context.Background())
	if st.Source != "none" || st.Email != "" || st.TokenHint != "" || st.Unreadable {
		t.Errorf("status = %+v, want none", st)
	}
}

// A server that still has JIRA_EMAIL / JIRA_API_TOKEN keeps working through the
// move: the pair is carried into the database once.
func TestLoad_CarriesTheLegacyEnvPairIntoTheDatabaseOnce(t *testing.T) {
	repo := &fakeRepo{}
	svc := newLegacyService(t, testKey, repo)

	if err := svc.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if email, token, ok := jira_account_holder.Current(); !ok || email != "legacy@esb.co.id" || token != "legacy-token-1234" {
		t.Errorf("credential in force = %q / %q / %v", email, token, ok)
	}
	if repo.rec == nil || repo.rec.Email != "legacy@esb.co.id" || repo.rec.UpdatedBy != "env-import" {
		t.Fatalf("stored = %+v, want the carried-over account", repo.rec)
	}
	if strings.Contains(string(repo.rec.TokenEncrypted), "legacy-token-1234") {
		t.Error("the carried-over token must be stored encrypted")
	}

	// From now on the database is the source: a changed env pair is ignored.
	svc2 := NewService(&config.AppConfig{CredentialEncryptionKey: testKey, LegacyJiraEmail: "other@esb.co.id", LegacyJiraAPIToken: "other-token"}, repo, &fakeChecker{})
	jira_account_holder.SetOverride(nil)
	if err := svc2.Load(context.Background()); err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if email, _, _ := jira_account_holder.Current(); email != "legacy@esb.co.id" {
		t.Errorf("credential in force = %q, want the stored account, not the env pair", email)
	}
	if repo.saveCalls != 1 {
		t.Errorf("saves = %d, want the carry-over to happen once", repo.saveCalls)
	}
}

func TestLoad_KeepsTheLegacyPairInMemoryWhenItCannotBeEncrypted(t *testing.T) {
	repo := &fakeRepo{}
	svc := newLegacyService(t, "", repo)
	if err := svc.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if email, _, ok := jira_account_holder.Current(); !ok || email != "legacy@esb.co.id" {
		t.Errorf("the bot must keep working: %q / %v", email, ok)
	}
	if repo.saveCalls != 0 {
		t.Error("nothing can be stored without an encryption key")
	}
}

// A changed CREDENTIAL_ENCRYPTION_KEY must not stop the service from booting;
// the admin is told and saves the account again.
func TestLoad_ReportsAStoredAccountThatCannotBeDecrypted(t *testing.T) {
	repo := &fakeRepo{rec: &Record{Email: "bot@esb.co.id", TokenEncrypted: []byte("garbage-not-a-sealed-token-at-all"), UpdatedBy: "x"}}
	svc := newService(t, testKey, repo, &fakeChecker{})

	if err := svc.Load(context.Background()); err != nil {
		t.Fatalf("Load must not fail: %v", err)
	}
	if jira_account_holder.Configured() {
		t.Error("an unreadable account must not be in force")
	}
	st, _ := svc.Status(context.Background())
	if st.Source != "none" || !st.Unreadable {
		t.Errorf("status = %+v, want none + unreadable", st)
	}

	// Saving a new one replaces it and clears the warning.
	if _, err := svc.Save(context.Background(), "bot@esb.co.id", "new-token", "", "admin@esb.co.id"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if st, _ := svc.Status(context.Background()); st.Unreadable || st.Source != "database" {
		t.Errorf("status after saving = %+v", st)
	}
}

func TestReset_RemovesTheAccountAndItsDate(t *testing.T) {
	repo := &fakeRepo{}
	svc := newService(t, testKey, repo, &fakeChecker{who: Identity{}})
	if _, err := svc.Save(context.Background(), "bot@esb.co.id", "tok", "2027-10-26", "a"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := svc.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if repo.rec != nil || repo.expiry != nil {
		t.Errorf("stored account %+v / date %v must be gone", repo.rec, repo.expiry)
	}
	if jira_account_holder.Configured() {
		t.Error("no account may stay in force after Reset")
	}
}

func TestStatus_ShowsTheAccountAndOnlyTheLastFourOfTheToken(t *testing.T) {
	repo := &fakeRepo{}
	svc := newService(t, testKey, repo, &fakeChecker{})

	if _, err := svc.Save(context.Background(), "bot@esb.co.id", "abcdefgh-WXYZ", "", "admin@esb.co.id"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	st, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Source != "database" || st.Email != "bot@esb.co.id" || st.TokenHint != "••••••••WXYZ" || st.UpdatedBy != "admin@esb.co.id" || st.UpdatedDate == nil || !st.EncryptionReady {
		t.Errorf("status = %+v", st)
	}
	if strings.Contains(st.TokenHint, "abcdefgh") {
		t.Error("the hint must not reveal more than the last characters")
	}
}

func TestTest_AsksJiraAboutTheAccountInForceAndRefusesWithoutOne(t *testing.T) {
	checker := &fakeChecker{who: Identity{DisplayName: "Yusuf Wijaya"}}
	svc := newService(t, testKey, &fakeRepo{}, checker)

	if _, err := svc.Test(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("without an account: err = %v, want ErrNotConfigured", err)
	}
	if checker.gotMail != "" {
		t.Error("Jira must not be asked when there is no account")
	}

	jira_account_holder.SetOverride(&jira_account_holder.Credential{Email: "yusuf.wijaya@esb.co.id", APIToken: "tok"})
	who, err := svc.Test(context.Background())
	if err != nil || who.DisplayName != "Yusuf Wijaya" {
		t.Fatalf("Test = %+v, %v", who, err)
	}
	if checker.gotMail != "yusuf.wijaya@esb.co.id" || checker.gotTok != "tok" {
		t.Errorf("checked %q / %q", checker.gotMail, checker.gotTok)
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

func TestSetExpiry_ChangesOnlyTheDateOfTheAccountInForce(t *testing.T) {
	repo := &fakeRepo{}
	svc := newService(t, testKey, repo, &fakeChecker{})

	if err := svc.SetExpiry(context.Background(), "2026-11-23", "admin@esb.co.id"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("without an account: err = %v, want ErrNotConfigured", err)
	}
	if repo.expiry != nil {
		t.Error("a date must not be stored for an account that does not exist")
	}

	jira_account_holder.SetOverride(&jira_account_holder.Credential{Email: "bot@esb.co.id", APIToken: "tok"})
	if err := svc.SetExpiry(context.Background(), "2026-11-23", "admin@esb.co.id"); err != nil {
		t.Fatalf("SetExpiry (a past date is fine): %v", err)
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
