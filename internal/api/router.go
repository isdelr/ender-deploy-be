package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/isdelr/ender-deploy-be/internal/api/handlers"
	"github.com/isdelr/ender-deploy-be/internal/services"
	"github.com/isdelr/ender-deploy-be/internal/websocket"
)

// NewRouter creates and configures a new Chi router.
func NewRouter(hub *websocket.Hub, serverService services.ServerServiceProvider) *chi.Mux {
	r := chi.NewRouter()

	// Basic middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// CORS configuration for development
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000"}, // Adjust for your frontend URL
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Initialize handlers
	serverHandler := handlers.NewServerHandler(serverService)
	wsHandler := handlers.NewWebSocketHandler(hub)

	// API versioning
	r.Route("/api/v1", func(r chi.Router) {
		// WebSocket connection endpoint
		r.Get("/ws", wsHandler.Serve)

		// REST API endpoints for servers
		r.Route("/servers", func(r chi.Router) {
			r.Get("/", serverHandler.GetAll)
			r.Post("/", serverHandler.Create)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", serverHandler.Get)
				r.Put("/", serverHandler.Update)
				r.Delete("/", serverHandler.Delete)
				r.Post("/action", serverHandler.PerformAction)
			})
		})

		// TODO: Add other resources like /users, /templates in a similar fashion
		// r.Route("/users", ...)
		// r.Route("/templates", ...)
	})

	return r
}
