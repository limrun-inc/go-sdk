package tunnel

import (
	"context"
	"net"
	"net/url"
	"strconv"
	"sync"
)

type MultiplexedOption func(*Multiplexed)

func MultiplexedWithLocalPort(port int) MultiplexedOption {
	return func(t *Multiplexed) { t.LocalPort = &port }
}

type Multiplexed struct {
	RemoteURL *url.URL
	LocalPort *int
	Token     string
	listener  net.Listener
	mu        sync.Mutex
	tcp       *TCP
	closed    bool
}

func NewMultiplexed(remoteURL *url.URL, remotePort int, token string, opts ...MultiplexedOption) (*Multiplexed, error) {
	u := *remoteURL
	q := u.Query()
	q.Set("port", strconv.Itoa(remotePort))
	u.RawQuery = q.Encode()
	t := &Multiplexed{RemoteURL: &u, Token: token}
	for _, f := range opts {
		f(t)
	}
	port := 0
	if t.LocalPort != nil {
		port = *t.LocalPort
	}
	if err := AssertPort(port, "localPort", 0, 65535); err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	t.listener = listener
	return t, nil
}
func (t *Multiplexed) Addr() string { return t.listener.Addr().String() }
func (t *Multiplexed) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return net.ErrClosed
	}
	if t.tcp != nil {
		return nil
	}
	tcp, err := startTCPListener(context.Background(), t.listener, t.RemoteURL.String(), t.Token, TCPOptions{Mode: "multiplexed"})
	t.tcp = tcp
	return err
}
func (t *Multiplexed) Close() error {
	t.mu.Lock()
	t.closed = true
	tcp := t.tcp
	t.mu.Unlock()
	if tcp != nil {
		return tcp.Close()
	}
	return t.listener.Close()
}
func (t *Multiplexed) GetConnectionState() ConnectionState {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.tcp == nil {
		return Disconnected
	}
	return t.tcp.GetConnectionState()
}
func (t *Multiplexed) OnConnectionStateChange(f func(ConnectionState)) func() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.tcp == nil {
		return func() {}
	}
	return t.tcp.OnConnectionStateChange(f)
}
