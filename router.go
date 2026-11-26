package main

import (
	"net/http"
	"nudge/modules/app_credential"
	"nudge/modules/app_setting"
	"nudge/modules/auto_done"
	"nudge/modules/onboarding"
	"nudge/modules/signup"
	"nudge/modules/time_report"

	"github.com/gin-gonic/gin"

	"nudge/config"
	"nudge/modules/admin_dashboard"
	"nudge/modules/admin_dashboard/admin_dashboard_handler"
	"nudge/modules/admin_roster"
	"nudge/modules/auth"
	"nudge/modules/auth/auth_handler"
	"nudge/modules/auth/auth_middleware"
	"nudge/modules/auth/auth_service"
	"nudge/modules/board_scope"
	"nudge/modules/chat_app_activation"
	"nudge/modules/chat_log/chat_log_handler"
	"nudge/modules/issue_create/issue_create_handler"
	"nudge/modules/jira_account"
	"nudge/modules/sprint_report"
	"nudge/modules/sprint_report/sprint_report_handler"
	"nudge/modules/tracking_rule"
	"nudge/modules/user_avatar/user_avatar_handler"
	"nudge/modules/user_dashboard"
	"nudge/modules/user_dashboard/user_dashboard_handler"
	"nudge/modules/user_leave"
	"nudge/modules/user_leave/user_leave_handler"
	"nudge/modules/work_calendar"
)

// SetupRouter mounts every route under cfg.BasePath — labcup.tech hosts
// several repos behind one nginx vhost, each claiming its own prefix with
// NO stripping by nginx (matching the pre-existing /nudge convention
// from the old registration page), so this app must own its full path
// itself rather than assume it's ever served from domain root.
func SetupRouter(
	cfg *config.AppConfig,
	authSvc auth_service.AuthService,
	authHandler auth_handler.AuthHandler,
	userDashboardHandler user_dashboard_handler.UserDashboardHandler,
	adminDashboardHandler admin_dashboard_handler.AdminDashboardHandler,
	sprintReportHandler sprint_report_handler.SprintReportHandler,
	adminUserSettings *user_dashboard_handler.SettingsHandler,
	userAvatars *user_avatar_handler.UserAvatarHandler,
	userLeaves *user_leave_handler.UserLeaveHandler,
	// adminGuards run on every admin route that carries :userID (the owner guard).
	adminGuards ...gin.HandlerFunc,
) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	base := r.Group(cfg.BasePath)

	base.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	// Bare BasePath (someone just types/bookmarks the /nudge prefix
	// with nothing after it) goes straight to the dashboard — AuthMiddleware
	// further down the chain bounces unauthenticated visitors to login same
	// as hitting /dashboard directly.
	base.GET("", redirectToDashboardHandler(cfg))
	r.NoRoute(notFoundHandler())

	auth.Router(base, authHandler)

	apiAuth := base.Group("/api/auth")
	apiAuth.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg))
	apiAuth.GET("/me", authHandler.Me)

	dashboardAPI := base.Group("/api/dashboard")
	dashboardAPI.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg))
	user_dashboard.Router(dashboardAPI, userDashboardHandler)
	sprint_report.Router(dashboardAPI, sprintReportHandler)
	// Tech Spec "Papan Tiket & Cuti": the session user's leave days.
	user_leave.Router(dashboardAPI, userLeaves)
	// Jira profile pictures (Tech Spec "Admin Panel Revamp").
	dashboardAPI.GET("/me/avatar", userAvatars.Mine)

	dashboardAdmin := base.Group("/dashboard/admin")
	dashboardAdmin.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg))
	dashboardAdmin.Use(auth_middleware.AdminMiddleware(cfg))
	dashboardAdmin.Use(adminGuards...)

	apiDashboardAdmin := dashboardAPI.Group("/admin")
	apiDashboardAdmin.Use(auth_middleware.AdminMiddleware(cfg))
	apiDashboardAdmin.Use(adminGuards...)

	admin_dashboard.Router(dashboardAdmin, apiDashboardAdmin, adminDashboardHandler, adminUserSettings)
	apiDashboardAdmin.GET("/users/:userID/avatar", userAvatars.ForUser)

	return r
}

func redirectToDashboardHandler(cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Redirect(http.StatusFound, cfg.FrontendURL+cfg.BasePath+"/dashboard")
	}
}

// notFoundHandler is registered via r.NoRoute (not a BasePath-scoped group —
// gin has no NoRoute on *RouterGroup), but that's fine: nginx only ever
// forwards this app requests already under cfg.BasePath, so any unmatched
// path reaching this process is a genuine 404 for it. The frontend is a
// separate Angular app, so the backend only ever answers with JSON.
func notFoundHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	}
}

// ChatAppRoutes are the Tech Spec "Nudge sebagai Google Chat App"
// endpoints. Events is nil when the Chat App is not configured.
type ChatAppRoutes struct {
	Events      gin.HandlerFunc
	Activation  *chat_app_activation.Handler
	AdminTarget user_dashboard_handler.TargetResolver
	AdminChange user_dashboard_handler.ChangeListener
}

// MountBoardScope mounts the session user's board choice (web navbar),
// shared with the chat commands.
func MountBoardScope(r *gin.Engine, cfg *config.AppConfig, authSvc auth_service.AuthService, h *board_scope.Handler) {
	api := r.Group(cfg.BasePath + "/api/dashboard")
	api.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg))
	api.GET("/board-scope", h.Get)
	api.PUT("/board-scope", h.Put)
}

// MountChatLog mounts the admin "Riwayat pesan" list (admin only).
func MountChatLog(r *gin.Engine, cfg *config.AppConfig, authSvc auth_service.AuthService, h *chat_log_handler.Handler) {
	admin := r.Group(cfg.BasePath + "/api/dashboard/admin")
	admin.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg), auth_middleware.AdminMiddleware(cfg))
	admin.GET("/chat-logs", h.List)
	admin.GET("/chat-logs/conversations", h.Conversations)
	admin.POST("/chat-logs/send", h.Send)
}

// MountChatApp mounts the Chat App routes next to SetupRouter's.
func MountChatApp(r *gin.Engine, cfg *config.AppConfig, authSvc auth_service.AuthService, routes ChatAppRoutes) {
	base := r.Group(cfg.BasePath)
	if routes.Events != nil {
		base.POST("/api/chat/events", routes.Events)
	}
	a := routes.Activation
	if a == nil {
		return
	}
	// Session routes. /auth/chat-app/* answers 401 JSON without a session;
	// it is only ever opened from the dashboard by a logged-in user.
	session := base.Group("")
	session.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg))
	session.GET("/auth/chat-app/activate", a.Activate)
	session.GET("/auth/chat-app/callback", a.Callback)
	session.GET("/api/dashboard/chat-app/status", a.Status(user_dashboard_handler.SessionTarget))
	session.PUT("/api/dashboard/settings/chat-sender", a.SetSender(user_dashboard_handler.SessionTarget, nil))

	admin := base.Group("/api/dashboard/admin")
	admin.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg), auth_middleware.AdminMiddleware(cfg))
	admin.GET("/users/:userID/chat-app/status", a.Status(routes.AdminTarget))
	admin.PUT("/users/:userID/settings/chat-sender", a.SetSender(routes.AdminTarget, routes.AdminChange))
}

// MountIssueCreate mounts "Buat Card" on Papan Tiket (Tech Spec "Papan Tiket Revamp & Buat Card Jira").
func MountIssueCreate(r *gin.Engine, cfg *config.AppConfig, authSvc auth_service.AuthService, h *issue_create_handler.Handler) {
	api := r.Group(cfg.BasePath + "/api/dashboard")
	api.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg))
	issue_create_handler.Routes(api, h)
}

// MountWorkCalendar mounts the Kalender page's data (any logged-in user).
func MountWorkCalendar(r *gin.Engine, cfg *config.AppConfig, authSvc auth_service.AuthService, h *work_calendar.Handler) {
	api := r.Group(cfg.BasePath + "/api/dashboard")
	api.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg))
	work_calendar.UserRoutes(api, h)
}

// MountAdminRoster mounts promote/demote admin (Admin > User). The route needs
// an admin; the service lets only the owner use it.
func MountAdminRoster(r *gin.Engine, cfg *config.AppConfig, authSvc auth_service.AuthService, h *admin_roster.Handler) {
	admin := r.Group(cfg.BasePath + "/api/dashboard/admin")
	admin.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg), auth_middleware.AdminMiddleware(cfg))
	admin_roster.AdminRoutes(admin, h)
}

// MountJiraAccount mounts Admin › Akun Jira (admin only): the central Jira
// account the bot acts as.
func MountJiraAccount(r *gin.Engine, cfg *config.AppConfig, authSvc auth_service.AuthService, h *jira_account.Handler) {
	admin := r.Group(cfg.BasePath + "/api/dashboard/admin")
	admin.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg), auth_middleware.AdminMiddleware(cfg))
	jira_account.AdminRoutes(admin, h)
}

// MountTrackingRules mounts Admin › Aturan Board (admin only) and the board
// workflow statuses Pengaturan offers (any logged-in user) — Tech Spec
// "Aturan Board (Admin CRUD)".
func MountTrackingRules(r *gin.Engine, cfg *config.AppConfig, authSvc auth_service.AuthService, h *tracking_rule.Handler) {
	user := r.Group(cfg.BasePath + "/api/dashboard")
	user.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg))
	tracking_rule.UserRoutes(user, h)

	admin := r.Group(cfg.BasePath + "/api/dashboard/admin")
	admin.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg), auth_middleware.AdminMiddleware(cfg))
	tracking_rule.AdminRoutes(admin, h)
}

// MountSignup mounts the registration API. It has no session middleware: the
// visitor has no account yet, and the signed cookie from the Google callback is
// the identity.
func MountSignup(r *gin.Engine, cfg *config.AppConfig, h *signup.Handler) {
	signup.Routes(r.Group(cfg.BasePath), h)
}

// MountOnboarding mounts step 2 of joining (the user's boards and their setup).
func MountOnboarding(r *gin.Engine, cfg *config.AppConfig, authSvc auth_service.AuthService, h *onboarding.Handler) {
	api := r.Group(cfg.BasePath + "/api")
	api.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg))
	onboarding.Routes(api, h)
}

// MountAppSettings mounts Admin › Pengaturan Aplikasi (admin only).
func MountAppSettings(r *gin.Engine, cfg *config.AppConfig, authSvc auth_service.AuthService, h *app_setting.Handler) {
	admin := r.Group(cfg.BasePath + "/api/dashboard/admin")
	admin.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg), auth_middleware.AdminMiddleware(cfg))
	app_setting.AdminRoutes(admin, h)
}

// MountAutoDone mounts the "geser otomatis ke selesai" switch (Pengaturan > Umum).
func MountAutoDone(r *gin.Engine, cfg *config.AppConfig, authSvc auth_service.AuthService, h *auto_done.Handler) {
	api := r.Group(cfg.BasePath + "/api/dashboard")
	api.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg))
	auto_done.Routes(api, h)
}

// MountAppCredentials mounts Pengaturan Aplikasi › Kredensial & token (admin only).
func MountAppCredentials(r *gin.Engine, cfg *config.AppConfig, authSvc auth_service.AuthService, h *app_credential.Handler) {
	admin := r.Group(cfg.BasePath + "/api/dashboard/admin")
	admin.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg), auth_middleware.AdminMiddleware(cfg))
	app_credential.AdminRoutes(admin, h)
}

// MountTimeReport mounts the user's Laporan (session user only).
func MountTimeReport(r *gin.Engine, cfg *config.AppConfig, authSvc auth_service.AuthService, h *time_report.Handler) {
	api := r.Group(cfg.BasePath + "/api/dashboard")
	api.Use(auth_middleware.AuthMiddlewareAPI(authSvc, cfg))
	time_report.Routes(api, h)
}
