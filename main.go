package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/isdelr/ender-deploy-be/internal/api"
	"github.com/isdelr/ender-deploy-be/internal/config"
	"github.com/isdelr/ender-deploy-be/internal/database"
	"github.com/isdelr/ender-deploy-be/internal/docker"
	"github.com/isdelr/ender-deploy-be/internal/services"
	"github.com/isdelr/ender-deploy-be/internal/websocket"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Ensure the base directory for server data exists
	if err := os.MkdirAll(cfg.ServerDataBase, 0755); err != nil {
		log.Fatalf("Failed to create base server data directory: %v", err)
	}

	// Set up database
	db, err := database.New(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		log.Fatalf("Failed to apply database migrations: %v", err)
	}

	// Set up Docker client
	dockerClient, err := docker.New()
	if err != nil {
		log.Fatalf("Failed to initialize Docker client: %v", err)
	}

	// Set up WebSocket Hub
	hub := websocket.NewHub()
	go hub.Run()

	// Set up services
	// In a real app, you would have separate services for users, templates, etc.
	serverService := services.NewServerService(db, dockerClient, hub, cfg.ServerDataBase)

	// Set up router
	router := api.NewRouter(hub, serverService)

	// Set up server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.ServerPort),
		Handler: router,
	}

	// Graceful shutdown
	go func() {
		log.Printf("Server starting on port %d\n", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe(): %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exiting")
}
