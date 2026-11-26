package app_credential

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
	List(ctx context.Context) ([]Item, error)
	SetValue(ctx context.Context, key, value, expiresAt, editor string) (Item, error)
	SetExpiry(ctx context.Context, key, expiresAt, editor string) (Item, error)
	Clear(ctx context.Context, key, editor string) (Item, error)
	Test(ctx context.Context, key string) (TestResult, error)
	Expiring(ctx context.Context) ([]Expiring, error)
}

type Handler struct{ api API }

func NewHandler(api API) *Handler { return &Handler{api: api} }

// AdminRoutes mounts Pengaturan Aplikasi › Kredensial & token on a group already behind
// AuthMiddlewareAPI + AdminMiddleware.
func AdminRoutes(g *gin.RouterGroup, h *Handler) {
	g.GET("/app-credentials", h.list)
	// Static segment first: gin matches it before the :key wildcard.
	g.GET("/app-credentials/expiring", h.expiring)
	g.PUT("/app-credentials/:key", h.setValue)
	g.PUT("/app-credentials/:key/expiry", h.setExpiry)
	g.DELETE("/app-credentials/:key", h.clear)
	g.POST("/app-credentials/:key/test", h.test)
}

const maxBody = 8 << 10

func editor(c *gin.Context) string {
	return c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session).Email
}

func fail(c *gin.Context, err error) {
	var v *ValidationError
	switch {
	case errors.As(err, &v):
		c.JSON(http.StatusBadRequest, gin.H{"error": v.Message})
	case errors.Is(err, ErrUnknownKey):
		c.JSON(http.StatusNotFound, gin.H{"error": "kredensial tidak dikenal"})
	case errors.Is(err, ErrNotEditable):
		c.JSON(http.StatusBadRequest, gin.H{"error": "kredensial ini tidak bisa diganti dari web; ganti di server lalu catat tanggal kedaluwarsanya di sini"})
	case errors.Is(err, ErrEncryptionKeyNotConfigured):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "server belum punya CREDENTIAL_ENCRYPTION_KEY, jadi nilai belum bisa disimpan"})
	default:
		log.Printf("app_credential: %s %s failed: %v", c.Request.Method, c.FullPath(), err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memproses kredensial"})
	}
}

func (h *Handler) list(c *gin.Context) {
	items, err := h.api.List(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *Handler) expiring(c *gin.Context) {
	items, err := h.api.Expiring(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	if items == nil {
		items = []Expiring{}
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *Handler) setValue(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBody)
	var req struct {
		Value     string `json:"value"`
		ExpiresAt string `json:"expiresAt"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "isian tidak valid"})
		return
	}
	it, err := h.api.SetValue(c.Request.Context(), c.Param("key"), req.Value, req.ExpiresAt, editor(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, it)
}

func (h *Handler) setExpiry(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBody)
	var req struct {
		ExpiresAt string `json:"expiresAt"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "isian tidak valid"})
		return
	}
	it, err := h.api.SetExpiry(c.Request.Context(), c.Param("key"), req.ExpiresAt, editor(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, it)
}

func (h *Handler) clear(c *gin.Context) {
	it, err := h.api.Clear(c.Request.Context(), c.Param("key"), editor(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, it)
}

func (h *Handler) test(c *gin.Context) {
	res, err := h.api.Test(c.Request.Context(), c.Param("key"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}
