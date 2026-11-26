package onboarding

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/tracking_rule"
)

// API is what the handler needs from Service.
type API interface {
	Boards(ctx context.Context, email string, fresh bool) ([]Board, error)
	Form(ctx context.Context, email string, boardID int64) (Form, error)
	Setup(ctx context.Context, email string, boardID int64, in tracking_rule.RuleInput) (Result, error)
}

type Handler struct{ api API }

func NewHandler(api API) *Handler { return &Handler{api: api} }

// Routes mounts /onboarding on a group already behind AuthMiddlewareAPI.
func Routes(g *gin.RouterGroup, h *Handler) {
	o := g.Group("/onboarding")
	o.GET("/boards", h.boards)
	o.GET("/boards/:boardId/form", h.form)
	o.POST("/boards/:boardId/setup", h.setup)
}

func email(c *gin.Context) string {
	return c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session).Email
}

func boardID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("boardId"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_board", "message": "Board tidak valid"})
		return 0, false
	}
	return id, true
}

func fail(c *gin.Context, err error) {
	var v *tracking_rule.ValidationError
	switch {
	case errors.Is(err, ErrBoardNotYours):
		c.JSON(http.StatusNotFound, gin.H{"error": "board_not_yours", "message": "Board ini tidak ada di sprint aktif kamu"})
	case errors.Is(err, ErrAlreadyRegistered):
		c.JSON(http.StatusConflict, gin.H{"error": "already_registered", "message": "Board ini sudah terdaftar"})
	case errors.As(err, &v):
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "message": "Periksa isian yang ditandai", "fields": v.Fields})
	case errors.Is(err, tracking_rule.ErrProjectNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "project_not_found", "message": "Project board ini tidak ditemukan di Jira"})
	default:
		log.Printf("onboarding: %s %s failed: %v", c.Request.Method, c.FullPath(), err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "jira_unavailable", "message": "Gagal membaca data Jira, coba lagi sebentar lagi"})
	}
}

func (h *Handler) boards(c *gin.Context) {
	boards, err := h.api.Boards(c.Request.Context(), email(c), c.Query("fresh") == "1")
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"boards": boards})
}

func (h *Handler) form(c *gin.Context) {
	id, ok := boardID(c)
	if !ok {
		return
	}
	f, err := h.api.Form(c.Request.Context(), email(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, f)
}

func (h *Handler) setup(c *gin.Context) {
	id, ok := boardID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var in tracking_rule.RuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "message": "Isian tidak valid"})
		return
	}
	// The project comes from the board, never from the body; the schedule of
	// special activities is an admin matter (Aturan Board), not part of onboarding.
	in.Excluded, in.CreateFields = nil, nil
	res, err := h.api.Setup(c.Request.Context(), email(c), id, in)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, res)
}
