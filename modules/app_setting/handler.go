package app_setting

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
	Get(ctx context.Context) (Settings, error)
	SetDeviationCap(ctx context.Context, percent float64, editor string) (Settings, error)
}

type Handler struct{ api API }

func NewHandler(api API) *Handler { return &Handler{api: api} }

// AdminRoutes mounts Admin › Pengaturan Aplikasi on a group already behind
// AuthMiddlewareAPI + AdminMiddleware.
func AdminRoutes(g *gin.RouterGroup, h *Handler) {
	g.GET("/app-settings", h.get)
	g.PUT("/app-settings/deviation", h.setDeviation)
}

func (h *Handler) get(c *gin.Context) {
	s, err := h.api.Get(c.Request.Context())
	if err != nil {
		log.Printf("app_setting: get failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memuat pengaturan"})
		return
	}
	c.JSON(http.StatusOK, s)
}

func (h *Handler) setDeviation(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<10)
	var req struct {
		DeviationCapPercent *float64 `json:"deviationCapPercent"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.DeviationCapPercent == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Isi nilai deviasi"})
		return
	}
	editor := c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session).Email
	s, err := h.api.SetDeviationCap(c.Request.Context(), *req.DeviationCapPercent, editor)
	var v *ValidationError
	switch {
	case errors.As(err, &v):
		c.JSON(http.StatusBadRequest, gin.H{"error": v.Message})
	case err != nil:
		log.Printf("app_setting: save failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menyimpan pengaturan"})
	default:
		c.JSON(http.StatusOK, s)
	}
}
