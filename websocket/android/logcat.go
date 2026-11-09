package android

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/limrun-inc/go-sdk/session"
)

type LogcatEntry struct {
	TS       float64  `json:"ts"`
	Lines    []string `json:"lines,omitempty"`
	Error    string   `json:"error,omitempty"`
	ExitCode *int     `json:"exitCode,omitempty"`
}
type LogcatProcess struct {
	client *Client
	id     string
	stream *session.Stream[LogcatEntry]
	lines  chan []string
	errors chan error
	done   chan struct{}
	once   sync.Once
	code   int
}

func (c *Client) Logcat(ctx context.Context, args ...string) (*LogcatProcess, error) {
	if args == nil {
		args = []string{}
	}
	var r struct {
		ID string `json:"id"`
	}
	if err := c.http.JSON(ctx, http.MethodPost, "logcat", map[string]any{"args": args}, &r); err != nil {
		return nil, err
	}
	if r.ID == "" {
		return nil, errors.New("logcat failed to start: server returned no run id")
	}
	p := &LogcatProcess{client: c, id: r.ID, lines: make(chan []string, 16), errors: make(chan error, 16), done: make(chan struct{}), code: -1}
	p.stream = session.StartStream[LogcatEntry](ctx, c.http.HTTPClient, c.http.URL+"/logcat/"+url.PathEscape(r.ID)+"/events", c.http.Token)
	go p.run()
	return p, nil
}
func (p *LogcatProcess) Lines() <-chan []string { return p.lines }
func (p *LogcatProcess) Err() <-chan error      { return p.errors }
func (p *LogcatProcess) Wait() int              { <-p.done; return p.code }
func (p *LogcatProcess) finish(code int) {
	p.once.Do(func() { p.code = code; p.stream.Stop(); close(p.done) })
}
func (p *LogcatProcess) deleteRun() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = p.client.http.JSON(ctx, http.MethodDelete, "logcat/"+url.PathEscape(p.id), nil, nil)
}
func (p *LogcatProcess) Stop() {
	select {
	case <-p.done:
		return
	default:
	}
	p.finish(-1)
	go p.deleteRun()
}
func (p *LogcatProcess) run() {
	defer close(p.lines)
	defer close(p.errors)
	for entry := range p.stream.Entries() {
		if len(entry.Lines) > 0 {
			select {
			case p.lines <- entry.Lines:
			case <-p.done:
				return
			}
		}
		if entry.Error != "" {
			select {
			case p.errors <- errors.New(entry.Error):
			case <-p.done:
				return
			}
		}
		if entry.ExitCode != nil {
			p.finish(*entry.ExitCode)
			return
		}
	}
	for err := range p.stream.Err() {
		select {
		case p.errors <- err:
		case <-p.done:
			return
		}
	}
	p.finish(-1)
	go p.deleteRun()
}
