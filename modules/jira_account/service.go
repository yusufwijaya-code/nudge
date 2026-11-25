package jira_account

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"nudge/config"
	"nudge/modules/jira_account/jira_account_holder"
	"nudge/modules/jira_credential/jira_credential_crypto"
	"nudge/modules/jira_credential/jira_credential_dto"
)

// ErrEncryptionKeyNotConfigured: CREDENTIAL_ENCRYPTION_KEY is missing or not
// a 32-byte base64 key, so a token cannot be stored.
var ErrEncryptionKeyNotConfigured = errors.New("jira_account: CREDENTIAL_ENCRYPTION_KEY is not configured")

// ErrInvalidInput is returned for an empty or malformed email / token.
var ErrInvalidInput = errors.New("jira_account: email and API token are required")

// tokenHintLen is how many trailing characters of the token the admin sees,
// enough to tell which token is in force, not enough to reconstruct it.
const tokenHintLen = 4

// ExpiryWarnDays is how many days before its expiry a token starts to warn.
// The web app uses the same number (jira-token-expiry.ts).
const ExpiryWarnDays = 14

// ExpiringSource lists the personal tokens that are close to their expiry.
type ExpiringSource interface {
	ListExpiring(ctx context.Context, within time.Duration) ([]jira_credential_dto.ExpiringRecord, error)
}

// ExpiringUser is a user whose personal Jira token expires soon (or has).
type ExpiringUser struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	ExpiresAt string `json:"expiresAt"`
}

// Status is what the admin page shows. The token itself never leaves here.
type Status struct {
	// Source is "database" (saved by an admin) or "env" (JIRA_EMAIL / JIRA_API_TOKEN).
	Source          string     `json:"source"`
	Email           string     `json:"email"`
	TokenHint       string     `json:"tokenHint"`
	UpdatedBy       string     `json:"updatedBy,omitempty"`
	UpdatedDate     *time.Time `json:"updatedDate,omitempty"`
	EncryptionReady bool       `json:"encryptionReady"`
	// ExpiresAt is the day the token in force expires ("YYYY-MM-DD"), entered
	// by an admin because Jira does not report it; "" = unknown.
	ExpiresAt string `json:"expiresAt"`
}

// API is what the handler needs from Service.
type API interface {
	Status(ctx context.Context) (Status, error)
	Test(ctx context.Context) (Identity, error)
	// Save verifies and stores a new account; expiresAt is "YYYY-MM-DD" or "".
	Save(ctx context.Context, email, apiToken, expiresAt, editor string) (Identity, error)
	// SetExpiry records only the expiry day of the token in force ("" clears it).
	SetExpiry(ctx context.Context, expiresAt, editor string) error
	// ExpiringUsers lists the users whose personal token expires within ExpiryWarnDays.
	ExpiringUsers(ctx context.Context) ([]ExpiringUser, error)
	Reset(ctx context.Context) error
}

type Service struct {
	repo     Repository
	checker  Checker
	key      [32]byte
	keyReady bool
	envEmail string
	envToken string
	expiring ExpiringSource
}

func NewService(cfg *config.AppConfig, repo Repository, checker Checker) *Service {
	key, ok := parseKey(cfg.CredentialEncryptionKey)
	return &Service{repo: repo, checker: checker, key: key, keyReady: ok, envEmail: cfg.JiraEmail, envToken: cfg.JiraAPIToken}
}

// WithExpiringUsers adds the personal-token source behind the admin banner.
func (s *Service) WithExpiringUsers(src ExpiringSource) *Service {
	s.expiring = src
	return s
}

func parseKey(encoded string) ([32]byte, bool) {
	var key [32]byte
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if encoded == "" || err != nil || len(decoded) != 32 {
		return key, false
	}
	copy(key[:], decoded)
	return key, true
}

// Load installs the saved account at start. A stored token that cannot be
// decrypted (key changed) is logged and ignored: the service keeps running on
// the environment pair rather than failing to boot.
func (s *Service) Load(ctx context.Context) error {
	rec, err := s.repo.Find(ctx)
	if err != nil {
		return err
	}
	if rec == nil {
		jira_account_holder.SetOverride(nil)
		return nil
	}
	if !s.keyReady {
		log.Printf("jira_account: a saved account exists but CREDENTIAL_ENCRYPTION_KEY is not usable; using the environment credential")
		jira_account_holder.SetOverride(nil)
		return nil
	}
	plain, err := jira_credential_crypto.Decrypt(rec.TokenEncrypted, s.key)
	if err != nil {
		log.Printf("jira_account: the saved account could not be decrypted (%v); using the environment credential", err)
		jira_account_holder.SetOverride(nil)
		return nil
	}
	jira_account_holder.SetOverride(&jira_account_holder.Credential{Email: rec.Email, APIToken: string(plain)})
	return nil
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	rec, err := s.repo.Find(ctx)
	if err != nil {
		return Status{}, err
	}
	email, token := jira_account_holder.Current(s.envEmail, s.envToken)
	st := Status{Source: "env", Email: email, TokenHint: hint(token), EncryptionReady: s.keyReady}
	if rec != nil && jira_account_holder.Override() != nil {
		st.Source = "database"
		st.UpdatedBy = rec.UpdatedBy
		updated := rec.UpdatedDate
		st.UpdatedDate = &updated
	}
	expiry, err := s.repo.FindExpiry(ctx)
	if err != nil {
		return Status{}, err
	}
	st.ExpiresAt = jira_credential_dto.FormatExpiryDate(expiry)
	return st, nil
}

func hint(token string) string {
	if len(token) <= tokenHintLen {
		return "••••"
	}
	return "••••••••" + token[len(token)-tokenHintLen:]
}

// Test asks Jira who the credential in force is.
func (s *Service) Test(ctx context.Context) (Identity, error) {
	email, token := jira_account_holder.Current(s.envEmail, s.envToken)
	return s.checker.WhoAmI(ctx, email, token)
}

// Save verifies email + apiToken against Jira, stores the token encrypted and
// switches every Jira client to it at once. A credential Jira rejects is never
// stored, so a typo cannot cut the bot off.
func (s *Service) Save(ctx context.Context, email, apiToken, expiresAt, editor string) (Identity, error) {
	email, apiToken = strings.TrimSpace(email), strings.TrimSpace(apiToken)
	if email == "" || apiToken == "" || !strings.Contains(email, "@") {
		return Identity{}, ErrInvalidInput
	}
	// Before anything is verified or stored, so a bad date cannot half-save.
	day, err := jira_credential_dto.ParseExpiryDate(expiresAt, time.Now())
	if err != nil {
		return Identity{}, err
	}
	if !s.keyReady {
		return Identity{}, ErrEncryptionKeyNotConfigured
	}
	who, err := s.checker.WhoAmI(ctx, email, apiToken)
	if err != nil {
		return Identity{}, err
	}
	sealed, err := jira_credential_crypto.Encrypt([]byte(apiToken), s.key)
	if err != nil {
		return Identity{}, fmt.Errorf("jira_account: encrypt token: %w", err)
	}
	if err := s.repo.Save(ctx, Record{Email: email, TokenEncrypted: sealed, UpdatedBy: editor}); err != nil {
		return Identity{}, err
	}
	// A new token never inherits the old token's date: set it, or forget it.
	if err := s.repo.SaveExpiry(ctx, day, editor); err != nil {
		log.Printf("jira_account: saving the expiry failed: %v", err)
	}
	jira_account_holder.SetOverride(&jira_account_holder.Credential{Email: email, APIToken: apiToken})
	return who, nil
}

func (s *Service) SetExpiry(ctx context.Context, expiresAt, editor string) error {
	day, err := jira_credential_dto.ParseExpiryDate(expiresAt, time.Now())
	if err != nil {
		return err
	}
	return s.repo.SaveExpiry(ctx, day, editor)
}

func (s *Service) ExpiringUsers(ctx context.Context) ([]ExpiringUser, error) {
	if s.expiring == nil {
		return nil, nil
	}
	records, err := s.expiring.ListExpiring(ctx, ExpiryWarnDays*24*time.Hour)
	if err != nil {
		return nil, err
	}
	out := make([]ExpiringUser, 0, len(records))
	for _, rec := range records {
		day := rec.ExpiresAt
		out = append(out, ExpiringUser{Name: rec.UserName, Email: rec.Email, ExpiresAt: jira_credential_dto.FormatExpiryDate(&day)})
	}
	return out, nil
}

// Reset removes the saved account: the environment pair is used again.
func (s *Service) Reset(ctx context.Context) error {
	if err := s.repo.Delete(ctx); err != nil {
		return err
	}
	// The date belonged to the token that was just removed.
	if err := s.repo.SaveExpiry(ctx, nil, ""); err != nil {
		return err
	}
	jira_account_holder.SetOverride(nil)
	return nil
}
