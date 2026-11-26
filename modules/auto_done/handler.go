package auto_done

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
)

// API is what the handler needs from Service.
type API interface {
	Status(ctx context.Context, userID int64, email string) (Status, error)
	SetEnabled(ctx context.Context, userID int64, email string, enabled bool) (Status, error)
}

type Handler struct{ api API }

func NewHandler(api API) *Handler { return &Handler{api: api} }

// Routes mounts the switch (Pengaturan > Umum) on a group already behind
// AuthMiddlewareAPI. It always acts on the signed-in user.
func Routes(g *gin.RouterGroup, h *Handler) {
	g.GET("/settings/auto-done", h.get)
	g.PUT("/settings/auto-done", h.put)
}

func session(c *gin.Context) auth_dto.Session {
	return c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session)
}

func (h *Handler) get(c *gin.Context) {
	s := session(c)
	st, err := h.api.Status(c.Request.Context(), s.UserID, s.Email)
	if err != nil {
		log.Printf("auto_done: status of %s failed: %v", s.Email, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memuat pengaturan geser otomatis"})
		return
	}
	c.JSON(http.StatusOK, st)
}

func (h *Handler) put(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<10)
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enabled wajib diisi (true/false)"})
		return
	}
	s := session(c)
	st, err := h.api.SetEnabled(c.Request.Context(), s.UserID, s.Email, *req.Enabled)
	switch {
	case errors.Is(err, ErrCredentialRequired):
		c.JSON(http.StatusPreconditionFailed, gin.H{"error": "credential_required", "message": "Isi Jira API Key Pribadi dulu supaya perpindahan status tercatat atas namamu."})
	case err != nil:
		log.Printf("auto_done: saving the switch of %s failed: %v", s.Email, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menyimpan pengaturan geser otomatis"})
	default:
		c.JSON(http.StatusOK, st)
	}
}
