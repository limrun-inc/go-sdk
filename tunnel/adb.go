package tunnel

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os/exec"
	"time"

	"github.com/gorilla/websocket"
)

// WithADBPath lets you supply a custom path to the adb executable if it's not in PATH.
func WithADBPath(p string) Option {
	return func(t *Tunnel) {
		t.ADBPath = p
	}
}

type Option func(*Tunnel)

// New returns a new Tunnel that will listen on an available port and converts Tunnel traffic into WebSocket.
func New(remoteURL, token string, opts ...Option) (*Tunnel, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, fmt.Errorf("creating a tcp listener failed: %w", err)
	}
	t := &Tunnel{
		RemoteURL: remoteURL,
		Token:     token,
		ADBPath:   "adb",
		listener:  listener,
	}
	for _, f := range opts {
		f(t)
	}
	return t, nil
}

// Tunnel connects to a remote WebSocket endpoint and forwards Tunnel packets from and to the address it listens on locally.
type Tunnel struct {
	// RemoteURL is the URL of the remote server.
	RemoteURL string

	// Token is used to authenticate the user. The server may still reject it
	// if it's marked as revoked.
	Token string

	// ADBPath is the path to adb executable. Defaults to just "adb".
	ADBPath string

	listener net.Listener
	cancel   context.CancelCauseFunc
}

// Start starts a tunnel to the Android instance through the given URL and notifies the local ADB to recognize
// it.
// It is non-blocking and continues to run in the background.
// Call Close() method of the returned Tunnel to make sure it's properly cleaned up.
func (t *Tunnel) Start() error {
	go func() {
		if err := t.startTunnel(); err != nil {
			log.Printf("failed to start TCP tunnel: %s", err)
		}
	}()
	out, err := exec.CommandContext(context.Background(), t.ADBPath, "connect", t.Addr()).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to connect adb: %w %s", err, string(out))
	}
	return nil
}

func (t *Tunnel) Addr() string {
	return fmt.Sprintf("127.0.0.1:%d", t.listener.Addr().(*net.TCPAddr).Port)
}

// Close closes the underlying Tunnel listener.
func (t *Tunnel) Close() {
	if t.cancel != nil {
		t.cancel(nil)
	}
}

// startTunnel starts the local Tunnel server to forward to WebSocket.
// Blocks until connection is closed.
// Cancel the context or call Close() when you'd like to stop this tunnel.
//
// You can optionally provide ready channel so that tunnel sends "true" when it's ready to accept connections,
// e.g. you can call "adb connect" after that message.
func (t *Tunnel) startTunnel() error {
	tCtx, cancel := context.WithCancelCause(context.Background())
	t.cancel = cancel
	defer cancel(nil)

	defer func() {
		_ = t.listener.Close()
	}()

	tcpConn, err := t.listener.Accept()
	if err != nil {
		return fmt.Errorf("failed to accept connection: %w", err)
	}
	defer func() {
		_ = tcpConn.Close()
	}()

	ws, _, err := websocket.DefaultDialer.Dial(t.RemoteURL, http.Header{
		"Authorization": []string{fmt.Sprintf("Bearer %s", t.Token)},
	})
	if err != nil {
		return fmt.Errorf("failed to dial remote websocket server: %w", err)
	}
	defer func() {
		_ = ws.Close()
	}()

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-tCtx.Done():
				return
			case <-ticker.C:
				if err := ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
					cancel(fmt.Errorf("ping failed: %v", err))
					return
				}
			}
		}
	}()

	go func() {
		// 32Kb is default frame size.
		buffer := make([]byte, 32*1024)
		for {
			select {
			case <-tCtx.Done():
				return
			default:
			}

			n, err := tcpConn.Read(buffer)
			if err != nil {
				if err != io.EOF {
					cancel(fmt.Errorf("failed to read from tcp: %w", err))
				} else {
					log.Printf("tcp->ws: TCP connection closed by client")
				}
				return
			}

			if n > 0 {
				err = ws.WriteMessage(websocket.BinaryMessage, buffer[:n])
				if err != nil {
					cancel(fmt.Errorf("failed to write to websocket: %w", err))
					return
				}
			}
		}
	}()

	go func() {
		for {
			select {
			case <-tCtx.Done():
				return
			default:
			}
			_, message, err := ws.ReadMessage()
			if err != nil {
				cancel(fmt.Errorf("websocket read error: %w", err))
				return
			}
			if len(message) > 0 {
				_, err = tcpConn.Write(message)
				if err != nil {
					cancel(fmt.Errorf("failed to write to tcp: %w", err))
					return
				}
			}
		}
	}()
	<-tCtx.Done()
	return context.Cause(tCtx)
}
