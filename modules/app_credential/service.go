// Package app_credential is Admin › Pengaturan Aplikasi › Kredensial & token: the
// application credentials that used to be fixed in the server environment. The text
// ones (Gemini API key, Google Chat webhook) can be changed from the web and apply at
// once; the others (OAuth tokens, the service account key, client secrets) are
// monitored: their status is shown and the day they expire is kept, so an admin gets
// a banner before one stops working.
package app_credential

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"nudge/config"
	"nudge/modules/app_credential/app_credential_holder"
	"nudge/modules/jira_credential/jira_credential_crypto"
	"nudge/modules/jira_credential/jira_credential_dto"
)

// WarnDays is how many days before its expiry a credential starts to warn (the
// same number as the Jira token banner).
const WarnDays = 14

const (
	KeyGemini        = "gemini_api_key"
	KeyChatWebhook   = "google_chat_webhook_url"
	KeyGroupsToken   = "groups_oauth_token"
	KeyChatToken     = "chat_oauth_token"
	KeyChatAppSA     = "chat_app_service_account"
	KeyOAuthSecret   = "google_oauth_client_secret"
	KeySignInSecret  = "google_signin_client_secret"
	KeyGroupsSecret  = "google_groups_client_secret"
	kindText         = "text"    // value kept here, changeable from the web
	kindMonitor      = "monitor" // only status and expiry day
	webhookPrefix    = "https://chat.googleapis.com/"
	minSecretLen     = 8
	maxSecretLen     = 1500
	geminiModelsURL  = "https://generativelanguage.googleapis.com/v1beta/models?pageSize=1"
	checkTimeout     = 12 * time.Second
	sourceDatabase   = "database"
	sourceEnv        = "env"
	sourceFile       = "file"
	sourceNone       = "none"
	importedByMarker = "env"
)

var (
	ErrEncryptionKeyNotConfigured = errors.New("app_credential: CREDENTIAL_ENCRYPTION_KEY is not configured")
	ErrUnknownKey                 = errors.New("app_credential: unknown credential")
	ErrNotEditable                = errors.New("app_credential: this credential cannot be changed from the web")
)

// ValidationError: the input is not acceptable (400).
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

type def struct {
	key, label, help, kind string
}

var catalog = []def{
	{KeyGemini, "Gemini API Key", "Dipakai Buat Card untuk merapikan Task Breakdown.", kindText},
	{KeyChatWebhook, "Webhook Google Chat", "Pengirim lama ke satu space Google Chat.", kindText},
	{KeyGroupsToken, "Token OAuth Google Groups", "Memasukkan user baru ke Google Group Nudge. Diperbarui lewat persetujuan ulang (--setup-groups-oauth).", kindMonitor},
	{KeyChatToken, "Token OAuth Chat operator", "Dipakai pengirim DM dari akun operator. Diperbarui lewat persetujuan ulang (--setup-oauth).", kindMonitor},
	{KeyChatAppSA, "Kunci service account Chat App", "Berkas JSON app Nudge di Google Chat. Diganti dengan mengunggah berkas baru ke server.", kindMonitor},
	{KeyOAuthSecret, "Client secret OAuth Chat", "Rahasia klien OAuth untuk token operator.", kindMonitor},
	{KeySignInSecret, "Client secret Sign-In", "Rahasia klien untuk Masuk dengan Google.", kindMonitor},
	{KeyGroupsSecret, "Client secret Google Groups", "Rahasia klien untuk token Google Groups.", kindMonitor},
}

func find(key string) (def, bool) {
	for _, d := range catalog {
		if d.key == key {
			return d, true
		}
	}
	return def{}, false
}

// Item is one credential as the admin page shows it. A secret itself never leaves here.
type Item struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Help string `json:"help"`
	// Editable: the value can be changed from the web ("text"); false = monitored only.
	Editable bool `json:"editable"`
	// Testable: "Uji" can check it right now.
	Testable   bool `json:"testable"`
	Configured bool `json:"configured"`
	// Source: "database" (kept here), "env" (still in the server environment),
	// "file" (a file on the server) or "none".
	Source string `json:"source"`
	// Hint: "••••abcd" for a text value; the file name for a file.
	Hint string `json:"hint,omitempty"`
	// Detail: a short fact about the file ("ada refresh token", the account email).
	Detail      string     `json:"detail,omitempty"`
	ExpiresAt   string     `json:"expiresAt"`
	UpdatedBy   string     `json:"updatedBy,omitempty"`
	UpdatedDate *time.Time `json:"updatedDate,omitempty"`
}

// Expiring is a credential that is about to expire (or has): the admin banner.
type Expiring struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	ExpiresAt string `json:"expiresAt"`
}

// TestResult is what "Uji" reports; a failed check is not an HTTP error.
type TestResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// Checks are the live checks; a nil one means "cannot be tested".
type Checks struct {
	// Gemini asks Google whether the key is accepted.
	Gemini func(ctx context.Context, apiKey string) error
	// Groups proves the operator token and the group (nil when auto join is off).
	Groups func(ctx context.Context) error
}

type Service struct {
	cfg      *config.AppConfig
	repo     Repository
	key      [32]byte
	keyReady bool
	checks   Checks
	now      func() time.Time
}

func NewService(cfg *config.AppConfig, repo Repository, checks Checks) *Service {
	key, ok := parseKey(cfg.CredentialEncryptionKey)
	if checks.Gemini == nil {
		checks.Gemini = geminiCheck(http.DefaultClient, geminiModelsURL)
	}
	return &Service{cfg: cfg, repo: repo, key: key, keyReady: ok, checks: checks, now: time.Now}
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

// geminiCheck lists one model with the key: 200 = accepted. The key travels in the
// header only, never in the URL.
func geminiCheck(hc *http.Client, url string) func(context.Context, string) error {
	return func(ctx context.Context, apiKey string) error {
		ctx, cancel := context.WithTimeout(ctx, checkTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("x-goog-api-key", apiKey)
		res, err := hc.Do(req)
		if err != nil {
			return fmt.Errorf("Google tidak merespons")
		}
		defer res.Body.Close()
		switch {
		case res.StatusCode == http.StatusOK:
			return nil
		case res.StatusCode == http.StatusBadRequest || res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
			return fmt.Errorf("key ditolak Google (HTTP %d)", res.StatusCode)
		}
		return fmt.Errorf("Google menjawab HTTP %d", res.StatusCode)
	}
}

// Load puts the stored text values in force at start. A text value still only in the
// environment is carried over once, encrypted into the database, so the page shows
// it and the environment entry can be removed; until the move the environment keeps
// working. Failures are logged: nothing here may stop the app from starting.
func (s *Service) Load(ctx context.Context) error {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("app_credential: load: %w", err)
	}
	for _, d := range catalog {
		if d.kind != kindText {
			continue
		}
		row, has := rows[d.key]
		if has && len(row.ValueEncrypted) > 0 {
			if plain, ok := s.open(d.key, row.ValueEncrypted); ok {
				s.apply(d.key, plain)
			}
			continue
		}
		if has { // removed on purpose: the environment must not bring it back
			continue
		}
		if env := s.env(d.key); env != "" && s.keyReady {
			sealed, err := jira_credential_crypto.Encrypt([]byte(env), s.key)
			if err != nil {
				log.Printf("app_credential: carrying %s over from the environment failed: %v", d.key, err)
				continue
			}
			if err := s.repo.SaveValue(ctx, d.key, sealed, nil, importedByMarker); err != nil {
				log.Printf("app_credential: saving %s failed: %v", d.key, err)
				continue
			}
			s.apply(d.key, env)
			log.Printf("app_credential: %s carried over from the environment into Pengaturan Aplikasi (the environment entry can be removed)", d.key)
		}
	}
	return nil
}

func (s *Service) open(key string, sealed []byte) (string, bool) {
	if !s.keyReady {
		return "", false
	}
	plain, err := jira_credential_crypto.Decrypt(sealed, s.key)
	if err != nil {
		log.Printf("app_credential: %s cannot be decrypted (the encryption key changed?): %v", key, err)
		return "", false
	}
	return string(plain), true
}

func (s *Service) apply(key, value string) {
	switch key {
	case KeyGemini:
		app_credential_holder.SetGeminiAPIKey(value)
	case KeyChatWebhook:
		app_credential_holder.SetChatWebhookURL(value)
	}
}

func (s *Service) env(key string) string {
	switch key {
	case KeyGemini:
		return strings.TrimSpace(s.cfg.GeminiAPIKey)
	case KeyChatWebhook:
		return strings.TrimSpace(s.cfg.GoogleChatWebhookURL)
	case KeyOAuthSecret:
		return s.cfg.GoogleOAuthClientSecret
	case KeySignInSecret:
		return s.cfg.GoogleSignInClientSecret
	case KeyGroupsSecret:
		return s.cfg.GoogleGroupsClientSecret
	}
	return ""
}

func (s *Service) filePath(key string) string {
	switch key {
	case KeyGroupsToken:
		return s.cfg.GoogleGroupsTokenPath
	case KeyChatToken:
		return s.cfg.GoogleOAuthTokenPath
	case KeyChatAppSA:
		return s.cfg.GoogleChatAppCredentialsFile
	}
	return ""
}

func hint(secret string) string {
	r := []rune(secret)
	if len(r) <= 4 {
		return "••••"
	}
	return "••••" + string(r[len(r)-4:])
}

func baseName(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// fileFacts says what can be said about a credential file without showing a secret.
func fileFacts(key, path string) (ok bool, detail string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, "berkas tidak ada atau tidak terbaca"
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return false, "isi berkas bukan JSON yang valid"
	}
	if key == KeyChatAppSA {
		email, _ := m["client_email"].(string)
		if email == "" || m["private_key"] == nil {
			return false, "bukan berkas service account yang lengkap"
		}
		return true, email
	}
	if rt, _ := m["refresh_token"].(string); rt == "" {
		return false, "tidak ada refresh token: perlu persetujuan ulang"
	}
	return true, "ada refresh token"
}

func (s *Service) item(d def, rows map[string]Row) Item {
	it := Item{Key: d.key, Name: d.label, Help: d.help, Editable: d.kind == kindText, Source: sourceNone}
	row, has := rows[d.key]
	if has {
		it.UpdatedBy = row.UpdatedBy
		t := row.UpdatedDate
		it.UpdatedDate = &t
		it.ExpiresAt = jira_credential_dto.FormatExpiryDate(row.ExpiresAt)
	}
	switch d.kind {
	case kindText:
		it.Testable = d.key == KeyGemini
		if has && len(row.ValueEncrypted) > 0 {
			if plain, ok := s.open(d.key, row.ValueEncrypted); ok {
				it.Configured, it.Source, it.Hint = true, sourceDatabase, hint(plain)
			} else {
				it.Source, it.Detail = sourceDatabase, "tersimpan tapi tidak bisa dibaca (kunci enkripsi berubah): simpan ulang"
			}
		} else if env := s.env(d.key); env != "" && !has {
			it.Configured, it.Source, it.Hint = true, sourceEnv, hint(env)
		}
	default:
		if path := s.filePath(d.key); path != "" {
			it.Testable = d.key == KeyGroupsToken && s.checks.Groups != nil
			if _, err := os.Stat(path); err == nil {
				ok, detail := fileFacts(d.key, path)
				it.Configured, it.Source, it.Hint, it.Detail = ok, sourceFile, baseName(path), detail
			} else {
				it.Source, it.Detail = sourceFile, "berkas tidak ada: "+baseName(path)
			}
		} else if env := s.env(d.key); env != "" {
			it.Configured, it.Source, it.Hint = true, sourceEnv, hint(env)
		}
	}
	return it
}

// List is every credential, in the order the page shows them.
func (s *Service) List(ctx context.Context) ([]Item, error) {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(catalog))
	for _, d := range catalog {
		out = append(out, s.item(d, rows))
	}
	return out, nil
}

func (s *Service) one(ctx context.Context, key string) (Item, error) {
	d, ok := find(key)
	if !ok {
		return Item{}, ErrUnknownKey
	}
	rows, err := s.repo.List(ctx)
	if err != nil {
		return Item{}, err
	}
	return s.item(d, rows), nil
}

func (s *Service) parseDay(raw string) (*time.Time, error) {
	t, err := jira_credential_dto.ParseExpiryDate(raw, s.now())
	if err != nil {
		return nil, &ValidationError{Message: "Tanggal kedaluwarsa tidak valid"}
	}
	return t, nil
}

// SetValue stores a new text value (checked first when it can be), applies it at once
// and records the expiry day ("YYYY-MM-DD" or "").
func (s *Service) SetValue(ctx context.Context, key, value, expiresAt, editor string) (Item, error) {
	d, ok := find(key)
	if !ok {
		return Item{}, ErrUnknownKey
	}
	if d.kind != kindText {
		return Item{}, ErrNotEditable
	}
	if !s.keyReady {
		return Item{}, ErrEncryptionKeyNotConfigured
	}
	value = strings.TrimSpace(value)
	if len(value) < minSecretLen || len(value) > maxSecretLen || strings.ContainsAny(value, " \r\n\t") {
		return Item{}, &ValidationError{Message: "Nilainya tidak valid (tanpa spasi, minimal 8 karakter)"}
	}
	if key == KeyChatWebhook && !strings.HasPrefix(value, webhookPrefix) {
		return Item{}, &ValidationError{Message: "Webhook harus diawali " + webhookPrefix}
	}
	day, err := s.parseDay(expiresAt)
	if err != nil {
		return Item{}, err
	}
	if key == KeyGemini {
		if err := s.checks.Gemini(ctx, value); err != nil {
			return Item{}, &ValidationError{Message: "Gemini API Key tidak diterima: " + err.Error()}
		}
	}
	sealed, err := jira_credential_crypto.Encrypt([]byte(value), s.key)
	if err != nil {
		return Item{}, err
	}
	if err := s.repo.SaveValue(ctx, key, sealed, day, editor); err != nil {
		return Item{}, err
	}
	s.apply(key, value)
	return s.one(ctx, key)
}

// SetExpiry records only the expiry day ("" forgets it); it works for every credential.
func (s *Service) SetExpiry(ctx context.Context, key, expiresAt, editor string) (Item, error) {
	if _, ok := find(key); !ok {
		return Item{}, ErrUnknownKey
	}
	day, err := s.parseDay(expiresAt)
	if err != nil {
		return Item{}, err
	}
	if err := s.repo.SaveExpiry(ctx, key, day, editor); err != nil {
		return Item{}, err
	}
	return s.one(ctx, key)
}

// Clear removes a text value kept here. The environment entry, if any, is NOT used
// again afterwards: removing means removing.
func (s *Service) Clear(ctx context.Context, key, editor string) (Item, error) {
	d, ok := find(key)
	if !ok {
		return Item{}, ErrUnknownKey
	}
	if d.kind != kindText {
		return Item{}, ErrNotEditable
	}
	if err := s.repo.SaveExpiry(ctx, key, nil, editor); err != nil { // makes sure the row exists
		return Item{}, err
	}
	if err := s.repo.ClearValue(ctx, key, editor); err != nil {
		return Item{}, err
	}
	s.apply(key, "")
	return s.one(ctx, key)
}

// Test checks one credential right now. Not being able to is a result, not an error.
func (s *Service) Test(ctx context.Context, key string) (TestResult, error) {
	d, ok := find(key)
	if !ok {
		return TestResult{}, ErrUnknownKey
	}
	switch key {
	case KeyGemini:
		apiKey := app_credential_holder.GeminiAPIKey()
		if apiKey == "" {
			apiKey = s.env(KeyGemini)
		}
		if apiKey == "" {
			return TestResult{Message: "Belum ada Gemini API Key."}, nil
		}
		if err := s.checks.Gemini(ctx, apiKey); err != nil {
			return TestResult{Message: err.Error()}, nil
		}
		return TestResult{OK: true, Message: "Key diterima Google."}, nil
	case KeyGroupsToken:
		if s.checks.Groups == nil {
			return TestResult{Message: "Auto join Google Group belum aktif di server."}, nil
		}
		if err := s.checks.Groups(ctx); err != nil {
			return TestResult{Message: "Token atau akses grup bermasalah: " + err.Error()}, nil
		}
		return TestResult{OK: true, Message: "Token valid dan grup bisa dikelola."}, nil
	}
	if path := s.filePath(key); path != "" {
		ok, detail := fileFacts(key, path)
		return TestResult{OK: ok, Message: detail}, nil
	}
	return TestResult{Message: d.label + " tidak bisa diuji dari sini."}, nil
}

// Expiring lists the credentials whose day is within WarnDays (or already past),
// soonest first: the admin banner.
func (s *Service) Expiring(ctx context.Context) ([]Expiring, error) {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	wib := time.FixedZone("WIB", 7*3600)
	n := s.now().In(wib)
	// Whole days, WIB: the day it expires still counts, and warning starts WarnDays before it.
	limit := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, wib).AddDate(0, 0, WarnDays)
	var out []Expiring
	for _, d := range catalog {
		row, ok := rows[d.key]
		if !ok || row.ExpiresAt == nil {
			continue
		}
		exp := row.ExpiresAt.In(wib)
		if !time.Date(exp.Year(), exp.Month(), exp.Day(), 0, 0, 0, 0, wib).After(limit) {
			out = append(out, Expiring{Key: d.key, Name: d.label, ExpiresAt: jira_credential_dto.FormatExpiryDate(row.ExpiresAt)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ExpiresAt < out[j].ExpiresAt })
	return out, nil
}
