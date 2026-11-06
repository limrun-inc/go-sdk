// Package stream provides resumable server-sent events for direct instance APIs.
package stream

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/limrun-inc/go-sdk/internal/direct"
	"github.com/limrun-inc/go-sdk/internal/proxy"
)

type Event struct{ ID, Type, Data string }
type HTTPError = direct.HTTPError

type callbackError struct{ error }

func (e callbackError) Unwrap() error { return e.error }

var ErrClosed = errors.New("server closed event stream (HTTP 204)")
var ErrLost = errors.New("event stream kept failing for 300s without delivering events")

type Options struct {
	FailOnClientError                  bool
	HTTPClient                         *http.Client
	URL, Token, LastEventID            string
	OnEvent                            func(Event) error
	OnDeadSince                        func(time.Time)
	HealthyConnection, GivingUp, Retry time.Duration
}

// Read consumes one SSE connection, including multi-line data and comment heartbeats.
func Read(r io.Reader, onEvent func(Event) error, onActivity func(), onRetry func(time.Duration)) error {
	reader := bufio.NewScanner(r)
	reader.Split(splitSSELine)
	reader.Buffer(make([]byte, 4096), 64*1024*1024)
	var event Event
	var data []string
	first := true
	dispatch := func() error {
		if len(data) == 0 {
			event.Type = ""
			return nil
		}
		event.Data = strings.Join(data, "\n")
		data = nil
		if event.Type == "" {
			event.Type = "message"
		}
		err := onEvent(event)
		if err != nil {
			err = callbackError{err}
		}
		event.Type = ""
		return err
	}
	for reader.Scan() {
		line := reader.Text()
		if first {
			line = strings.TrimPrefix(line, "\ufeff")
			first = false
		}
		if line == "" {
			if e := dispatch(); e != nil {
				return e
			}
		} else if strings.HasPrefix(line, ":") {
			onActivity()
		} else {
			key, value, found := strings.Cut(line, ":")
			if !found {
				value = ""
			}
			value = strings.TrimPrefix(value, " ")
			switch key {
			case "event":
				event.Type = value
			case "data":
				data = append(data, value)
				onActivity()
			case "id":
				if !strings.ContainsRune(value, '\x00') {
					event.ID = value
				}
			case "retry":
				if ms, e := strconv.ParseInt(value, 10, 64); e == nil && ms >= 0 {
					onRetry(time.Duration(ms) * time.Millisecond)
				}
			}
		}
	}
	if err := reader.Err(); err != nil {
		return err
	}
	return io.EOF
}

// SSE accepts LF, CRLF and bare CR, including delimiters split across reads.
func splitSSELine(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b == '\n' {
			return i + 1, data[:i], nil
		}
		if b == '\r' {
			if i+1 == len(data) && !atEOF {
				return 0, nil, nil
			}
			end := i + 1
			if end < len(data) && data[end] == '\n' {
				end++
			}
			return end, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}
func Run(ctx context.Context, opts Options) error {
	client := opts.HTTPClient
	if client == nil {
		client = proxy.Client()
	}
	healthy := opts.HealthyConnection
	if healthy == 0 {
		healthy = 15 * time.Second
	}
	giveup := opts.GivingUp
	if giveup == 0 {
		giveup = 5 * time.Minute
	}
	retry := opts.Retry
	if retry == 0 {
		retry = time.Second
	}
	id := opts.LastEventID
	var deadSince time.Time
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		start := time.Now()
		proof := false
		clean := false
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.URL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Authorization", "Bearer "+opts.Token)
		if id != "" {
			req.Header.Set("Last-Event-ID", id)
		}
		res, err := client.Do(req)
		if err == nil {
			if res.StatusCode == 204 {
				res.Body.Close()
				return ErrClosed
			}
			if res.StatusCode < 200 || res.StatusCode >= 300 {
				body, _ := io.ReadAll(res.Body)
				err = direct.ResponseError("event stream", res, string(body))
				if opts.FailOnClientError && res.StatusCode >= 400 && res.StatusCode < 500 && res.StatusCode != 408 && res.StatusCode != 429 {
					res.Body.Close()
					return err
				}
			} else {
				clean = true
				err = Read(res.Body, func(e Event) error {
					id = e.ID
					proof = true
					deadSince = time.Time{}
					if opts.OnDeadSince != nil {
						opts.OnDeadSince(deadSince)
					}
					return opts.OnEvent(e)
				}, func() { proof = true }, func(d time.Duration) { retry = d })
			}
			res.Body.Close()
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var callback callbackError
		if errors.As(err, &callback) {
			return callback.error
		}

		now := time.Now()
		if proof || (clean && now.Sub(start) >= healthy) {
			deadSince = time.Time{}
		} else if deadSince.IsZero() {
			deadSince = now
		} else if now.Sub(deadSince) >= giveup {
			return fmt.Errorf("%w: %v", ErrLost, err)
		}
		if opts.OnDeadSince != nil {
			opts.OnDeadSince(deadSince)
		}
		before := time.Now()
		timer := time.NewTimer(retry)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if time.Since(before)-retry > 30*time.Second {
			deadSince = time.Time{}
		}
	}
}
