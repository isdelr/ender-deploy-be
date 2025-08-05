package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/isdelr/ender-deploy-be/internal/api"
	"github.com/isdelr/ender-deploy-be/internal/config"
	"github.com/isdelr/ender-deploy-be/internal/database"
	"github.com/isdelr/ender-deploy-be/internal/docker"
	"github.com/isdelr/ender-deploy-be/internal/logger" // Import the new logger package
	"github.com/isdelr/ender-deploy-be/internal/monitoring"
	"github.com/isdelr/ender-deploy-be/internal/services"
	"github.com/isdelr/ender-deploy-be/internal/websocket"
	"github.com/rs/zerolog/log" // Import zerolog's global logger
)

func main() {
	// Initialize the structured, colorized logger as the first step.
	logger.Init()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Ensure the base directory for server data exists
	if err := os.MkdirAll(cfg.ServerDataBase, 0755); err != nil {
		log.Fatal().Err(err).Str("path", cfg.ServerDataBase).Msg("Failed to create base server data directory")
	}

	// Ensure the base directory for backups exists
	if err := os.MkdirAll(cfg.BackupPath, 0755); err != nil {
		log.Fatal().Err(err).Str("path", cfg.BackupPath).Msg("Failed to create base backup directory")
	}

	// Set up database
	db, err := database.New(cfg.DatabasePath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		log.Fatal().Err(err).Msg("Failed to apply database migrations")
	}

	// Set up Docker client
	dockerClient, err := docker.New()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize Docker client")
	}

	// Set up WebSocket Hub
	hub := websocket.NewHub()
	go hub.Run()

	// Set up services
	templateService := services.NewTemplateService(db)
	userService := services.NewUserService(db)
	eventService := services.NewEventService(db)
	serverService := services.NewServerService(db, dockerClient, hub, templateService, eventService, cfg.ServerDataBase)
	backupService := services.NewBackupService(db, serverService, eventService, cfg.BackupPath)
	scheduleService := services.NewScheduleService(db, eventService)

	// Set up and run the background stats updater
	statUpdater := monitoring.NewStatUpdater(db, dockerClient, serverService, eventService)
	go statUpdater.Run()

	// Set up and run the background scheduler
	scheduler := monitoring.NewScheduler(scheduleService, serverService, backupService, eventService)
	go scheduler.Run()

	// Set up router
	router := api.NewRouter(hub, serverService, templateService, userService, backupService, eventService, scheduleService)

	// Set up server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.ServerPort),
		Handler: router,
	}

	// Graceful shutdown
	go func() {
		log.Info().Int("port", cfg.ServerPort).Msg("Server starting")
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("ListenAndServe() failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("Shutting down server...")

	statUpdater.Stop() // Stop the monitoring service
	scheduler.Stop()   // Stop the scheduler

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exiting")
}
