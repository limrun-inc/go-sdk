package tunnel

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

func MultiplexedWithLocalPort(port int) MultiplexedOption {
	return func(r *Multiplexed) {
		r.LocalPort = &port
	}
}

type MultiplexedOption func(*Multiplexed)

// NewMultiplexed returns a new Multiplexed tunnel.
func NewMultiplexed(remoteURL *url.URL, remotePort int, token string, opts ...MultiplexedOption) (*Multiplexed, error) {
	q := remoteURL.Query()
	q.Set("port", strconv.Itoa(remotePort))
	u := remoteURL.JoinPath()
	u.RawQuery = q.Encode()
	t := &Multiplexed{
		RemoteURL: u,
		Token:     token,
	}
	for _, f := range opts {
		f(t)
	}
	localPort := ":0"
	if t.LocalPort != nil {
		localPort = fmt.Sprintf(":%d", *t.LocalPort)
	}
	listener, err := net.Listen("tcp", localPort)
	if err != nil {
		return nil, fmt.Errorf("creating a tcp listener failed: %w", err)
	}
	t.listener = listener
	return t, nil
}

// Protocol Format:
// All WebSocket messages use this binary format:
//
//	[4 bytes: connection ID (big-endian uint32)][data bytes]
//
// Connection Lifecycle:
//   - First message with a new connection ID implicitly opens the connection
//   - Subsequent messages with data are forwarded to/from the TCP connection
//   - Message with empty data (only 4-byte header) signals connection close
//
// This allows multiple TCP connections to share a single WebSocket connection
// with only 4 bytes of overhead per message.

// Multiplexed connects to a remote WebSocket endpoint once and handles all TCP connections through that single WebSocket
// connection.
//
// It prefixes the data with connection ID so it requires server-side to support it.
type Multiplexed struct {
	// RemoteURL is the URL of the remote server.
	RemoteURL *url.URL

	// LocalPort for TCP server to listen on.
	// If not given, an empty port requested from the operating system.
	LocalPort *int

	// Token is used to authenticate the user. The server may still reject it
	// if it's marked as revoked.
	Token string

	listener net.Listener
	cancel   context.CancelCauseFunc

	// Multiplexing state
	ws          *websocket.Conn
	wsMu        sync.Mutex
	nextConnID  atomic.Uint32
	connections sync.Map // map[uint32]net.Conn
}

// Start establishes a WebSocket connection and starts listening on TCP connections.
//
// It is non-blocking and continues to run in the background.
// Call Close() method of the returned Multiplexed to make sure it's properly cleaned up.
func (t *Multiplexed) Start() error {
	if t.listener == nil {
		return fmt.Errorf("tunnel listener is not initialized")
	}
	go func() {
		if err := t.startTunnel(); err != nil {
			log.Printf("failed to start TCP tunnel: %s", err)
		}
	}()
	return nil
}

func (t *Multiplexed) Addr() string {
	return fmt.Sprintf("127.0.0.1:%d", t.listener.Addr().(*net.TCPAddr).Port)
}

// Close closes the underlying ADB listener and WebSocket connection.
func (t *Multiplexed) Close() {
	if t.cancel != nil {
		t.cancel(nil)
	}
	if t.ws != nil {
		_ = t.ws.Close()
	}
}

// startTunnel starts the local ADB server to forward to WebSocket.
// Blocks until connection is closed.
// Call Close() when you'd like to stop this tunnel.
func (t *Multiplexed) startTunnel() error {
	tCtx, cancel := context.WithCancelCause(context.Background())
	t.cancel = cancel
	defer cancel(nil)

	defer func() {
		_ = t.listener.Close()
	}()

	// Establish persistent WebSocket connection upfront
	ws, _, err := websocket.DefaultDialer.Dial(t.RemoteURL.String(), http.Header{
		"Authorization": []string{fmt.Sprintf("Bearer %s", t.Token)},
	})
	if err != nil {
		return fmt.Errorf("failed to dial remote websocket server: %w", err)
	}
	t.ws = ws
	defer func() {
		_ = ws.Close()
	}()

	// Start WebSocket reader to demultiplex incoming messages
	go t.readFromWebSocket(tCtx)

	// Accept TCP connections in a loop
	for {
		select {
		case <-tCtx.Done():
			return context.Cause(tCtx)
		default:
		}

		tcpConn, err := t.listener.Accept()
		if err != nil {
			select {
			case <-tCtx.Done():
				// Listener was closed intentionally
				return context.Cause(tCtx)
			default:
				return fmt.Errorf("failed to accept connection: %w", err)
			}
		}

		// Handle each connection in its own goroutine
		go t.handleConnection(tCtx, tcpConn)
	}
}

// readFromWebSocket reads from the WebSocket and demultiplexes messages to the correct TCP connection.
// Message format: [4 bytes: connection ID][data]
// Empty data indicates connection close signal.
func (t *Multiplexed) readFromWebSocket(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, message, err := t.ws.ReadMessage()
		if err != nil {
			log.Printf("websocket read error: %v", err)
			if t.cancel != nil {
				t.cancel(fmt.Errorf("websocket read error: %w", err))
			}
			return
		}

		if len(message) < 4 {
			log.Printf("received message too short: %d bytes", len(message))
			continue
		}

		// Extract connection ID (first 4 bytes, big-endian)
		connID := binary.BigEndian.Uint32(message[:4])
		data := message[4:]

		// Look up the TCP connection
		conn, ok := t.connections.Load(connID)
		if !ok {
			// This is normal when both client and server close simultaneously
			// The connection was already cleaned up on our side
			if len(data) > 0 {
				// Only log if there was actual data we couldn't deliver
				log.Printf("received message for unknown connection ID: %d", connID)
			}
			continue
		}
		tcpConn := conn.(net.Conn)

		// Empty data means close signal from server
		if len(data) == 0 {
			log.Printf("ws->tcp: received close signal for connection %d", connID)
			_ = tcpConn.Close()
			t.connections.Delete(connID)
			continue
		}

		// Write data to TCP connection
		_, err = tcpConn.Write(data)
		if err != nil {
			log.Printf("failed to write to tcp connection %d: %v", connID, err)
			_ = tcpConn.Close()
			t.connections.Delete(connID)
		}
	}
}

// handleConnection handles a single TCP connection by multiplexing it over the shared WebSocket.
// Message format: [4 bytes: connection ID][data]
func (t *Multiplexed) handleConnection(ctx context.Context, tcpConn net.Conn) {
	// Assign unique connection ID
	connID := t.nextConnID.Add(1)

	// Register connection
	t.connections.Store(connID, tcpConn)

	defer func() {
		_ = tcpConn.Close()
		t.connections.Delete(connID)

		// Send close signal: [4 bytes: connID][empty data]
		closeMsg := make([]byte, 4)
		binary.BigEndian.PutUint32(closeMsg, connID)
		t.wsMu.Lock()
		_ = t.ws.WriteMessage(websocket.BinaryMessage, closeMsg)
		t.wsMu.Unlock()
	}()

	// Read from TCP and send to WebSocket with connection ID prefix
	buffer := make([]byte, 32*1024) // 32KB buffer
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, err := tcpConn.Read(buffer)
		if err != nil {
			if err != io.EOF {
				log.Printf("tcp->ws: error reading from connection %d: %v", connID, err)
			}
			return
		}

		if n > 0 {
			// Build message: [4 bytes: connID][data]
			msg := make([]byte, 4+n)
			binary.BigEndian.PutUint32(msg[:4], connID)
			copy(msg[4:], buffer[:n])

			// Send to WebSocket (with mutex to prevent concurrent writes)
			t.wsMu.Lock()
			err = t.ws.WriteMessage(websocket.BinaryMessage, msg)
			t.wsMu.Unlock()

			if err != nil {
				log.Printf("failed to write to websocket: %v", err)
				if t.cancel != nil {
					t.cancel(fmt.Errorf("websocket write error: %w", err))
				}
				return
			}
		}
	}
}
