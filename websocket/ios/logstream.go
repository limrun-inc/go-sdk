package ios

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

// LogStream is a handle to a running log subscription (app logs or syslog). It
// uses a dedicated WebSocket connection separate from the main signaling
// connection and delivers batched log lines (typically every ~500ms).
//
// Consume lines from the Lines channel and watch the Err channel for a
// terminal error. Call Stop to unsubscribe and close the connection. A server
// error message or a dropped connection also terminates the stream. In all
// cases the Lines channel is closed when the stream ends, so ranging over it
// terminates.
type LogStream struct {
	ws             *websocket.Conn
	subscriptionID string
	terminateType  string

	linesCh chan []string
	errCh   chan error
	done    chan struct{}

	closeDone sync.Once
	closed    atomic.Bool
}

// logStreamMessage is a message from the log stream WebSocket.
type logStreamMessage struct {
	Type  string   `json:"type"`
	ID    string   `json:"id"`
	Lines []string `json:"lines"`
	Error string   `json:"error"`
}

// StreamAppLog streams logs for the given bundle ID. The provided context
// governs only the initial connection; use Stop to end the stream.
func (c *Client) StreamAppLog(ctx context.Context, bundleID string) (*LogStream, error) {
	return c.startLogStream(ctx, map[string]any{"type": "streamAppLog", "bundleId": bundleID}, "streamAppLogTerminate")
}

// StreamSyslog streams the device syslog. The provided context governs only
// the initial connection; use Stop to end the stream.
func (c *Client) StreamSyslog(ctx context.Context) (*LogStream, error) {
	return c.startLogStream(ctx, map[string]any{"type": "streamSyslog"}, "streamSyslogTerminate")
}

func (c *Client) startLogStream(ctx context.Context, subscribe map[string]any, terminateType string) (*LogStream, error) {
	wsURL, err := signalingURL(c.apiURL, c.token)
	if err != nil {
		return nil, err
	}

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, http.Header{})
	if err != nil {
		return nil, fmt.Errorf("websocket dial: %w", err)
	}

	id := newSessionID()
	subscribe["id"] = id

	s := &LogStream{
		ws:             conn,
		subscriptionID: id,
		terminateType:  terminateType,
		linesCh:        make(chan []string, 16),
		errCh:          make(chan error, 1),
		done:           make(chan struct{}),
	}

	data, err := json.Marshal(subscribe)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		conn.Close()
		return nil, fmt.Errorf("send subscribe: %w", err)
	}

	go s.readLoop()
	return s, nil
}

// Lines returns the channel of batched log lines. The channel is closed when
// the stream stops.
func (s *LogStream) Lines() <-chan []string {
	return s.linesCh
}

// Err returns a channel that receives at most one terminal error if the stream
// fails (other than via an explicit Stop).
func (s *LogStream) Err() <-chan error {
	return s.errCh
}

func (s *LogStream) readLoop() {
	defer close(s.linesCh)

	for {
		_, message, err := s.ws.ReadMessage()
		if err != nil {
			if !s.closed.Swap(true) {
				s.emitErr(err)
				s.shutdown()
				s.ws.Close()
			}
			return
		}

		var msg logStreamMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			// Ignore non-JSON messages.
			continue
		}
		if msg.Error != "" {
			// A server error message is terminal for the subscription: emit it
			// and shut down so Lines() closes and consumers stop blocking. Skip
			// emitting if the stream was already stopped explicitly — Err() is
			// only for failures other than an explicit Stop().
			if !s.closed.Swap(true) {
				s.emitErr(errors.New(msg.Error))
				s.shutdown()
				s.ws.Close()
			}
			return
		}
		if len(msg.Lines) > 0 {
			select {
			case s.linesCh <- msg.Lines:
			case <-s.done:
				return
			}
		}
	}
}

func (s *LogStream) emitErr(err error) {
	select {
	case s.errCh <- err:
	default:
	}
}

func (s *LogStream) shutdown() {
	s.closeDone.Do(func() { close(s.done) })
}

// Stop unsubscribes and closes the dedicated WebSocket connection. It is safe
// to call multiple times.
func (s *LogStream) Stop() error {
	if s.closed.Swap(true) {
		return nil
	}
	s.shutdown()

	// Best-effort terminate message before closing.
	term, err := json.Marshal(map[string]any{"type": s.terminateType, "id": s.subscriptionID})
	if err == nil {
		_ = s.ws.WriteMessage(websocket.TextMessage, term)
	}
	return s.ws.Close()
}
