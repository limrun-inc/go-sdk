package android

import (
	"context"
	"errors"

	"github.com/limrun-inc/go-sdk/tunnel"
)

type TunnelOptions = tunnel.DestinationOptions
type TunnelStatus = tunnel.DestinationStatus

func (c *Client) StartTunnel(ctx context.Context, opts TunnelOptions) (*tunnel.Destination, error) {
	if c.adbURL == "" {
		return nil, errors.New("adbUrl is required to start a destination tunnel")
	}
	remote, e := tunnel.DestinationURL(c.adbURL)
	if e != nil {
		return nil, e
	}
	if opts.Inspection == nil {
		opts.Inspection = &tunnel.InspectionConfig{Enabled: true}
	}
	if opts.WebSocketDialer == nil {
		opts.WebSocketDialer = c.connectionOptions.Dialer
	}
	return tunnel.StartDestinationTCP(ctx, remote, c.http.Token, opts)
}
func (c *Client) GetTunnelStatus(ctx context.Context) (*TunnelStatus, error) {
	if c.adbURL == "" {
		return nil, errors.New("adbUrl is required to get tunnel status")
	}
	return tunnel.GetDestinationStatus(ctx, c.adbURL, c.http.Token, c.http.HTTPClient)
}
func (c *Client) StopTunnel(ctx context.Context, id string) error {
	if c.adbURL == "" {
		return errors.New("adbUrl is required to stop a tunnel")
	}
	return tunnel.StopDestination(ctx, c.adbURL, c.http.Token, id, c.http.HTTPClient)
}
