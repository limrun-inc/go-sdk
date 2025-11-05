package tunnel

import (
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
	"time"

	"github.com/gorilla/websocket"
)

/*
 * Multiplexed establishes a single WebSocket connection to the remote and allows many TCP connections to be tunneled
 * through that same WebSocket connection. For cases, like HTTP servers, every request is a new TCP connection so this
 * implementation saves the overhead of WebSocket dialing for every request. But if you create a single TCP connection
 * anyway, like Android ADB does, it does not provide much benefit.
 *
 * Note that it requires the server side to support connection ID prefixing so it can track of connection pairs.
 * See the protocol below.
 */

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

const (
	connIDSize = 4 // Size of connection ID in bytes
)

// encodeMessage creates a WebSocket message by prefixing data with connection ID.
// Format: [4 bytes: connID][data]
func encodeMessage(connID uint32, data []byte) []byte {
	msg := make([]byte, connIDSize+len(data))
	binary.BigEndian.PutUint32(msg[:connIDSize], connID)
	copy(msg[connIDSize:], data)
	return msg
}

// decodeMessage extracts the connection ID and data from a WebSocket message.
// Format: [4 bytes: connID][data]
// Returns an error if the message is too short.
func decodeMessage(message []byte) (connID uint32, data []byte, err error) {
	if len(message) < connIDSize {
		return 0, nil, fmt.Errorf("message too short: %d bytes, expected at least %d", len(message), connIDSize)
	}
	connID = binary.BigEndian.Uint32(message[:connIDSize])
	data = message[connIDSize:]
	return connID, data, nil
}

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
	addr, ok := t.listener.Addr().(*net.TCPAddr)
	if !ok {
		return t.listener.Addr().String()
	}
	return fmt.Sprintf("127.0.0.1:%d", addr.Port)
}

// Close closes the underlying listener and WebSocket connection.
func (t *Multiplexed) Close() error {
	var errs []error

	if t.listener != nil {
		if err := t.listener.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing listener: %w", err))
		}
	}

	if t.ws != nil {
		if err := t.ws.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing websocket: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

// startTunnel starts the local TCP server and establishes the single persistent
// WebSocket connection to the remote server. For every TCP connection, a new
// go routine is started to handle it using the shared WebSocket connection.
//
// Blocks until Close() is called.
func (t *Multiplexed) startTunnel() error {
	ws, _, err := websocket.DefaultDialer.Dial(t.RemoteURL.String(), http.Header{
		"Authorization": []string{fmt.Sprintf("Bearer %s", t.Token)},
	})
	if err != nil {
		return fmt.Errorf("failed to dial remote websocket server: %w", err)
	}
	t.ws = ws

	// Start WebSocket reader to demultiplex incoming messages
	go t.readFromWebSocket()

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
					log.Printf("websocket ping failed: %v", err)
				}
			}
		}
	}()

	for {
		tcpConn, err := t.listener.Accept()
		if err != nil {
			return fmt.Errorf("failed to accept connection: %w", err)
		}

		// Handle each connection in its own goroutine
		go t.handleConnection(tcpConn)
	}
}

// readFromWebSocket reads from the WebSocket and forwards messages to the correct TCP connection.
// Message format: [4 bytes: connection ID][data]
// Empty data indicates connection close signal.
func (t *Multiplexed) readFromWebSocket() {
	for {
		_, message, err := t.ws.ReadMessage()
		if err != nil {
			log.Printf("websocket read error: %v", err)
			return
		}

		connID, data, err := decodeMessage(message)
		if err != nil {
			log.Printf("failed to decode message: %v", err)
			continue
		}

		conn, ok := t.connections.Load(connID)
		if !ok {
			// When connection is closed, both sides send empty data. The server
			// may send it after we closed and cleaned up the connection so we ignore
			// the message if we're closed and it's empty.
			if len(data) > 0 {
				// Only log if there was actual data we couldn't deliver
				log.Printf("received message for unknown connection ID: %d", connID)
			}
			continue
		}

		tcpConn, ok := conn.(net.Conn)
		if !ok {
			log.Printf("invalid connection type for ID %d", connID)
			t.connections.Delete(connID)
			continue
		}

		// Empty data means close signal from server
		if len(data) == 0 {
			_ = tcpConn.Close()
			t.connections.Delete(connID)
			continue
		}
		if _, err := tcpConn.Write(data); err != nil {
			log.Printf("failed to write to tcp connection %d: %v", connID, err)
			_ = tcpConn.Close()
			t.connections.Delete(connID)
		}
	}
}

// handleConnection handles a single TCP connection by multiplexing it over the shared WebSocket.
// Message format: [4 bytes: connection ID][data]
func (t *Multiplexed) handleConnection(tcpConn net.Conn) {
	connID := t.nextConnID.Add(1)
	t.connections.Store(connID, tcpConn)

	defer func() {
		_ = tcpConn.Close()
		t.connections.Delete(connID)

		// Send close signal: [4 bytes: connID][empty data]
		closeMsg := encodeMessage(connID, nil)
		t.wsMu.Lock()
		defer t.wsMu.Unlock()
		_ = t.ws.WriteMessage(websocket.BinaryMessage, closeMsg)
	}()
	buffer := make([]byte, 32*1024) // 32KB data buffer
	for {
		n, err := tcpConn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				// io.EOF is expected when the connection is closed by the client.
				return
			}
			log.Printf("tcp->ws: error reading from connection %d: %v", connID, err)
			continue
		}
		if n == 0 {
			continue
		}
		t.wsMu.Lock()
		err = t.ws.WriteMessage(websocket.BinaryMessage, encodeMessage(connID, buffer[:n]))
		t.wsMu.Unlock()
		if err != nil {
			log.Printf("failed to write to websocket: %v", err)
			continue
		}
	}
}
