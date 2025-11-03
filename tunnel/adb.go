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
	return func(t *ADB) {
		t.ADBPath = p
	}
}

type Option func(*ADB)

// NewADB returns a new ADB that will listen on an available port and converts ADB traffic into WebSocket.
func NewADB(remoteURL, token string, opts ...Option) (*ADB, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, fmt.Errorf("creating a tcp listener failed: %w", err)
	}
	t := &ADB{
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

// ADB connects to a remote WebSocket endpoint and forwards ADB packets from and to the address it listens on locally.
type ADB struct {
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
// Call Close() method of the returned ADB to make sure it's properly cleaned up.
func (t *ADB) Start() error {
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

func (t *ADB) Addr() string {
	return fmt.Sprintf("127.0.0.1:%d", t.listener.Addr().(*net.TCPAddr).Port)
}

// Close closes the underlying ADB listener.
func (t *ADB) Close() {
	if t.cancel != nil {
		t.cancel(nil)
	}
}

// startTunnel starts the local ADB server to forward to WebSocket.
// Blocks until connection is closed.
// Cancel the context or call Close() when you'd like to stop this tunnel.
//
// You can optionally provide ready channel so that tunnel sends "true" when it's ready to accept connections,
// e.g. you can call "adb connect" after that message.
func (t *ADB) startTunnel() error {
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
