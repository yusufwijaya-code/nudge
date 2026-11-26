package time_report

import (
	"bytes"
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
	Start(ctx context.Context, email string, p Params) (Progress, error)
	Progress(email, id string) (Progress, error)
	Cancel(email, id string) error
	Result(email, id string) (*Report, error)
}

type Handler struct{ api API }

func NewHandler(api API) *Handler { return &Handler{api: api} }

// Routes mounts the Laporan on a group already behind AuthMiddlewareAPI. A report
// only ever covers the signed-in user's own tickets.
func Routes(g *gin.RouterGroup, h *Handler) {
	g.POST("/reports/jobs", h.start)
	g.GET("/reports/jobs/:id", h.progress)
	g.DELETE("/reports/jobs/:id", h.cancel)
	g.GET("/reports/jobs/:id/pdf", h.pdf)
}

func email(c *gin.Context) string {
	return c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session).Email
}

func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidRange):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_range", "message": "Rentang tanggal tidak valid (maksimal 1 tahun, tanggal akhir tidak sebelum tanggal awal)."})
	case errors.Is(err, ErrBoardUnknown):
		c.JSON(http.StatusBadRequest, gin.H{"error": "board_unknown", "message": "Board ini belum terdaftar di Nudge."})
	case errors.Is(err, ErrJobNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "Laporan tidak ditemukan atau sudah kedaluwarsa. Tampilkan ulang."})
	case errors.Is(err, ErrJobNotFinished):
		c.JSON(http.StatusConflict, gin.H{"error": "not_finished", "message": "Laporan belum selesai dihitung."})
	default:
		log.Printf("time_report: %s %s failed: %v", c.Request.Method, c.FullPath(), err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "report_failed", "message": "Gagal membuat laporan, coba lagi."})
	}
}

func (h *Handler) start(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<10)
	var p Params
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "message": "Isian tidak valid."})
		return
	}
	pr, err := h.api.Start(c.Request.Context(), email(c), p)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, pr)
}

func (h *Handler) progress(c *gin.Context) {
	pr, err := h.api.Progress(email(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, pr)
}

func (h *Handler) cancel(c *gin.Context) {
	if err := h.api.Cancel(email(c), c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) pdf(c *gin.Context) {
	r, err := h.api.Result(email(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	var buf bytes.Buffer
	if err := RenderPDF(&buf, r); err != nil {
		log.Printf("time_report: rendering the PDF failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "pdf_failed", "message": "Gagal membuat PDF, coba lagi."})
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+FileName(r)+`"`)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "application/pdf", buf.Bytes())
}
