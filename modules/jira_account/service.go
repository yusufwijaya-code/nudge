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

// ErrNotConfigured is returned when something needs the account in force and
// no account has been set up.
var ErrNotConfigured = errors.New("jira_account: no Jira account is set up")

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
	// Source is "database" (an account is set up) or "none" (nothing is set up:
	// the bot cannot read Jira until an admin fills it in).
	Source    string `json:"source"`
	Email     string `json:"email"`
	TokenHint string `json:"tokenHint"`
	// Unreadable: an account is stored but cannot be decrypted (the encryption
	// key changed), so it is not in force. Saving a new one replaces it.
	Unreadable      bool       `json:"unreadable"`
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
	// SetExpiry records only the expiry day of the account in force ("" clears it).
	SetExpiry(ctx context.Context, expiresAt, editor string) error
	// ExpiringUsers lists the users whose personal token expires within ExpiryWarnDays.
	ExpiringUsers(ctx context.Context) ([]ExpiringUser, error)
	// Reset removes the account: the bot stops reading Jira until a new one is saved.
	Reset(ctx context.Context) error
}

type Service struct {
	repo     Repository
	checker  Checker
	key      [32]byte
	keyReady bool
	// legacyEmail / legacyToken are the JIRA_EMAIL / JIRA_API_TOKEN a server
	// used before the account moved to the admin panel. They are read once, to
	// carry that account over (see Load); nothing else uses them.
	legacyEmail string
	legacyToken string
	expiring    ExpiringSource
	unreadable  bool
}

func NewService(cfg *config.AppConfig, repo Repository, checker Checker) *Service {
	key, ok := parseKey(cfg.CredentialEncryptionKey)
	return &Service{repo: repo, checker: checker, key: key, keyReady: ok, legacyEmail: cfg.LegacyJiraEmail, legacyToken: cfg.LegacyJiraAPIToken}
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

// Load puts the saved account in force at start.
//
// A server that still has JIRA_EMAIL / JIRA_API_TOKEN in its environment and no
// saved account has that pair carried over once, encrypted into the database,
// so the bot keeps working through the move to the admin panel; the two
// variables can then be deleted. A stored token that cannot be decrypted (the
// key changed) is logged and left out of force rather than failing to boot.
func (s *Service) Load(ctx context.Context) error {
	s.unreadable = false
	rec, err := s.repo.Find(ctx)
	if err != nil {
		return err
	}
	if rec == nil {
		return s.carryOverLegacy(ctx)
	}
	if !s.keyReady {
		log.Printf("jira_account: a saved account exists but CREDENTIAL_ENCRYPTION_KEY is not usable; no Jira account is in force")
		s.unreadable = true
		jira_account_holder.SetOverride(nil)
		return nil
	}
	plain, err := jira_credential_crypto.Decrypt(rec.TokenEncrypted, s.key)
	if err != nil {
		log.Printf("jira_account: the saved account could not be decrypted (%v); no Jira account is in force", err)
		s.unreadable = true
		jira_account_holder.SetOverride(nil)
		return nil
	}
	jira_account_holder.SetOverride(&jira_account_holder.Credential{Email: rec.Email, APIToken: string(plain)})
	return nil
}

// carryOverLegacy moves the old environment account into the database, once.
func (s *Service) carryOverLegacy(ctx context.Context) error {
	email, token := strings.TrimSpace(s.legacyEmail), strings.TrimSpace(s.legacyToken)
	if email == "" || token == "" {
		jira_account_holder.SetOverride(nil)
		return nil
	}
	cred := &jira_account_holder.Credential{Email: email, APIToken: token}
	if !s.keyReady {
		log.Printf("jira_account: using JIRA_EMAIL / JIRA_API_TOKEN from the environment for now; CREDENTIAL_ENCRYPTION_KEY is not usable, so it cannot be moved into the admin panel")
		jira_account_holder.SetOverride(cred)
		return nil
	}
	sealed, err := jira_credential_crypto.Encrypt([]byte(token), s.key)
	if err != nil {
		return fmt.Errorf("jira_account: encrypt the carried-over token: %w", err)
	}
	if err := s.repo.Save(ctx, Record{Email: email, TokenEncrypted: sealed, UpdatedBy: "env-import"}); err != nil {
		return err
	}
	jira_account_holder.SetOverride(cred)
	log.Printf("jira_account: carried the Jira account %s over from the environment into the admin panel; JIRA_EMAIL and JIRA_API_TOKEN can be deleted from .env", email)
	return nil
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	st := Status{Source: "none", EncryptionReady: s.keyReady, Unreadable: s.unreadable}
	email, token, ok := jira_account_holder.Current()
	if !ok {
		return st, nil
	}
	st.Source, st.Email, st.TokenHint = "database", email, hint(token)
	rec, err := s.repo.Find(ctx)
	if err != nil {
		return Status{}, err
	}
	if rec != nil {
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

// Test asks Jira who the account in force is.
func (s *Service) Test(ctx context.Context) (Identity, error) {
	email, token, ok := jira_account_holder.Current()
	if !ok {
		return Identity{}, ErrNotConfigured
	}
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
	s.unreadable = false
	jira_account_holder.SetOverride(&jira_account_holder.Credential{Email: email, APIToken: apiToken})
	return who, nil
}

// SetExpiry changes only the date of the account in force.
func (s *Service) SetExpiry(ctx context.Context, expiresAt, editor string) error {
	if !jira_account_holder.Configured() {
		return ErrNotConfigured
	}
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

// Reset removes the account: no Jira account is in force until a new one is saved.
func (s *Service) Reset(ctx context.Context) error {
	if err := s.repo.Delete(ctx); err != nil {
		return err
	}
	// The date belonged to the token that was just removed.
	if err := s.repo.SaveExpiry(ctx, nil, ""); err != nil {
		return err
	}
	s.unreadable = false
	jira_account_holder.SetOverride(nil)
	return nil
}
