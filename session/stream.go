package session

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/limrun-inc/go-sdk/internal/stream"
)

type Stream[T any] struct {
	entries chan T
	errors  chan error
	cancel  context.CancelFunc
	done    chan struct{}
}

func (s *Stream[T]) Entries() <-chan T     { return s.entries }
func (s *Stream[T]) Err() <-chan error     { return s.errors }
func (s *Stream[T]) Stop()                 { s.cancel() }
func (s *Stream[T]) Done() <-chan struct{} { return s.done }
func StartStream[T any](ctx context.Context, client *http.Client, url, token string) *Stream[T] {
	ctx, cancel := context.WithCancel(ctx)
	s := &Stream[T]{make(chan T, 32), make(chan error, 1), cancel, make(chan struct{})}
	go func() {
		defer close(s.entries)
		defer close(s.errors)
		defer close(s.done)
		err := stream.Run(ctx, stream.Options{HTTPClient: client, URL: url, Token: token, OnEvent: func(e stream.Event) error {
			var check map[string]json.RawMessage
			if json.Unmarshal([]byte(e.Data), &check) != nil {
				return nil
			}
			var ts float64
			if b, ok := check["ts"]; !ok || string(b) == "null" || json.Unmarshal(b, &ts) != nil {
				return nil
			}
			var value T
			if json.Unmarshal([]byte(e.Data), &value) != nil {
				return nil
			}
			select {
			case s.entries <- value:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}})
		if err != nil && !errors.Is(err, context.Canceled) {
			if errors.Is(err, stream.ErrLost) {
				err = &StreamLostError{err}
			}
			s.errors <- err
		}
	}()
	return s
}
