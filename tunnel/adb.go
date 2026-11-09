package tunnel

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"sync"
)

type Option func(*ADB)

func WithADBPath(path string) Option                  { return func(t *ADB) { t.ADBPath = path } }
func WithADBConnectionOptions(opts TCPOptions) Option { return func(t *ADB) { t.options = opts } }

type ADB struct {
	RemoteURL, Token, ADBPath string
	listener                  net.Listener
	mu                        sync.Mutex
	tcp                       *TCP
	options                   TCPOptions
	closed                    bool
}

func NewADB(remoteURL, token string, opts ...Option) (*ADB, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	t := &ADB{RemoteURL: remoteURL, Token: token, ADBPath: "adb", listener: listener}
	for _, f := range opts {
		f(t)
	}
	return t, nil
}
func (t *ADB) Addr() string { return t.listener.Addr().String() }
func (t *ADB) Start() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return net.ErrClosed
	}
	if t.tcp != nil {
		t.mu.Unlock()
		return nil
	}
	tcp, err := startTCPListener(context.Background(), t.listener, t.RemoteURL, t.Token, t.options)
	t.tcp = tcp
	t.mu.Unlock()
	if err != nil {
		return err
	}
	out, err := exec.Command(t.ADBPath, "connect", t.Addr()).CombinedOutput()
	if err != nil {
		t.Close()
		return fmt.Errorf("failed to connect adb: %w %s", err, out)
	}
	return nil
}
func (t *ADB) Close() {
	t.mu.Lock()
	t.closed = true
	tcp := t.tcp
	t.mu.Unlock()
	if tcp != nil {
		tcp.Close()
	} else {
		t.listener.Close()
	}
}
func (t *ADB) GetConnectionState() ConnectionState {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.tcp == nil {
		return Disconnected
	}
	return t.tcp.GetConnectionState()
}
func (t *ADB) OnConnectionStateChange(f func(ConnectionState)) func() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.tcp == nil {
		return func() {}
	}
	return t.tcp.OnConnectionStateChange(f)
}
