package tunnel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/limrun-inc/go-sdk/internal/proxy"

	"github.com/limrun-inc/go-sdk/internal/stream"
)

// HAREntry preserves standard HAR fields and server-provided extensions.
type HAREntry map[string]any
type InspectionGap struct {
	FromSequence, ToSequence int64
	Message                  string
}
type InspectionEvent struct {
	Type                 string
	Sequence             int64
	RequestID, Direction string
	Body                 []byte
	Complete             HAREntry
	Gap                  *InspectionGap
	Data                 map[string]any
}
type InspectionStreamOptions struct {
	OnEvent                                  func(InspectionEvent)
	OnError                                  func(error)
	HTTPClient                               *http.Client
	ReconnectInitialDelay, ReconnectMaxDelay time.Duration
}
type InspectionStream struct {
	cancel context.CancelFunc
	once   sync.Once
	done   chan struct{}
}

func (s *InspectionStream) Close()                { s.once.Do(s.cancel) }
func (s *InspectionStream) Done() <-chan struct{} { return s.done }
func safeError(f func(error), err error) {
	if f != nil {
		func() { defer func() { _ = recover() }(); f(err) }()
	}
}
func safeEvent(f func(InspectionEvent), event InspectionEvent, onError func(error)) {
	if f != nil {
		func() {
			defer func() {
				if p := recover(); p != nil {
					safeError(onError, fmt.Errorf("inspection callback panicked: %v", p))
				}
			}()
			f(event)
		}()
	}
}
func deriveEndpoint(base, path string, websocket bool) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http", "ws":
		if websocket {
			u.Scheme = "ws"
		} else {
			u.Scheme = "http"
		}
	case "https", "wss":
		if websocket {
			u.Scheme = "wss"
		} else {
			u.Scheme = "https"
		}
	default:
		return "", fmt.Errorf("unsupported apiUrl protocol: %s", u.Scheme)
	}
	encoded := strings.TrimRight(u.EscapedPath(), "/") + "/" + path
	u.Path, err = url.PathUnescape(encoded)
	if err != nil {
		return "", err
	}
	u.RawPath = encoded
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}
func DestinationURL(base string) (string, error) { return deriveEndpoint(base, "tunnel", true) }
func InspectionURL(remote, tunnelID, token string, afterSequence int64) (string, error) {
	if afterSequence < 0 || afterSequence > 9007199254740991 {
		return "", fmt.Errorf("afterSequence must be a safe non-negative integer")
	}
	endpoint, err := deriveEndpoint(remote, url.PathEscape(tunnelID)+"/inspection/events", false)
	if err != nil {
		return "", err
	}
	u, _ := url.Parse(endpoint)
	q := u.Query()
	if afterSequence > 0 {
		q.Set("after-sequence", strconv.FormatInt(afterSequence, 10))
	}
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
func StartInspectionStream(ctx context.Context, remote, tunnelID, token string, opts InspectionStreamOptions) (*InspectionStream, error) {
	endpoint, err := InspectionURL(remote, tunnelID, token, 0)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &InspectionStream{cancel: cancel, done: make(chan struct{})}
	client := opts.HTTPClient
	if client == nil {
		client = proxy.Client()
	}
	go func() {
		defer close(s.done)
		var last int64
		id := ""
		retry := time.Second
		emitGap := func(from, to int64) {
			safeEvent(opts.OnEvent, InspectionEvent{Type: "gap", Sequence: to, Gap: &InspectionGap{from, to, fmt.Sprintf("Inspection stream gap: missing sequences %d-%d", from, to)}}, opts.OnError)
		}
		for ctx.Err() == nil {
			req, e := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
			if e != nil {
				safeError(opts.OnError, e)
				return
			}
			req.Header.Set("Accept", "text/event-stream")
			if id != "" {
				req.Header.Set("Last-Event-ID", id)
			}
			res, e := client.Do(req)
			if e == nil {
				if res.StatusCode == 204 {
					res.Body.Close()
					return
				}
				if res.StatusCode < 200 || res.StatusCode >= 300 {
					body, _ := io.ReadAll(res.Body)
					e = fmt.Errorf("inspection SSE failed: %d %s", res.StatusCode, body)
				} else {
					e = stream.Read(res.Body, func(message stream.Event) error {
						id = message.ID
						event, err := DecodeInspectionSSEEvent(message.ID, message.Data)
						if err != nil {
							safeError(opts.OnError, err)
							return nil
						}
						if event.Sequence <= last {
							safeError(opts.OnError, protocolErrorf("inspection sequence %d is not newer than %d", event.Sequence, last))
							return nil
						}
						if event.Type == "gap" {
							if event.Gap.FromSequence != last+1 || event.Gap.ToSequence != event.Sequence || event.Gap.FromSequence > event.Gap.ToSequence {
								safeError(opts.OnError, protocolErrorf("invalid inspection gap range"))
								return nil
							}
							emitGap(event.Gap.FromSequence, event.Gap.ToSequence)
						} else {
							if event.Sequence > last+1 {
								emitGap(last+1, event.Sequence-1)
							}
							safeEvent(opts.OnEvent, event, opts.OnError)
						}
						last = event.Sequence
						return nil
					}, func() {}, func(d time.Duration) { retry = d })
				}
				res.Body.Close()
			}
			if ctx.Err() != nil {
				return
			}
			safeError(opts.OnError, fmt.Errorf("inspection SSE disconnected; reconnecting: %w", e))
			if !wait(ctx, retry) {
				return
			}
		}
	}()
	return s, nil
}
func readNumber(r map[string]json.RawMessage, key string) (float64, error) {
	var n float64
	b, ok := r[key]
	if !ok || string(b) == "null" || json.Unmarshal(b, &n) != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, protocolErrorf("%s must be a finite number", key)
	}
	return n, nil
}
func readSequence(r map[string]json.RawMessage, key string) (int64, error) {
	n, e := readInteger(r, key)
	if e != nil {
		return 0, e
	}
	if n < 0 || n > 9007199254740991 {
		return 0, protocolErrorf("inspection sequence must be a safe non-negative integer")
	}
	return n, nil
}
func DecodeInspectionSSEEvent(id, text string) (event InspectionEvent, err error) {
	if id == "" || id != "0" && strings.HasPrefix(id, "0") || strings.IndexFunc(id, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return event, protocolErrorf("inspection SSE event ID must be an unsigned integer")
	}
	event.Sequence, err = strconv.ParseInt(id, 10, 64)
	if err != nil || event.Sequence > 9007199254740991 {
		return event, protocolErrorf("inspection sequence must be a safe non-negative integer")
	}
	record, err := readRecord([]byte(text))
	if err != nil {
		return event, err
	}
	if _, ok := record["sequence"]; ok {
		n, e := readSequence(record, "sequence")
		if e != nil {
			return event, e
		}
		if n != event.Sequence {
			return event, protocolErrorf("inspection SSE event ID does not match envelope sequence")
		}
	}
	if event.Type, err = readString(record, "type"); err != nil {
		return event, err
	}
	data, err := readRecord(record["data"])
	if err != nil {
		return event, err
	}
	if event.Type == "body" || event.Type == "complete" || record["requestId"] != nil {
		if event.RequestID, err = readString(record, "requestId"); err != nil {
			return event, err
		}
		if event.RequestID == "" {
			return event, protocolErrorf("inspection request ID must not be empty")
		}
	}
	switch event.Type {
	case "body":
		if event.Direction, err = readString(data, "direction"); err != nil {
			return event, err
		}
		if event.Direction != "request" && event.Direction != "response" {
			return event, protocolErrorf("invalid inspection body direction")
		}
		encoding, e := readString(data, "encoding")
		if e != nil {
			return event, e
		}
		if encoding != "base64" {
			return event, protocolErrorf("inspection body encoding must be base64")
		}
		chunk, e := readString(data, "chunk")
		if e != nil {
			return event, e
		}
		event.Body, err = base64.StdEncoding.Strict().DecodeString(chunk)
		if err != nil || base64.StdEncoding.EncodeToString(event.Body) != chunk {
			return event, protocolErrorf("inspection body chunk must be valid base64")
		}
	case "gap":
		from, e := readSequence(data, "fromSequence")
		if e != nil {
			return event, e
		}
		to, e := readSequence(data, "toSequence")
		if e != nil {
			return event, e
		}
		event.Gap = &InspectionGap{from, to, fmt.Sprintf("Inspection stream gap: missing sequences %d-%d", from, to)}
	case "inspection_error":
		if _, err = readString(data, "message"); err != nil {
			return event, err
		}
		err = json.Unmarshal(record["data"], &event.Data)
	case "complete":
		if err = validateComplete(data); err != nil {
			return event, err
		}
		err = json.Unmarshal(record["data"], &event.Complete)
	default:
		err = protocolErrorf("unknown inspection metadata type %s", event.Type)
	}
	return event, err
}
func validateComplete(r map[string]json.RawMessage) error {
	started, e := readString(r, "startedDateTime")
	if e != nil {
		return e
	}
	if _, e = time.Parse(time.RFC3339Nano, started); e != nil {
		return protocolErrorf("startedDateTime must be an ISO date-time")
	}
	if _, e = readNumber(r, "time"); e != nil {
		return e
	}
	for _, kind := range []string{"request", "response"} {
		record, e := readRecord(r[kind])
		if e != nil {
			return e
		}
		fields := []string{"httpVersion"}
		arrays := []string{"headers", "cookies"}
		numbers := []string{"headersSize", "bodySize"}
		if kind == "request" {
			fields = append(fields, "method", "url")
			arrays = append(arrays, "queryString")
			if b, ok := record["postData"]; ok {
				if _, e = readRecord(b); e != nil {
					return e
				}
			}
		} else {
			fields = append(fields, "statusText", "redirectURL")
			numbers = append(numbers, "status")
			content, e := readRecord(record["content"])
			if e != nil {
				return e
			}
			if _, e = readNumber(content, "size"); e != nil {
				return e
			}
			if _, e = readString(content, "mimeType"); e != nil {
				return e
			}
		}
		for _, key := range fields {
			if _, e = readString(record, key); e != nil {
				return e
			}
		}
		for _, key := range numbers {
			if _, e = readNumber(record, key); e != nil {
				return e
			}
		}
		for _, key := range arrays {
			if e = validateNameValues(record[key]); e != nil {
				return e
			}
		}
	}
	extension, e := readRecord(r["_limrun"])
	if e != nil {
		return e
	}
	for _, key := range []string{"tunnelId", "selectorId"} {
		if _, e = readString(extension, key); e != nil {
			return e
		}
	}
	if _, e = readOptionalString(extension, "error"); e != nil {
		return e
	}
	for _, key := range []string{"requestBodyTruncated", "responseBodyTruncated"} {
		if _, e = readOptionalBool(extension, key, false); e != nil {
			return e
		}
	}
	if b, ok := extension["responseTrailers"]; ok {
		if e = validateNameValues(b); e != nil {
			return e
		}
	}
	if b, ok := extension["responseCookieSameSite"]; ok {
		var values []json.RawMessage
		if e = json.Unmarshal(b, &values); e != nil || values == nil {
			return protocolErrorf("responseCookieSameSite must be an array")
		}
		for _, raw := range values {
			value, e := readRecord(raw)
			if e != nil {
				return e
			}
			if _, e = readSequence(value, "index"); e != nil {
				return e
			}
			if _, e = readString(value, "value"); e != nil {
				return e
			}
		}
	}
	return nil
}
func validateNameValues(b []byte) error {
	var values []json.RawMessage
	if e := json.Unmarshal(b, &values); e != nil || values == nil {
		return protocolErrorf("name/value entries must be an array")
	}
	for _, raw := range values {
		r, e := readRecord(raw)
		if e != nil {
			return e
		}
		if _, e = readString(r, "name"); e != nil {
			return e
		}
		if _, e = readString(r, "value"); e != nil {
			return e
		}
	}
	return nil
}
