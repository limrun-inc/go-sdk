package ios

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/limrun-inc/go-sdk/tunnel"
)

type TunnelOptions = tunnel.DestinationOptions
type TunnelStatus = tunnel.DestinationStatus
type ReverseTunnelOptions struct {
	RemotePort, LocalPort int
	LocalHost             string
}

func DeriveReverseTunnelURL(apiURL string, remotePort int) (string, error) {
	u, e := url.Parse(apiURL)
	if e != nil {
		return "", e
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported apiUrl protocol for reverse tunnel: %s", u.Scheme)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/reverse-tunnel"
	u.RawPath = ""
	u.RawQuery = url.Values{"remotePort": []string{strconv.Itoa(remotePort)}}.Encode()
	u.Fragment = ""
	return u.String(), nil
}
func (c *Client) StartReverseTunnel(ctx context.Context, opts ReverseTunnelOptions) (*tunnel.ReverseTCP, error) {
	if e := tunnel.AssertPort(opts.RemotePort, "remotePort", 57090, 57099); e != nil {
		return nil, e
	}
	if opts.LocalPort == 0 {
		opts.LocalPort = opts.RemotePort
	}
	remote, e := DeriveReverseTunnelURL(c.apiURL, opts.RemotePort)
	if e != nil {
		return nil, e
	}
	return tunnel.StartReverseTCP(ctx, remote, c.token, tunnel.ReverseTCPOptions{LocalHost: opts.LocalHost, LocalPort: opts.LocalPort})
}
func (c *Client) StartTunnel(ctx context.Context, opts TunnelOptions) (*tunnel.Destination, error) {
	remote, e := tunnel.DestinationURL(c.apiURL)
	if e != nil {
		return nil, e
	}
	if opts.Inspection == nil {
		opts.Inspection = &tunnel.InspectionConfig{Enabled: true}
	}
	return tunnel.StartDestinationTCP(ctx, remote, c.token, opts)
}
func (c *Client) GetTunnelStatus(ctx context.Context) (*TunnelStatus, error) {
	return tunnel.GetDestinationStatus(ctx, c.apiURL, c.token, c.httpClient)
}
func (c *Client) StopTunnel(ctx context.Context, id string) error {
	return tunnel.StopDestination(ctx, c.apiURL, c.token, id, c.httpClient)
}

type HTTPProxyOptions struct {
	LocalPort     int
	RemoteBaseURL string
}
type ForwardHTTPProxyOptions struct {
	MatchPort     int
	RemoteBaseURL string
}

func (c *Client) StartHTTPProxy(opts HTTPProxyOptions) (int, error) {
	if e := tunnel.AssertPort(opts.LocalPort, "localPort", 1, 65535); e != nil {
		return 0, e
	}
	proxy, e := tunnel.StartHTTPProxy(tunnel.HTTPProxyOptions{LocalPort: opts.LocalPort, RemoteBaseURL: opts.RemoteBaseURL, Headers: http.Header{"Authorization": []string{"Bearer " + c.token}}})
	if e != nil {
		return 0, e
	}
	c.cleanupsMu.Lock()
	if c.closed.Load() {
		c.cleanupsMu.Unlock()
		proxy.Close()
		return 0, net.ErrClosed
	}
	c.cleanups = append(c.cleanups, func() { proxy.Close() })
	c.cleanupsMu.Unlock()
	return proxy.Port, nil
}
func (c *Client) StartForwardHTTPProxy(opts ForwardHTTPProxyOptions) (int, error) {
	if e := tunnel.AssertPort(opts.MatchPort, "matchPort", 1, 65535); e != nil {
		return 0, e
	}
	proxy, e := tunnel.StartForwardHTTPProxy(tunnel.ForwardHTTPProxyOptions{MatchPort: opts.MatchPort, RemoteBaseURL: opts.RemoteBaseURL, Headers: http.Header{"Authorization": []string{"Bearer " + c.token}}})
	if e != nil {
		return 0, e
	}
	c.cleanupsMu.Lock()
	if c.closed.Load() {
		c.cleanupsMu.Unlock()
		proxy.Close()
		return 0, net.ErrClosed
	}
	c.cleanups = append(c.cleanups, func() { proxy.Close() })
	c.cleanupsMu.Unlock()
	return proxy.Port, nil
}
