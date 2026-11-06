package build

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/limrun-inc/go-sdk/tunnel"
)

const DefaultRBETunnelPort = 8980

type RBETunnelOptions struct {
	Host string
	Port *int
}

func DeriveRBETunnelURL(apiURL string) (string, error) {
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
		return "", fmt.Errorf("unsupported apiUrl protocol for rbe tunnel: %s", u.Scheme)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/rbe/tunnel"
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}
func (c *XcodeClient) StartRbeTunnel(ctx context.Context, options ...RBETunnelOptions) (*tunnel.TCP, error) {
	var opts RBETunnelOptions
	if len(options) > 0 {
		opts = options[0]
	}
	port := DefaultRBETunnelPort
	if opts.Port != nil {
		port = *opts.Port
	}
	remote, e := DeriveRBETunnelURL(c.http.URL)
	if e != nil {
		return nil, e
	}
	return tunnel.StartTCP(ctx, remote, c.http.Token, opts.Host, port, tunnel.TCPOptions{Mode: "multiplexed"})
}
