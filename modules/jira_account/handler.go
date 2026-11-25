package jira_account

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/jira_credential/jira_credential_dto"
)

type Handler struct{ api API }

func NewHandler(api API) *Handler { return &Handler{api: api} }

// AdminRoutes mounts Admin › Akun Jira on a group already behind
// AuthMiddlewareAPI + AdminMiddleware.
func AdminRoutes(g *gin.RouterGroup, h *Handler) {
	g.GET("/jira-account", h.status)
	g.POST("/jira-account/test", h.test)
	g.PUT("/jira-account", h.save)
	g.PUT("/jira-account/expiry", h.saveExpiry)
	g.GET("/jira-account/expiring-users", h.expiringUsers)
	g.DELETE("/jira-account", h.reset)
}

const maxBodyBytes = 16 << 10

func (h *Handler) status(c *gin.Context) {
	st, err := h.api.Status(c.Request.Context())
	if err != nil {
		log.Printf("jira_account: status failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memuat akun Jira"})
		return
	}
	c.JSON(http.StatusOK, st)
}

// test never fails the request for a Jira-side problem: the page shows the
// reason next to the account instead of an error toast.
func (h *Handler) test(c *gin.Context) {
	who, err := h.api.Test(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "message": describe(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "account": who})
}

type saveRequest struct {
	Email    string `json:"email"`
	APIToken string `json:"apiToken"`
	// ExpiresAt is "YYYY-MM-DD" (the date Atlassian shows) or empty = unknown.
	ExpiresAt string `json:"expiresAt"`
}

type expiryRequest struct {
	ExpiresAt string `json:"expiresAt"`
}

func (h *Handler) save(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	var req saveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email dan API token wajib diisi"})
		return
	}
	editor := c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session).Email
	who, err := h.api.Save(c.Request.Context(), req.Email, req.APIToken, req.ExpiresAt, editor)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"ok": true, "account": who})
	case errors.Is(err, ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "email dan API token wajib diisi"})
	case errors.Is(err, jira_credential_dto.ErrInvalidExpiry):
		c.JSON(http.StatusBadRequest, gin.H{"error": "tanggal kedaluwarsa tidak valid"})
	case errors.Is(err, ErrEncryptionKeyNotConfigured):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "CREDENTIAL_ENCRYPTION_KEY belum diatur di server, token belum bisa disimpan"})
	case errors.Is(err, ErrRejected):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Jira menolak email/API token ini. Akun lama tetap dipakai."})
	default:
		log.Printf("jira_account: save failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "belum bisa menghubungi Jira untuk memverifikasi, akun lama tetap dipakai"})
	}
}

func (h *Handler) saveExpiry(c *gin.Context) {
	var req expiryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "expiresAt wajib ada"})
		return
	}
	editor := c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session).Email
	switch err := h.api.SetExpiry(c.Request.Context(), req.ExpiresAt, editor); {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"ok": true})
	case errors.Is(err, jira_credential_dto.ErrInvalidExpiry):
		c.JSON(http.StatusBadRequest, gin.H{"error": "tanggal kedaluwarsa tidak valid"})
	case errors.Is(err, ErrNotConfigured):
		c.JSON(http.StatusConflict, gin.H{"error": "belum ada akun Jira, isi akunnya dulu"})
	default:
		log.Printf("jira_account: saving the expiry failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menyimpan tanggal kedaluwarsa"})
	}
}

// expiringUsers feeds the admin banner: users whose personal Jira token
// expires soon or already has.
func (h *Handler) expiringUsers(c *gin.Context) {
	users, err := h.api.ExpiringUsers(c.Request.Context())
	if err != nil {
		log.Printf("jira_account: listing expiring tokens failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memuat token user"})
		return
	}
	if users == nil {
		users = []ExpiringUser{}
	}
	c.JSON(http.StatusOK, gin.H{"users": users, "warnDays": ExpiryWarnDays})
}

func (h *Handler) reset(c *gin.Context) {
	if err := h.api.Reset(c.Request.Context()); err != nil {
		log.Printf("jira_account: reset failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menghapus akun Jira"})
		return
	}
	c.Status(http.StatusNoContent)
}

func describe(err error) string {
	if errors.Is(err, ErrNotConfigured) {
		return "Belum ada akun Jira. Isi email dan API token di bawah."
	}
	if errors.Is(err, ErrRejected) {
		return "Jira menolak credential ini (token salah atau sudah kedaluwarsa)."
	}
	return "Belum bisa menghubungi Jira."
}
