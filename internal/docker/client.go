package docker

import (
	"context"
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

// StopContainer stops a container by its ID.
func (c *Client) StopContainer(ctx context.Context, id string) error {
	return c.cli.ContainerStop(ctx, id, container.StopOptions{})
}

// RestartContainer restarts a container by its ID.
func (c *Client) RestartContainer(ctx context.Context, id string) error {
	return c.cli.ContainerRestart(ctx, id, container.RestartOptions{})
}

// RemoveContainer deletes a container by its ID.
func (c *Client) RemoveContainer(ctx context.Context, id string) error {
	return c.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
}

// GetContainerLogs returns a reader for the container's logs.
func (c *Client) GetContainerLogs(ctx context.Context, id string, follow bool) (io.ReadCloser, error) {
	return c.cli.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Timestamps: true,
	})
}

// GetContainerStats returns a reader for the container's resource usage stats.
func (c *Client) GetContainerStats(ctx context.Context, id string) (types.ContainerStats, error) {
	return c.cli.ContainerStats(ctx, id, false) // false for not streaming
}

// ListContainers lists all containers managed by this application.
func (c *Client) ListContainers(ctx context.Context) ([]types.Container, error) {
	// In a real app, you would filter by a specific label, e.g. "com.ender-deploy.managed=true"
	return c.cli.ContainerList(ctx, container.ListOptions{All: true})
}
