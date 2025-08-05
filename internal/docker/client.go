package docker

import (
	"context"
	"encoding/json"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// Client wraps the official Docker client to provide specific functionalities.
type Client struct {
	cli *client.Client
}

// New creates a new Docker client wrapper.
func New() (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Client{cli: cli}, nil
}

// CreateContainer creates a new Docker container with the given configurations.
func (c *Client) CreateContainer(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, containerName string) (container.CreateResponse, error) {
	return c.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, containerName)
}

// StartContainer starts a container by its ID.
func (c *Client) StartContainer(ctx context.Context, id string) error {
	return c.cli.ContainerStart(ctx, id, container.StartOptions{})
}

// StopContainer stops a container by its ID with a timeout.
func (c *Client) StopContainer(ctx context.Context, id string) error {
	// Specify a 10-second timeout for graceful shutdown
	timeout := 10
	return c.cli.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout})
}

// RestartContainer restarts a container by its ID.
func (c *Client) RestartContainer(ctx context.Context, id string) error {
	return c.cli.ContainerRestart(ctx, id, container.StopOptions{})
}

// RemoveContainer deletes a container by its ID.
func (c *Client) RemoveContainer(ctx context.Context, id string) error {
	return c.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true, RemoveVolumes: true})
}

// GetContainerLogs returns a reader for the container's logs.
func (c *Client) GetContainerLogs(ctx context.Context, id string, follow bool) (io.ReadCloser, error) {
	return c.cli.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Timestamps: true,
		Tail:       "100", // Get last 100 lines
	})
}

// InspectContainer returns the JSON response from a container inspect.
func (c *Client) InspectContainer(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	return c.cli.ContainerInspect(ctx, containerID)
}

// GetContainerStats returns a reader for the container's resource usage stats.
func (c *Client) GetContainerStats(ctx context.Context, id string) (*container.StatsResponse, error) {
	stats, err := c.cli.ContainerStats(ctx, id, false) // false for not streaming
	if err != nil {
		return nil, err
	}
	defer stats.Body.Close()

	var statsJSON container.StatsResponse
	if err := json.NewDecoder(stats.Body).Decode(&statsJSON); err != nil {
		return nil, err
	}

	return &statsJSON, nil
}

// ListContainers lists all containers managed by this application.
func (c *Client) ListContainers(ctx context.Context) ([]types.Container, error) {
	// In a real app, you would filter by a specific label, e.g. "com.ender-deploy.managed=true"
	return c.cli.ContainerList(ctx, container.ListOptions{All: true})
}

// CalculateCPUPercent calculates the CPU usage percentage from Docker stats.
func CalculateCPUPercent(stats *container.StatsResponse) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) - float64(stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage) - float64(stats.PreCPUStats.SystemUsage)
	onlineCPUs := float64(stats.CPUStats.OnlineCPUs)
	if onlineCPUs == 0.0 {
		onlineCPUs = float64(len(stats.CPUStats.CPUUsage.PercpuUsage))
	}

	if systemDelta > 0.0 && cpuDelta > 0.0 {
		return (cpuDelta / systemDelta) * onlineCPUs * 100.0
	}
	return 0.0
}

// CalculateRAMPercent calculates the RAM usage percentage from Docker stats.
func CalculateRAMPercent(stats *container.StatsResponse) float64 {
	if stats.MemoryStats.Limit > 0 {
		return float64(stats.MemoryStats.Usage) / float64(stats.MemoryStats.Limit) * 100.0
	}
	return 0.0
}
