package monitoring

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/client"
	"github.com/isdelr/ender-deploy-be/internal/docker"
	"github.com/isdelr/ender-deploy-be/internal/models"
	"github.com/isdelr/ender-deploy-be/internal/services"
)

// StatUpdater is responsible for periodically fetching and updating server stats.
type StatUpdater struct {
	db           *sql.DB
	docker       *docker.Client
	serverSvc    services.ServerServiceProvider
	eventSvc     services.EventServiceProvider
	ticker       *time.Ticker
	done         chan bool
	highCpuAlert map[string]time.Time
}

// NewStatUpdater creates a new StatUpdater.
func NewStatUpdater(db *sql.DB, docker *docker.Client, serverSvc services.ServerServiceProvider, eventSvc services.EventServiceProvider) *StatUpdater {
	return &StatUpdater{
		db:           db,
		docker:       docker,
		serverSvc:    serverSvc,
		eventSvc:     eventSvc,
		done:         make(chan bool),
		highCpuAlert: make(map[string]time.Time),
	}
}

// Run starts the periodic updates.
func (su *StatUpdater) Run() {
	log.Println("Starting background stat updater...")
	su.ticker = time.NewTicker(15 * time.Second) // Update every 15 seconds
	defer su.ticker.Stop()

	// Run once immediately on start
	su.updateAllServerStats()

	for {
		select {
		case <-su.done:
			log.Println("Stopping background stat updater.")
			return
		case <-su.ticker.C:
			su.updateAllServerStats()
		}
	}
}

// Stop halts the periodic updates.
func (su *StatUpdater) Stop() {
	su.done <- true
}

// updateAllServerStats fetches all servers from the DB and updates their stats if they are online.
func (su *StatUpdater) updateAllServerStats() {
	servers, err := su.serverSvc.GetAllServers()
	if err != nil {
		log.Printf("StatUpdater: Failed to query servers: %v", err)
		return
	}

	for _, s := range servers {
		server := s // Create a new variable to avoid capturing the loop variable in the goroutine
		if server.Status == "online" || server.Status == "starting" || server.Status == "stopping" {
			go su.updateSingleServer(&server)
		}
	}
}

// updateSingleServer fetches stats for one specific server.
func (su *StatUpdater) updateSingleServer(server *models.Server) {
	ctx := context.Background()
	stats, err := su.docker.GetContainerStats(ctx, server.DockerContainerID)
	if err != nil {
		if client.IsErrNotFound(err) && server.Status != "offline" {
			log.Printf("StatUpdater: Container for %s (%s) not found, marking as offline.", server.Name, server.ID)
			server.Status = "offline"
			server.Resources = models.ResourceUsage{}
			server.Players.Current = 0
		}
		// Don't log other errors as they are common when a container is stopping
	} else {
		// If we get stats, the container is running.
		server.Status = "online"
		server.Resources.CPU = docker.CalculateCPUPercent(stats)
		server.Resources.RAM = docker.CalculateRAMPercent(stats)
		server.Resources.Storage = su.getDirectorySizePercentage(server.DataPath)

		su.checkAndAlertForHighCPU(server)
	}

	err = su.serverSvc.UpdateServerStats(*server)
	if err != nil {
		log.Printf("StatUpdater: Failed to update server stats in DB for %s: %v", server.Name, err)
	}
}

func (su *StatUpdater) checkAndAlertForHighCPU(server *models.Server) {
	const highCpuThreshold = 90.0
	const alertCooldown = 15 * time.Minute

	if server.Resources.CPU > highCpuThreshold {
		if lastAlertTime, ok := su.highCpuAlert[server.ID]; ok {
			// If an alert was sent recently, do nothing.
			if time.Since(lastAlertTime) < alertCooldown {
				return
			}
		}
		// If CPU is high and no recent alert was sent, create one.
		msg := fmt.Sprintf("High CPU usage (%.1f%%) detected on server '%s'.", server.Resources.CPU, server.Name)
		su.eventSvc.CreateEvent("system.alert.cpu", "warn", msg, &server.ID)
		su.highCpuAlert[server.ID] = time.Now()
	}
}

// getDirectorySizePercentage calculates the size of a directory.
func (su *StatUpdater) getDirectorySizePercentage(path string) int {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	if err != nil {
		log.Printf("StatUpdater: Could not calculate size of %s: %v", path, err)
		return 0
	}

	// This is a rough estimation. In a real-world scenario, you would have a defined
	// storage limit per server to calculate a percentage against.
	// For now, let's assume a 50GB limit for percentage calculation.
	const storageLimitBytes = 50 * 1024 * 1024 * 1024 // 50 GB
	if size > 0 && storageLimitBytes > 0 {
		return int((float64(size) / float64(storageLimitBytes)) * 100)
	}

	return 0
}
