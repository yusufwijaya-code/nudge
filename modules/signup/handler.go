package signup

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_handler"
	"nudge/modules/registration/registration_dto"
	"nudge/modules/registration/registration_service"
)

const (
	cookieName = "signup_token"
	// cookieTTL: long enough for a visitor to read the page and press the button.
	cookieTTL = 15 * time.Minute
)

// Signups is what the handler needs from Service.
type Signups interface {
	IsNew(ctx context.Context, email string) (bool, error)
	Register(ctx context.Context, name, email string) (*registration_dto.User, error)
}

// Sessions issues the login session once the account exists.
type Sessions interface {
	IssueSession(ctx context.Context, userID int64) (string, error)
}

// Hook runs after a successful sign-up, like the login hook (profile picture…).
type Hook func(ctx context.Context, userID int64, claims auth_dto.GoogleClaims)

type Handler struct {
	svc        Signups
	tokens     *Tokens
	sessions   Sessions
	basePath   string
	adminEmail string
	onSignup   Hook
}

func NewHandler(svc Signups, tokens *Tokens, sessions Sessions, basePath, adminEmail string, onSignup Hook) *Handler {
	return &Handler{svc: svc, tokens: tokens, sessions: sessions, basePath: basePath, adminEmail: adminEmail, onSignup: onSignup}
}

// IsNew and Begin make Handler the auth handler's SignupGate.
func (h *Handler) IsNew(ctx context.Context, email string) (bool, error) {
	return h.svc.IsNew(ctx, email)
}

func (h *Handler) Begin(c *gin.Context, claims auth_dto.GoogleClaims) string {
	value := h.tokens.Sign(Pending{Email: claims.Email, Name: claims.Name, Picture: claims.Picture}, cookieTTL)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(cookieName, value, int(cookieTTL.Seconds()), h.basePath, "", true, true)
	return "/register"
}

func (h *Handler) clear(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(cookieName, "", -1, h.basePath, "", true, true)
}

func (h *Handler) pending(c *gin.Context) (Pending, bool) {
	raw, err := c.Cookie(cookieName)
	if err != nil {
		return Pending{}, false
	}
	return h.tokens.Verify(raw)
}

// Routes mounts /api/signup (no session yet: the signed cookie is the identity).
func Routes(g gin.IRouter, h *Handler) {
	s := g.Group("/api/signup")
	s.GET("/me", h.me)
	s.POST("", h.register)
}

func expired(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{"error": "signup_expired", "message": "Sesi pendaftaran sudah berakhir. Masuk lagi dengan Google untuk melanjutkan."})
}

func (h *Handler) me(c *gin.Context) {
	p, ok := h.pending(c)
	if !ok {
		expired(c)
		return
	}
	isNew, err := h.svc.IsNew(c.Request.Context(), p.Email)
	if err == nil && !isNew {
		// Registered in the meantime (another tab): nothing left to do here.
		c.JSON(http.StatusOK, gin.H{"email": p.Email, "name": p.Name, "registered": true})
		return
	}
	c.JSON(http.StatusOK, gin.H{"email": p.Email, "name": p.Name, "registered": false, "adminEmail": h.adminEmail})
}

func (h *Handler) register(c *gin.Context) {
	p, ok := h.pending(c)
	if !ok {
		expired(c)
		return
	}
	ctx := c.Request.Context()
	user, err := h.svc.Register(ctx, p.Name, p.Email)
	if err != nil {
		h.fail(c, err)
		return
	}
	token, err := h.sessions.IssueSession(ctx, user.UserID)
	if err != nil {
		log.Printf("signup: issuing the session of %s failed: %v", p.Email, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "session_failed", "message": "Akun dibuat, tapi gagal masuk otomatis. Silakan masuk lagi."})
		return
	}
	if h.onSignup != nil {
		h.onSignup(ctx, user.UserID, auth_dto.GoogleClaims{Email: p.Email, Name: p.Name, Picture: p.Picture})
	}
	auth_handler.SetSessionCookie(c, h.basePath, token)
	h.clear(c)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrGroupUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":      "group_unavailable",
			"message":    "Pendaftaran belum bisa diproses karena akses ke Google Group Nudge sedang bermasalah. Akun kamu belum dibuat. Hubungi admin lewat chat, lalu coba lagi.",
			"adminEmail": h.adminEmail,
		})
	case errors.Is(err, registration_service.ErrJiraAccountNotFound):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "jira_account_not_found", "message": "Email ini belum terdaftar sebagai akun Jira esb-id. Akun kamu belum dibuat.", "adminEmail": h.adminEmail})
	case errors.Is(err, registration_service.ErrJiraLookupUnavailable):
		c.JSON(http.StatusBadGateway, gin.H{"error": "jira_unavailable", "message": "Gagal memverifikasi akun ke Jira. Akun kamu belum dibuat, coba lagi sebentar lagi."})
	case errors.Is(err, ErrDeactivated):
		c.JSON(http.StatusForbidden, gin.H{"error": "deactivated", "message": "Akun kamu sudah dinonaktifkan. Hubungi admin lewat chat.", "adminEmail": h.adminEmail})
	default:
		log.Printf("signup: registering failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "register_failed", "message": "Gagal memproses pendaftaran. Akun kamu belum dibuat, coba lagi atau hubungi admin lewat chat.", "adminEmail": h.adminEmail})
	}
}
