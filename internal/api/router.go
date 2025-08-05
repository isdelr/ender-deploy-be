package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/isdelr/ender-deploy-be/internal/api/handlers"
	"github.com/isdelr/ender-deploy-be/internal/auth"
	"github.com/isdelr/ender-deploy-be/internal/services"
	"github.com/isdelr/ender-deploy-be/internal/websocket"
)

// NewRouter creates and annotes a new Chi router.
func NewRouter(hub *websocket.Hub, serverService services.ServerServiceProvider, templateService services.TemplateServiceProvider, userService services.UserServiceProvider, backupService services.BackupServiceProvider, eventService services.EventServiceProvider, scheduleService services.ScheduleServiceProvider) *chi.Mux {
	r := chi.NewRouter()

	// Basic middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// CORS configuration for development
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "http://127.0.0.1:3000"}, // Adjust for your frontend URL
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Initialize handlers
	serverHandler := handlers.NewServerHandler(serverService)
	templateHandler := handlers.NewTemplateHandler(templateService)
	userHandler := handlers.NewUserHandler(userService)
	wsHandler := handlers.NewWebSocketHandler(hub, serverService)
	backupHandler := handlers.NewBackupHandler(backupService)
	eventHandler := handlers.NewEventHandler(eventService)
	scheduleHandler := handlers.NewScheduleHandler(scheduleService)

	// API versioning
	r.Route("/api/v1", func(r chi.Router) {
		// Public routes (auth)
		r.Post("/register", userHandler.Register)
		r.Post("/login", userHandler.Login)
		r.Get("/available-port", serverHandler.BindPort)

		// WebSocket connection endpoint - protected by JWT in practice
		// The websocket upgrade itself doesn't use the middleware directly,
		// but the initial HTTP request should be authenticated.
		r.Route("/ws", func(r chi.Router) {
			r.Use(auth.JWTMiddleware())
			r.Get("/global", wsHandler.Serve)
			r.Get("/servers/{id}", wsHandler.Serve)
		})

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(auth.JWTMiddleware())

			// Dashboard & Events
			r.Get("/dashboard/stats", serverHandler.GetDashboardStats)
			r.Get("/events", eventHandler.GetRecent)

			// REST API endpoints for servers
			r.Route("/servers", func(r chi.Router) {
				r.Get("/", serverHandler.GetAll)
				r.Post("/", serverHandler.Create)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", serverHandler.Get)
					r.Put("/", serverHandler.Update)
					r.Delete("/", serverHandler.Delete)
					r.Post("/action", serverHandler.PerformAction)
					r.Post("/command", serverHandler.SendServerConsoleCommand)

					// Server Settings
					r.Get("/settings", serverHandler.GetServerSettings)
					r.Post("/settings", serverHandler.UpdateServerSettings)

					// Resource History
					r.Get("/resources/history", serverHandler.GetServerResourceHistory)

					// Player Management
					r.Get("/players", serverHandler.GetOnlinePlayers)
					r.Post("/players/manage", serverHandler.ManagePlayer)

					// File Management
					r.Get("/files", serverHandler.ListServerFiles)
					r.Get("/files/content", serverHandler.GetServerFileContent)
					r.Post("/files/update", serverHandler.UpdateServerFile)

					// Backup Management
					r.Route("/backups", func(r chi.Router) {
						r.Get("/", backupHandler.GetAllForServer)
						r.Post("/", backupHandler.Create)
						r.Route("/{backupId}", func(r chi.Router) {
							r.Post("/restore", backupHandler.Restore)
							r.Delete("/", backupHandler.Delete)
						})
					})

					// Schedule Management
					r.Route("/schedules", func(r chi.Router) {
						r.Get("/", scheduleHandler.GetAllForServer)
						r.Post("/", scheduleHandler.Create)
						r.Route("/{scheduleId}", func(r chi.Router) {
							r.Put("/", scheduleHandler.Update)
							r.Delete("/", scheduleHandler.Delete)
						})
					})
				})
			})

			// REST API endpoints for templates
			r.Route("/templates", func(r chi.Router) {
				r.Get("/", templateHandler.GetAll)
				r.Post("/", templateHandler.Create)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", templateHandler.Get)
					r.Put("/", templateHandler.Update)
					r.Delete("/", templateHandler.Delete)
				})
			})

			// REST API endpoints for users
			r.Route("/users", func(r chi.Router) {
				r.Get("/me", userHandler.GetMe) // Get the current authenticated user
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", userHandler.Get)
					r.Put("/", userHandler.Update)
					r.Delete("/", userHandler.Delete)
					r.Post("/change-password", userHandler.ChangePassword)
				})
			})
		})
	})

	return r

}
