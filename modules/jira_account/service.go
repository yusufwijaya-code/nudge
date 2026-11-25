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
)

// ErrEncryptionKeyNotConfigured: CREDENTIAL_ENCRYPTION_KEY is missing or not
// a 32-byte base64 key, so a token cannot be stored.
var ErrEncryptionKeyNotConfigured = errors.New("jira_account: CREDENTIAL_ENCRYPTION_KEY is not configured")

// ErrInvalidInput is returned for an empty or malformed email / token.
var ErrInvalidInput = errors.New("jira_account: email and API token are required")

// tokenHintLen is how many trailing characters of the token the admin sees,
// enough to tell which token is in force, not enough to reconstruct it.
const tokenHintLen = 4

// Status is what the admin page shows. The token itself never leaves here.
type Status struct {
	// Source is "database" (saved by an admin) or "env" (JIRA_EMAIL / JIRA_API_TOKEN).
	Source          string     `json:"source"`
	Email           string     `json:"email"`
	TokenHint       string     `json:"tokenHint"`
	UpdatedBy       string     `json:"updatedBy,omitempty"`
	UpdatedDate     *time.Time `json:"updatedDate,omitempty"`
	EncryptionReady bool       `json:"encryptionReady"`
}

// API is what the handler needs from Service.
type API interface {
	Status(ctx context.Context) (Status, error)
	Test(ctx context.Context) (Identity, error)
	Save(ctx context.Context, email, apiToken, editor string) (Identity, error)
	Reset(ctx context.Context) error
}

type Service struct {
	repo     Repository
	checker  Checker
	key      [32]byte
	keyReady bool
	envEmail string
	envToken string
}

func NewService(cfg *config.AppConfig, repo Repository, checker Checker) *Service {
	key, ok := parseKey(cfg.CredentialEncryptionKey)
	return &Service{repo: repo, checker: checker, key: key, keyReady: ok, envEmail: cfg.JiraEmail, envToken: cfg.JiraAPIToken}
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
func (s *Service) Save(ctx context.Context, email, apiToken, editor string) (Identity, error) {
	email, apiToken = strings.TrimSpace(email), strings.TrimSpace(apiToken)
	if email == "" || apiToken == "" || !strings.Contains(email, "@") {
		return Identity{}, ErrInvalidInput
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
	jira_account_holder.SetOverride(&jira_account_holder.Credential{Email: email, APIToken: apiToken})
	return who, nil
}

// Reset removes the saved account: the environment pair is used again.
func (s *Service) Reset(ctx context.Context) error {
	if err := s.repo.Delete(ctx); err != nil {
		return err
	}
	jira_account_holder.SetOverride(nil)
	return nil
}
