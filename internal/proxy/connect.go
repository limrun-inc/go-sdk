package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Error struct{ Code, Message string }

func (e *Error) Error() string { return e.Message }

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
func (c *bufferedConn) CloseWrite() error {
	if c, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return c.CloseWrite()
	}
	return nil
}

// Connect opens a half-close-capable TCP stream, using CONNECT for both HTTP and HTTPS proxies.
func Connect(ctx context.Context, target string, endpoint *url.URL, dial func(context.Context, string, string) (net.Conn, error)) (net.Conn, error) {
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	if endpoint == nil {
		return dial(ctx, "tcp", target)
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return nil, &Error{"EPROXY", "unsupported proxy protocol " + endpoint.Scheme}
	}
	proxyPort := endpoint.Port()
	if proxyPort == "" {
		proxyPort = "80"
		if endpoint.Scheme == "https" {
			proxyPort = "443"
		}
	}
	conn, err := dial(ctx, "tcp", net.JoinHostPort(endpoint.Hostname(), proxyPort))
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			conn.Close()
		}
	}()
	rawConn := conn
	stop := context.AfterFunc(ctx, func() { rawConn.Close() })
	defer stop()
	if deadline, has := ctx.Deadline(); has {
		conn.SetDeadline(deadline)
	}
	if endpoint.Scheme == "https" {
		secure := tls.Client(conn, &tls.Config{ServerName: endpoint.Hostname(), NextProtos: []string{"http/1.1"}, MinVersion: tls.VersionTLS12})
		if err = secure.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		conn = secure
	}
	headers := "CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\nProxy-Connection: Keep-Alive\r\n"
	if endpoint.User != nil {
		password, _ := endpoint.User.Password()
		headers += "Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(endpoint.User.Username()+":"+password)) + "\r\n"
	}
	if _, err = conn.Write([]byte(headers + "\r\n")); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(conn)
	var raw strings.Builder
	for {
		line, e := reader.ReadString('\n')
		if e != nil {
			return nil, e
		}
		raw.WriteString(line)
		if raw.Len() > 64*1024 {
			return nil, &Error{"EPROXY", "proxy CONNECT response headers exceeded 65536 bytes"}
		}
		if line == "\r\n" {
			break
		}
	}
	response, err := http.ReadResponse(bufio.NewReader(strings.NewReader(raw.String())), &http.Request{Method: "CONNECT"})
	if err != nil {
		return nil, &Error{"EPROXY", "invalid proxy CONNECT response"}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		code := "EPROXYCONNECT"
		if response.StatusCode == 407 {
			code = "EPROXYAUTH"
		}
		return nil, &Error{code, fmt.Sprintf("proxy CONNECT failed with status %d", response.StatusCode)}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	stop()
	conn.SetDeadline(time.Time{})
	ok = true
	return &bufferedConn{conn, reader}, nil
}
