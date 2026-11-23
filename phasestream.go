package limrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/limrun-inc/go-sdk/internal/requestconfig"
	"github.com/limrun-inc/go-sdk/internal/stream"
	"github.com/limrun-inc/go-sdk/option"
)

// phaseStream describes one instance's phase stream: an endpoint that answers JSON, or an event
// per phase change when asked for text/event-stream, and a "gone" event when the instance leaves.
// The Xcode cache and the Gradle snapshot both serve this shape.
type phaseStream[T any] struct {
	endpoint  string
	eventName string
	// confirmGone asks the endpoint for JSON; a 404 means the instance is over.
	confirmGone   func(ctx context.Context) error
	transitionKey func(T) string
	isTerminal    func(T) bool
	onUpdate      func(T)
	onOpen        func()
}

var errPhaseTerminal = errors.New("phase stream reached a terminal state")

// errPhaseTimeout reports that the wait ran out; last is the final state seen, if any.
type errPhaseTimeout[T any] struct{ last *T }

func (e *errPhaseTimeout[T]) Error() string { return "phase stream timed out" }

// follow reads the stream until a state is terminal, the instance is collected, or ctx ends. It
// reconnects when the server closes the stream, which Kubernetes watches do routinely. gone is
// true when the instance was collected; last is then the final state seen, or nil if none was.
func (p phaseStream[T]) follow(ctx context.Context, timeout time.Duration, opts []option.RequestOption) (last *T, gone bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	lastKey := ""
	delay := time.Second
	for {
		var res *http.Response
		requestOpts := slices.Concat(opts, []option.RequestOption{option.WithHeader("Accept", "text/event-stream"), option.WithMaxRetries(0)})
		err := requestconfig.ExecuteNewRequest(ctx, http.MethodGet, p.endpoint, nil, &res, requestOpts...)
		var apiErr *Error
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			gone = true
		} else if err == nil {
			if p.onOpen != nil {
				p.onOpen()
			}
			err = stream.Read(res.Body, func(e stream.Event) error {
				if e.Type != p.eventName && e.Type != "gone" {
					return nil
				}
				var state T
				if err := json.Unmarshal([]byte(e.Data), &state); err != nil {
					return fmt.Errorf("%s stream sent unreadable data: %w", p.eventName, err)
				}
				last = &state
				if e.Type == "gone" {
					// The server also sends gone when its own watch is cut short, so a request
					// decides; the stream reconnects when the instance is in fact still there.
					confirmErr := p.confirmGone(ctx)
					var httpErr *Error
					if errors.As(confirmErr, &httpErr) && httpErr.StatusCode == 404 {
						gone = true
						return errPhaseTerminal
					}
					return nil
				}
				// A reconnect replays the current state, which is not a transition.
				if key := p.transitionKey(state); key != lastKey {
					lastKey = key
					if p.onUpdate != nil {
						p.onUpdate(state)
					}
				}
				if p.isTerminal(state) {
					return errPhaseTerminal
				}
				return nil
			}, func() {}, func(d time.Duration) { delay = d })
			res.Body.Close()
			if errors.Is(err, errPhaseTerminal) && !gone {
				return last, false, nil
			}
			if err != nil && !errors.Is(err, errPhaseTerminal) && ctx.Err() == nil {
				var syntax *json.SyntaxError
				if errors.As(err, &syntax) {
					return nil, false, err
				}
			}
		}
		if gone {
			return last, true, nil
		}
		if ctx.Err() != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, false, &errPhaseTimeout[T]{last}
			}
			return nil, false, ctx.Err()
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
}
