package ios

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/limrun-inc/go-sdk/internal/applog"
	"github.com/limrun-inc/go-sdk/internal/direct"
	"github.com/limrun-inc/go-sdk/internal/proxy"

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
	ws *websocket.Conn
	// events is the app log's server-sent event body, when it is not a WebSocket.
	events         io.ReadCloser
	cancelEvents   context.CancelFunc
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
//
// App logs arrive over the instance's /app-logs/stream server-sent events, one
// line per batch; the stream ends, closing Lines without an error, when the
// instance ends it.
func (c *Client) StreamAppLog(ctx context.Context, bundleID string) (*LogStream, error) {
	life, cancel := context.WithCancel(context.Background())
	stop := context.AfterFunc(ctx, cancel)
	body, err := applog.Open(life, direct.Client{URL: c.apiURL, Token: c.token, HTTPClient: c.httpClient}, bundleID)
	if !stop() || err != nil {
		cancel()
		if err == nil {
			body.Close()
			err = ctx.Err()
		}
		return nil, err
	}
	s := &LogStream{
		events:       body,
		cancelEvents: cancel,
		linesCh:      make(chan []string, 16),
		errCh:        make(chan error, 1),
		done:         make(chan struct{}),
	}
	go s.readEvents()
	return s, nil
}

func (s *LogStream) readEvents() {
	defer close(s.linesCh)
	defer close(s.errCh)
	err := applog.Read(s.events, func(line string) error {
		select {
		case s.linesCh <- []string{line}:
			return nil
		case <-s.done:
			return context.Canceled
		}
	})
	if !s.closed.Swap(true) {
		if !errors.Is(err, applog.ErrEnded) {
			s.emitErr(err)
		}
		s.shutdown()
		s.cancelEvents()
		s.events.Close()
	}
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

	dialer := c.websocketDialer
	if dialer == nil {
		dialer = proxy.WebSocketDialer(wsURL)
	}
	conn, res, err := dialer.DialContext(ctx, wsURL, http.Header{})
	if err != nil {
		if res != nil && res.Body != nil {
			res.Body.Close()
		}
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
	defer close(s.errCh)

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
	if s.events != nil {
		s.cancelEvents()
		return s.events.Close()
	}

	// Best-effort terminate message before closing.
	term, err := json.Marshal(map[string]any{"type": s.terminateType, "id": s.subscriptionID})
	if err == nil {
		_ = s.ws.WriteMessage(websocket.TextMessage, term)
	}
	return s.ws.Close()
}
