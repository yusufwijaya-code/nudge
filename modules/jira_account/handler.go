package jira_account

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
)

type Handler struct{ api API }

func NewHandler(api API) *Handler { return &Handler{api: api} }

// AdminRoutes mounts Admin › Akun Jira on a group already behind
// AuthMiddlewareAPI + AdminMiddleware.
func AdminRoutes(g *gin.RouterGroup, h *Handler) {
	g.GET("/jira-account", h.status)
	g.POST("/jira-account/test", h.test)
	g.PUT("/jira-account", h.save)
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
}

func (h *Handler) save(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	var req saveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email dan API token wajib diisi"})
		return
	}
	editor := c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session).Email
	who, err := h.api.Save(c.Request.Context(), req.Email, req.APIToken, editor)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"ok": true, "account": who})
	case errors.Is(err, ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "email dan API token wajib diisi"})
	case errors.Is(err, ErrEncryptionKeyNotConfigured):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "CREDENTIAL_ENCRYPTION_KEY belum diatur di server, token belum bisa disimpan"})
	case errors.Is(err, ErrRejected):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Jira menolak email/API token ini. Akun lama tetap dipakai."})
	default:
		log.Printf("jira_account: save failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "belum bisa menghubungi Jira untuk memverifikasi, akun lama tetap dipakai"})
	}
}

func (h *Handler) reset(c *gin.Context) {
	if err := h.api.Reset(c.Request.Context()); err != nil {
		log.Printf("jira_account: reset failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengembalikan ke env"})
		return
	}
	c.Status(http.StatusNoContent)
}

func describe(err error) string {
	if errors.Is(err, ErrRejected) {
		return "Jira menolak credential ini (token salah atau sudah kedaluwarsa)."
	}
	return "Belum bisa menghubungi Jira."
}
