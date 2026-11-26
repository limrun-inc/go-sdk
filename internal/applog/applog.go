// Package applog follows an instance's app log, which iOS and macOS instances
// serve as server-sent events at GET <apiUrl>/app-logs/stream, one event per
// line. The stream has no event ids and replays nothing.
package applog

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/limrun-inc/go-sdk/internal/direct"
	"github.com/limrun-inc/go-sdk/internal/stream"
)

// ErrEnded is returned by Read when the instance ends the stream.
var ErrEnded = errors.New("the app log stream ended")

// Open requests the app log of bundleID. A non-2xx response is an error. The
// response body lives as long as ctx.
func Open(ctx context.Context, c direct.Client, bundleID string) (io.ReadCloser, error) {
	target := strings.TrimRight(c.URL, "/") + "/app-logs/stream?" + url.Values{"bundleId": {bundleID}}.Encode()
	res, err := c.Do(ctx, http.MethodGet, target, nil, "", -1)
	if err != nil {
		return nil, err
	}
	return res.Body, nil
}

// Read calls emit with each line of an opened app log until the stream ends,
// which returns ErrEnded, or reading or emit fails.
func Read(body io.Reader, emit func(string) error) error {
	err := stream.Read(body, func(e stream.Event) error { return emit(e.Data) }, func() {}, func(time.Duration) {})
	if err == nil || errors.Is(err, io.EOF) {
		return ErrEnded
	}
	return err
}
