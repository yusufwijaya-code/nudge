// Package app_credential_holder keeps the application credentials an admin set in
// Pengaturan Aplikasi (the Gemini API key, the Google Chat webhook URL) where the
// code that uses them reads them on every call, so a change applies at once,
// without a restart. A value that is not set here is "", and the caller falls back
// to the environment. It has no dependencies.
package app_credential_holder

import "sync/atomic"

var (
	gemini  atomic.Pointer[string]
	webhook atomic.Pointer[string]
)

func load(p *atomic.Pointer[string]) string {
	if v := p.Load(); v != nil {
		return *v
	}
	return ""
}

func store(p *atomic.Pointer[string], v string) {
	if v == "" {
		p.Store(nil)
		return
	}
	p.Store(&v)
}

// GeminiAPIKey is the key set by an admin; "" when none (use GEMINI_API_KEY).
func GeminiAPIKey() string { return load(&gemini) }

func SetGeminiAPIKey(v string) { store(&gemini, v) }

// ChatWebhookURL is the webhook set by an admin; "" when none (use GOOGLE_CHAT_WEBHOOK_URL).
func ChatWebhookURL() string { return load(&webhook) }

func SetChatWebhookURL(v string) { store(&webhook, v) }
