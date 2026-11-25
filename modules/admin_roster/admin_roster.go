// Package admin_roster lets the owner promote users to admin (Admin › User).
// A promoted admin has every admin feature; the one thing an admin cannot do is
// change the owner's account (see ProtectOwner), and only the owner can promote
// or demote.
package admin_roster

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"nudge/modules/admin_roster/admin_roster_holder"
	"nudge/modules/auth/auth_dto"
	"nudge/modules/auth/auth_middleware"
)

var (
	ErrOwnerOnly    = errors.New("admin_roster: only the owner can promote or demote")
	ErrOwnerFixed   = errors.New("admin_roster: the owner is always an admin")
	ErrUserNotFound = errors.New("admin_roster: user not found")
)

// RefreshInterval: a change made outside this process (a manual DB edit)
// applies within this window.
const RefreshInterval = 5 * time.Minute

// Repository reads and writes ms_user.isAdmin.
type Repository interface {
	ListAdminEmails(ctx context.Context) ([]string, error)
	SetAdmin(ctx context.Context, userID int64, admin bool) error
	// FindEmail returns the email of userID; found=false when there is no such user.
	FindEmail(ctx context.Context, userID int64) (email string, found bool, err error)
}

type repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) Repository { return &repository{db: db} }

func (r *repository) ListAdminEmails(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT email FROM ms_user WHERE isAdmin = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, err
		}
		out = append(out, email)
	}
	return out, rows.Err()
}

func (r *repository) SetAdmin(ctx context.Context, userID int64, admin bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ms_user SET isAdmin = ? WHERE userID = ?`, admin, userID)
	return err
}

func (r *repository) FindEmail(ctx context.Context, userID int64) (string, bool, error) {
	var email string
	err := r.db.QueryRowContext(ctx, `SELECT email FROM ms_user WHERE userID = ?`, userID).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return email, err == nil, err
}

// API is what the handler needs from Service.
type API interface {
	SetAdmin(ctx context.Context, actorEmail string, targetUserID int64, admin bool) error
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

// Load puts the promoted admins in force.
func (s *Service) Load(ctx context.Context) error {
	emails, err := s.repo.ListAdminEmails(ctx)
	if err != nil {
		return err
	}
	admin_roster_holder.SetAdmins(emails)
	return nil
}

// Run reloads every RefreshInterval until ctx ends.
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(RefreshInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.Load(ctx); err != nil {
				log.Printf("admin_roster: refresh failed, keeping the last roster: %v", err)
			}
		}
	}
}

// SetAdmin promotes or demotes targetUserID. Only the owner may, and the owner
// cannot be changed: they are always an admin.
func (s *Service) SetAdmin(ctx context.Context, actorEmail string, targetUserID int64, admin bool) error {
	if !admin_roster_holder.IsOwner(actorEmail) {
		return ErrOwnerOnly
	}
	email, found, err := s.repo.FindEmail(ctx, targetUserID)
	if err != nil {
		return err
	}
	if !found {
		return ErrUserNotFound
	}
	if admin_roster_holder.IsOwner(email) {
		return ErrOwnerFixed
	}
	if err := s.repo.SetAdmin(ctx, targetUserID, admin); err != nil {
		return err
	}
	return s.Load(ctx)
}

type Handler struct{ api API }

func NewHandler(api API) *Handler { return &Handler{api: api} }

// AdminRoutes mounts the promote/demote action on a group already behind
// AuthMiddlewareAPI + AdminMiddleware.
func AdminRoutes(g *gin.RouterGroup, h *Handler) {
	g.PUT("/users/:userID/admin", h.setAdmin)
}

type setAdminRequest struct {
	Admin *bool `json:"admin"`
}

func (h *Handler) setAdmin(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("userID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid userID"})
		return
	}
	var req setAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Admin == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "admin wajib diisi (true/false)"})
		return
	}
	actor := c.MustGet(auth_middleware.CurrentUserKey).(auth_dto.Session)
	switch err := h.api.SetAdmin(c.Request.Context(), actor.Email, id, *req.Admin); {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"ok": true, "isAdmin": *req.Admin})
	case errors.Is(err, ErrOwnerOnly):
		c.JSON(http.StatusForbidden, gin.H{"error": "Hanya pemilik yang bisa menjadikan atau mencabut admin."})
	case errors.Is(err, ErrOwnerFixed):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Pemilik selalu menjadi admin dan tidak bisa diubah."})
	case errors.Is(err, ErrUserNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "user tidak ditemukan"})
	default:
		log.Printf("admin_roster: setting admin of user %d failed: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menyimpan, coba lagi"})
	}
}

// OwnerEmailLookup finds the email of a user id (for the guard).
type OwnerEmailLookup func(ctx context.Context, userID int64) (email string, found bool, err error)

// ProtectOwner stops every change to the owner's account by anyone but the
// owner. It sits on the admin route groups that carry :userID: any request
// other than a read (GET/HEAD/OPTIONS) on /users/:userID/... whose target is the
// owner is refused with 403, so a new route under it is protected too. Reading
// the owner's page stays allowed; if the target cannot be looked up the request
// is refused rather than let through.
func ProtectOwner(lookup OwnerEmailLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		raw := c.Param("userID")
		if raw == "" {
			c.Next()
			return
		}
		session, ok := c.Get(auth_middleware.CurrentUserKey)
		if !ok {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		if admin_roster_holder.IsOwner(session.(auth_dto.Session).Email) {
			c.Next()
			return
		}
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			c.Next() // the handler answers 400 for a bad id
			return
		}
		email, found, err := lookup(c.Request.Context(), id)
		switch {
		case err != nil:
			log.Printf("admin_roster: owner guard lookup of user %d failed: %v", id, err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "gagal memeriksa akun, coba lagi"})
		case found && admin_roster_holder.IsOwner(email):
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Akun pemilik hanya bisa diubah oleh pemilik."})
		default:
			c.Next()
		}
	}
}
