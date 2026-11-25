package jira_credential_service

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"nudge/config"
	"nudge/modules/jira_credential/jira_credential_dto"
	"nudge/modules/registration/registration_dto"
)

const testKeyBase64 = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=" // base64("0123456789abcdef0123456789abcdef")

type fakeJiraCredentialRepo struct {
	byUserID  map[int64]jira_credential_dto.Record
	upsertErr error
	deleteErr error
}

func (f *fakeJiraCredentialRepo) Upsert(ctx context.Context, userID int64, jiraEmail string, encrypted []byte) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	if f.byUserID == nil {
		f.byUserID = map[int64]jira_credential_dto.Record{}
	}
	f.byUserID[userID] = jira_credential_dto.Record{UserID: userID, JiraEmail: jiraEmail, JiraAPITokenEncrypted: encrypted, FlagActive: true}
	return nil
}

func (f *fakeJiraCredentialRepo) FindByUserID(ctx context.Context, userID int64) (*jira_credential_dto.Record, bool, error) {
	rec, ok := f.byUserID[userID]
	if !ok {
		return nil, false, nil
	}
	return &rec, true, nil
}

func (f *fakeJiraCredentialRepo) DeleteByUserID(ctx context.Context, userID int64) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.byUserID, userID)
	return nil
}

func (f *fakeJiraCredentialRepo) FindAll(ctx context.Context) ([]jira_credential_dto.Record, error) {
	var out []jira_credential_dto.Record
	for _, rec := range f.byUserID {
		out = append(out, rec)
	}
	return out, nil
}

func (f *fakeJiraCredentialRepo) SetExpiry(ctx context.Context, userID int64, expiresAt *time.Time) error {
	rec, ok := f.byUserID[userID]
	if !ok {
		return nil
	}
	rec.ExpiresAt = expiresAt
	f.byUserID[userID] = rec
	return nil
}

func (f *fakeJiraCredentialRepo) ListExpiringBefore(ctx context.Context, cutoff time.Time) ([]jira_credential_dto.ExpiringRecord, error) {
	var out []jira_credential_dto.ExpiringRecord
	for _, rec := range f.byUserID {
		if rec.ExpiresAt != nil && rec.ExpiresAt.Before(cutoff) {
			out = append(out, jira_credential_dto.ExpiringRecord{UserID: rec.UserID, Email: rec.JiraEmail, ExpiresAt: *rec.ExpiresAt})
		}
	}
	return out, nil
}

type fakeRegistrationRepoForCredential struct {
	usersByEmail map[string]registration_dto.User
}

func (f *fakeRegistrationRepoForCredential) UpsertUser(ctx context.Context, name, email, jiraAccountID string) error {
	return nil
}
func (f *fakeRegistrationRepoForCredential) FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error) {
	u, ok := f.usersByEmail[email]
	if !ok {
		return nil, false, nil
	}
	return &u, true, nil
}
func (f *fakeRegistrationRepoForCredential) FindUserByJiraAccountID(ctx context.Context, jiraAccountID string) (*registration_dto.User, bool, error) {
	return nil, false, nil
}
func (f *fakeRegistrationRepoForCredential) UpdateGoogleChatDMSpaceID(ctx context.Context, email, spaceID string) error {
	return nil
}

func (f *fakeRegistrationRepoForCredential) FindAll(ctx context.Context) ([]registration_dto.User, error) {
	return nil, nil
}

func (f *fakeRegistrationRepoForCredential) UpdateFlagActive(ctx context.Context, userID int64, active bool) error {
	return nil
}

func (f *fakeRegistrationRepoForCredential) UpdateJiraAccountID(ctx context.Context, userID int64, jiraAccountID string) error {
	return nil
}

func (f *fakeRegistrationRepoForCredential) DeleteByID(ctx context.Context, userID int64) error {
	return nil
}
func (f *fakeRegistrationRepoForCredential) FindAllWithGoogleChatDMSpaceID(ctx context.Context) ([]registration_dto.User, error) {
	return nil, nil
}

func newTestConfig() *config.AppConfig {
	return &config.AppConfig{CredentialEncryptionKey: testKeyBase64}
}

func TestUpsertAndResolveCredential_RoundTrips(t *testing.T) {
	credRepo := &fakeJiraCredentialRepo{}
	regRepo := &fakeRegistrationRepoForCredential{usersByEmail: map[string]registration_dto.User{
		"dev@esb.co.id": {UserID: 7, Email: "dev@esb.co.id"},
	}}
	svc := NewJiraCredentialService(newTestConfig(), credRepo, regRepo)

	if err := svc.UpsertCredential(context.Background(), 7, "dev@esb.co.id", "my-api-token"); err != nil {
		t.Fatalf("UpsertCredential returned error: %v", err)
	}

	cred, err := svc.ResolveCredential(context.Background(), "dev@esb.co.id")
	if err != nil {
		t.Fatalf("ResolveCredential returned error: %v", err)
	}
	if cred == nil {
		t.Fatal("ResolveCredential returned nil, want the credential just set")
	}
	if cred.APIToken != "my-api-token" || cred.Email != "dev@esb.co.id" {
		t.Errorf("cred = %+v, want {dev@esb.co.id my-api-token}", cred)
	}
}

func TestResolveCredential_ReturnsNilWhenNeverSetUp(t *testing.T) {
	credRepo := &fakeJiraCredentialRepo{}
	regRepo := &fakeRegistrationRepoForCredential{usersByEmail: map[string]registration_dto.User{
		"dev@esb.co.id": {UserID: 7, Email: "dev@esb.co.id"},
	}}
	svc := NewJiraCredentialService(newTestConfig(), credRepo, regRepo)

	cred, err := svc.ResolveCredential(context.Background(), "dev@esb.co.id")
	if err != nil {
		t.Fatalf("ResolveCredential returned error: %v, want nil — \"never set up\" is not an error (spec AC5.4)", err)
	}
	if cred != nil {
		t.Errorf("cred = %+v, want nil", cred)
	}
}

func TestResolveCredential_ReturnsNilWhenUnknownEmail(t *testing.T) {
	credRepo := &fakeJiraCredentialRepo{}
	regRepo := &fakeRegistrationRepoForCredential{usersByEmail: map[string]registration_dto.User{}}
	svc := NewJiraCredentialService(newTestConfig(), credRepo, regRepo)

	cred, err := svc.ResolveCredential(context.Background(), "ghost@esb.co.id")
	if err != nil || cred != nil {
		t.Errorf("ResolveCredential = %v, %v, want nil, nil", cred, err)
	}
}

func TestDeleteCredential_RemovesIt(t *testing.T) {
	credRepo := &fakeJiraCredentialRepo{}
	regRepo := &fakeRegistrationRepoForCredential{usersByEmail: map[string]registration_dto.User{
		"dev@esb.co.id": {UserID: 7, Email: "dev@esb.co.id"},
	}}
	svc := NewJiraCredentialService(newTestConfig(), credRepo, regRepo)

	if err := svc.UpsertCredential(context.Background(), 7, "dev@esb.co.id", "token"); err != nil {
		t.Fatalf("UpsertCredential returned error: %v", err)
	}
	if err := svc.DeleteCredential(context.Background(), 7); err != nil {
		t.Fatalf("DeleteCredential returned error: %v", err)
	}

	cred, err := svc.ResolveCredential(context.Background(), "dev@esb.co.id")
	if err != nil || cred != nil {
		t.Errorf("after delete, ResolveCredential = %v, %v, want nil, nil (spec AC5.5)", cred, err)
	}
}

func TestUpsertCredential_FailsWhenKeyNotConfigured(t *testing.T) {
	credRepo := &fakeJiraCredentialRepo{}
	regRepo := &fakeRegistrationRepoForCredential{}
	svc := NewJiraCredentialService(&config.AppConfig{}, credRepo, regRepo) // no CredentialEncryptionKey

	err := svc.UpsertCredential(context.Background(), 7, "dev@esb.co.id", "token")
	if !errors.Is(err, ErrEncryptionKeyNotConfigured) {
		t.Errorf("err = %v, want ErrEncryptionKeyNotConfigured", err)
	}
}

func TestMaskedPreview_ShowsOnlyLastFourCharacters(t *testing.T) {
	credRepo := &fakeJiraCredentialRepo{}
	regRepo := &fakeRegistrationRepoForCredential{usersByEmail: map[string]registration_dto.User{
		"dev@esb.co.id": {UserID: 7, Email: "dev@esb.co.id"},
	}}
	svc := NewJiraCredentialService(newTestConfig(), credRepo, regRepo)

	if err := svc.UpsertCredential(context.Background(), 7, "dev@esb.co.id", "ATATT3xFfGF0superSecretTokenAb3f"); err != nil {
		t.Fatalf("UpsertCredential returned error: %v", err)
	}

	preview, found, err := svc.MaskedPreview(context.Background(), 7)
	if err != nil || !found {
		t.Fatalf("MaskedPreview = %q, %v, %v", preview, found, err)
	}
	if preview != "••••••••Ab3f" {
		t.Errorf("preview = %q, want %q (spec AC6.4 — never the full token)", preview, "••••••••Ab3f")
	}
}

func TestMaskedPreview_NotFoundWhenNeverSetUp(t *testing.T) {
	credRepo := &fakeJiraCredentialRepo{}
	svc := NewJiraCredentialService(newTestConfig(), credRepo, &fakeRegistrationRepoForCredential{})

	_, found, err := svc.MaskedPreview(context.Background(), 99)
	if err != nil || found {
		t.Errorf("MaskedPreview = found=%v err=%v, want found=false err=nil", found, err)
	}
}

func TestParseKey_RejectsInvalidBase64(t *testing.T) {
	if _, ok := parseKey("not-valid-base64!!!"); ok {
		t.Error("parseKey accepted invalid base64, want ok=false")
	}
}

func TestParseKey_RejectsWrongLength(t *testing.T) {
	shortKey := base64.StdEncoding.EncodeToString([]byte("too-short"))
	if _, ok := parseKey(shortKey); ok {
		t.Error("parseKey accepted a non-32-byte key, want ok=false")
	}
}

func TestExpiry_IsSetReadAndClearedPerUserAndListedWhenClose(t *testing.T) {
	repo := &fakeJiraCredentialRepo{byUserID: map[int64]jira_credential_dto.Record{
		1: {UserID: 1, JiraEmail: "a@esb.co.id", FlagActive: true},
		2: {UserID: 2, JiraEmail: "b@esb.co.id", FlagActive: true},
	}}
	svc := NewJiraCredentialService(&config.AppConfig{}, repo, &fakeRegistrationRepoForCredential{})
	ctx := context.Background()

	if day, err := svc.Expiry(ctx, 1); err != nil || day != nil {
		t.Fatalf("before: Expiry = %v, %v; want unknown", day, err)
	}
	soon := time.Now().Add(5 * 24 * time.Hour)
	later := time.Now().Add(200 * 24 * time.Hour)
	if err := svc.SetExpiry(ctx, 1, &soon); err != nil {
		t.Fatalf("SetExpiry: %v", err)
	}
	if err := svc.SetExpiry(ctx, 2, &later); err != nil {
		t.Fatalf("SetExpiry: %v", err)
	}
	if day, _ := svc.Expiry(ctx, 1); day == nil || !day.Equal(soon) {
		t.Errorf("Expiry(1) = %v, want %v", day, soon)
	}

	close, err := svc.ListExpiring(ctx, 14*24*time.Hour)
	if err != nil || len(close) != 1 || close[0].UserID != 1 {
		t.Errorf("ListExpiring(14 days) = %+v, %v; want only user 1", close, err)
	}

	if err := svc.SetExpiry(ctx, 1, nil); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if day, _ := svc.Expiry(ctx, 1); day != nil {
		t.Errorf("Expiry(1) after clearing = %v", day)
	}
	if day, err := svc.Expiry(ctx, 99); err != nil || day != nil {
		t.Errorf("Expiry of a user without a credential = %v, %v; want nil", day, err)
	}
}
