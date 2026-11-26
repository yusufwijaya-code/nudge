// Package signup is the registration step that sits between "Masuk dengan Google"
// and having an account: a visitor with no account is sent here, and the account
// is created only after the Google Group that makes the Nudge Chat App visible
// has accepted them (Tech Spec "Pisah Login dan Registrasi").
package signup

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

// Pending is a Google identity that signed in but has no account yet.
type Pending struct {
	Email   string `json:"e"`
	Name    string `json:"n"`
	Picture string `json:"p,omitempty"`
	Expires int64  `json:"x"`
}

// Tokens signs the Pending identity into the short-lived cookie that carries it
// between the Google callback and the registration page, so no server state is
// kept for visitors who never finish.
type Tokens struct {
	key []byte
	now func() time.Time
}

// NewTokens derives the signing key from secret (CREDENTIAL_ENCRYPTION_KEY). An
// empty secret gets a random per-process key: pending registrations then do not
// survive a restart, which only costs the visitor one more sign-in.
func NewTokens(secret string) *Tokens {
	key := sha256.Sum256([]byte("nudge-signup-v1|" + secret))
	k := key[:]
	if secret == "" {
		k = make([]byte, 32)
		_, _ = rand.Read(k)
	}
	return &Tokens{key: k, now: time.Now}
}

func (t *Tokens) mac(payload string) []byte {
	m := hmac.New(sha256.New, t.key)
	m.Write([]byte(payload))
	return m.Sum(nil)
}

// Sign returns the cookie value for p, valid for ttl.
func (t *Tokens) Sign(p Pending, ttl time.Duration) string {
	p.Expires = t.now().Add(ttl).Unix()
	raw, _ := json.Marshal(p)
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + "." + base64.RawURLEncoding.EncodeToString(t.mac(payload))
}

// Verify returns the identity inside value, false when it is forged or expired.
func (t *Tokens) Verify(value string) (Pending, bool) {
	payload, sig, ok := strings.Cut(value, ".")
	if !ok {
		return Pending{}, false
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, t.mac(payload)) {
		return Pending{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return Pending{}, false
	}
	var p Pending
	if json.Unmarshal(raw, &p) != nil || p.Email == "" || t.now().Unix() > p.Expires {
		return Pending{}, false
	}
	return p, true
}
