// Package build executes and observes remote Xcode and Gradle workloads.
package build

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/limrun-inc/go-sdk/internal/direct"
	"github.com/limrun-inc/go-sdk/internal/stream"
)

type ExecRequest interface {
	execRequest()
	budget() time.Duration
	downloadURL() *string
}

func (XcodeBuildExecRequest) execRequest()  {}
func (GradleBuildExecRequest) execRequest() {}
func (RunExecRequest) execRequest()         {}
func (r XcodeBuildExecRequest) budget() time.Duration {
	t := time.Hour
	if r.Testflight != nil {
		seconds := float64(0)
		if r.Testflight.WaitTimeoutSeconds != nil {
			seconds = math.Max(0, *r.Testflight.WaitTimeoutSeconds)
		}
		t += time.Duration(seconds+900) * time.Second
	}
	if r.Xcodebuild != nil && r.Xcodebuild.Action != nil && *r.Xcodebuild.Action == "build-for-testing" {
		t += 2 * time.Hour
	}
	return t
}
func (GradleBuildExecRequest) budget() time.Duration { return time.Hour }
func (r RunExecRequest) budget() time.Duration {
	seconds := float64(3600)
	if r.TimeoutSeconds != nil {
		seconds = math.Max(1, *r.TimeoutSeconds)
	}
	return time.Duration(seconds+60) * time.Second
}
func (r XcodeBuildExecRequest) downloadURL() *string {
	if r.AdditionalMetadata != nil {
		return r.AdditionalMetadata.SignedDownloadURL
	}
	return nil
}
func (r GradleBuildExecRequest) downloadURL() *string {
	if r.AdditionalMetadata != nil {
		return r.AdditionalMetadata.SignedDownloadURL
	}
	return nil
}
func (RunExecRequest) downloadURL() *string { return nil }

type XctestEvent struct {
	Case    *XctestCaseEvent
	Summary *XctestSummaryEvent
}
type ExecOptions struct {
	APIURL, Token           string
	HTTPClient              *http.Client
	Command, Stdout, Stderr io.Writer
	OnEvent                 func(ExecLogEvent)
	OnXctestEvent           func(XctestEvent)
	Log                     func(level, message string)
	Detached                bool
}
type ExecStreamLostError struct{ Cause error }

func (e *ExecStreamLostError) Error() string { return e.Cause.Error() }
func (e *ExecStreamLostError) Unwrap() error { return e.Cause }

type ExecStreamClosedError struct{}

func (*ExecStreamClosedError) Error() string { return stream.ErrClosed.Error() }
func (*ExecStreamClosedError) Unwrap() error { return stream.ErrClosed }

var errTerminal = errors.New("terminal execution event")

type Process struct {
	mu               sync.Mutex
	id               string
	startErr         error
	result           ExecResult
	err              error
	started, done    chan struct{}
	cancel           context.CancelFunc
	detached, killed atomic.Bool
	opts             ExecOptions
	http             direct.Client
}

func Exec(ctx context.Context, request ExecRequest, opts ExecOptions) *Process {
	ctx, cancel := context.WithCancel(ctx)
	p := &Process{started: make(chan struct{}), done: make(chan struct{}), cancel: cancel, opts: opts, http: direct.Client{URL: opts.APIURL, Token: opts.Token, HTTPClient: opts.HTTPClient}}
	p.detached.Store(opts.Detached)
	go p.run(ctx, request)
	return p
}
func (p *Process) ID() string { p.mu.Lock(); defer p.mu.Unlock(); return p.id }
func (p *Process) Wait(ctx context.Context) (ExecResult, error) {
	select {
	case <-ctx.Done():
		return ExecResult{}, ctx.Err()
	case <-p.done:
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.result, p.err
	}
}

// Detach returns once the remote execution has been accepted, without opening
// its event stream or waiting for completion. Call it right after starting the
// process: a detach that lands before following begins leaves Wait pending for
// good, while one that lands later changes nothing and Wait still settles on
// exit. The remote execution keeps running either way; observe it through a
// build-finish webhook or the build-log APIs.
func (p *Process) Detach(ctx context.Context) (string, error) {
	p.detached.Store(true)
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-p.started:
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.id, p.startErr
	}
}
func (p *Process) Kill(ctx context.Context) error {
	p.killed.Store(true)
	p.cancel()
	id := p.ID()
	if id == "" {
		return nil
	}
	return p.http.JSON(ctx, http.MethodPost, "exec/"+url.PathEscape(id)+"/cancel", nil, nil)
}
func (p *Process) log(level, msg string) {
	if p.opts.Log != nil {
		p.opts.Log(level, msg)
	}
}
func (p *Process) run(ctx context.Context, request ExecRequest) {
	var started struct {
		ExecID string `json:"execId"`
	}
	err := p.http.JSON(ctx, http.MethodPost, "exec", request, &started)
	p.mu.Lock()
	p.id = started.ExecID
	p.startErr = err
	p.mu.Unlock()
	close(p.started)
	if err != nil {
		p.mu.Lock()
		if p.killed.Load() {
			p.result = ExecResult{ExitCode: -1, Status: "CANCELLED"}
		} else {
			p.err = err
		}
		p.mu.Unlock()
		close(p.done)
		return
	}
	if p.detached.Load() {
		return
	}
	followCtx, cancel := context.WithTimeout(ctx, request.budget())
	defer cancel()
	result := ExecResult{ExecID: started.ExecID, SignedDownloadURL: request.downloadURL()}
	var deadSince time.Time
	code, err := follow(followCtx, p.http, started.ExecID, false, func(e ExecLogEvent) error {
		if p.opts.OnEvent != nil {
			p.opts.OnEvent(e)
		}
		var writer io.Writer
		switch e.Type {
		case "command":
			writer = p.opts.Command
		case "stdout":
			writer = p.opts.Stdout
		case "stderr":
			writer = p.opts.Stderr
		case "testflight":
			v := AppStoreEvent{State: "unknown"}
			if json.Unmarshal([]byte(e.Data), &v) != nil {
				v = AppStoreEvent{State: "unknown"}
			}
			result.Appstore = &v
		case "playstore":
			v := PlaystoreEvent{State: "unknown"}
			if json.Unmarshal([]byte(e.Data), &v) != nil {
				v = PlaystoreEvent{State: "unknown"}
			}
			result.Playstore = &v
		case "xctest":
			var head struct {
				Type string `json:"type"`
			}
			if json.Unmarshal([]byte(e.Data), &head) != nil {
				break
			}
			event := XctestEvent{}
			switch head.Type {
			case "case":
				var v XctestCaseEvent
				if json.Unmarshal([]byte(e.Data), &v) != nil {
					break
				}
				if result.Xctest == nil {
					result.Xctest = &ExecResultXctest{Cases: []XctestCaseEvent{}}
				}
				result.Xctest.Cases = append(result.Xctest.Cases, v)
				event.Case = &v
			case "summary":
				var v XctestSummaryEvent
				if json.Unmarshal([]byte(e.Data), &v) != nil {
					break
				}
				if result.Xctest == nil {
					result.Xctest = &ExecResultXctest{Cases: []XctestCaseEvent{}}
				}
				result.Xctest.Summary = &v
				event.Summary = &v
			}
			if p.opts.OnXctestEvent != nil && (event.Case != nil || event.Summary != nil) {
				func() {
					defer func() {
						if r := recover(); r != nil {
							p.log("warn", fmt.Sprintf("onXctestEvent callback panicked: %v", r))
						}
					}()
					p.opts.OnXctestEvent(event)
				}()
			}
		}
		if writer != nil {
			_, err := io.WriteString(writer, e.Data)
			return err
		}
		return nil
	}, func(t time.Time) { deadSince = t })
	if err != nil {
		if p.killed.Load() {
			code = -1
		} else {
			code = 1
			flag := true
			result.TimedOut = &flag
			reason := "timeout"
			if errors.Is(err, stream.ErrClosed) {
				reason = "stream-closed"
			} else if errors.Is(err, stream.ErrLost) || !deadSince.IsZero() {
				reason = "stream-lost"
			}
			result.Incomplete = &ExecResultIncomplete{Reason: reason, Message: err.Error()}
		}
	}
	result.ExitCode = code
	result.Status = status(code)
	p.mu.Lock()
	p.result = result
	p.mu.Unlock()
	close(p.done)
}
func status(code int) string {
	if code == 0 {
		return "SUCCEEDED"
	}
	if code == -1 {
		return "CANCELLED"
	}
	return "FAILED"
}
func follow(ctx context.Context, h direct.Client, id string, failOnClientError bool, onEvent func(ExecLogEvent) error, onDead func(time.Time)) (int, error) {
	lastID := int64(-1)
	code := 0
	err := stream.Run(ctx, stream.Options{HTTPClient: h.HTTPClient, URL: strings.TrimRight(h.URL, "/") + "/exec/" + url.PathEscape(id) + "/events?follow=true", Token: h.Token, FailOnClientError: failOnClientError, OnDeadSince: onDead, OnEvent: func(e stream.Event) error {
		if n, err := strconv.ParseInt(e.ID, 10, 64); err == nil {
			if n <= lastID {
				return nil
			}
			lastID = n
		}
		var id *string
		if e.ID != "" {
			id = &e.ID
		}
		event := ExecLogEvent{ID: id, Type: e.Type, Data: e.Data}
		if err := onEvent(event); err != nil {
			return err
		}
		if e.Type == "exitCode" {
			var err error
			code, err = strconv.Atoi(strings.TrimSpace(e.Data))
			if err != nil {
				return fmt.Errorf("invalid exec exit code: %s", e.Data)
			}
			return errTerminal
		}
		return nil
	}})
	if errors.Is(err, errTerminal) {
		return code, nil
	}
	if errors.Is(err, stream.ErrLost) {
		err = &ExecStreamLostError{err}
	}
	if errors.Is(err, stream.ErrClosed) {
		err = &ExecStreamClosedError{}
	}
	return code, err
}

type ExecLogOptions struct {
	Follow  bool
	OnEvent func(ExecLogEvent)
}

func ObserveExecLogs(ctx context.Context, execID string, opts ExecOptions, logs ExecLogOptions) (*ExecLogResult, error) {
	if strings.TrimSpace(execID) == "" {
		return nil, errors.New("execId must not be empty")
	}
	h := direct.Client{URL: opts.APIURL, Token: opts.Token, HTTPClient: opts.HTTPClient}
	if logs.Follow && (execID == "active" || execID == "latest") {
		var buffered []ExecLogEvent
		snapshot, err := ObserveExecLogs(ctx, execID, opts, ExecLogOptions{OnEvent: func(e ExecLogEvent) { buffered = append(buffered, e) }})
		if err != nil {
			return nil, err
		}
		if snapshot.Status != "RUNNING" {
			if logs.OnEvent != nil {
				for _, e := range buffered {
					logs.OnEvent(e)
				}
			}
			return snapshot, nil
		}
		if snapshot.ExecID == execID {
			return nil, fmt.Errorf("server did not resolve the %s build alias", execID)
		}
		return ObserveExecLogs(ctx, snapshot.ExecID, opts, logs)
	}
	result := &ExecLogResult{ExecID: execID, Status: "RUNNING"}
	onEvent := func(e ExecLogEvent) error {
		if e.Type == "meta" {
			var m struct {
				ID string `json:"id"`
			}
			if json.Unmarshal([]byte(e.Data), &m) == nil && m.ID != "" {
				result.ExecID = m.ID
			}
		}
		if logs.OnEvent != nil {
			logs.OnEvent(e)
		}
		if e.Type == "exitCode" {
			code, err := strconv.Atoi(strings.TrimSpace(e.Data))
			if err != nil {
				return fmt.Errorf("invalid exec exit code: %s", e.Data)
			}
			result.ExitCode = &code
			result.Status = status(code)
		}
		return nil
	}
	if logs.Follow {
		_, err := follow(ctx, h, execID, true, onEvent, nil)
		return result, err
	}
	res, err := h.Do(ctx, http.MethodGet, strings.TrimRight(h.URL, "/")+"/exec/"+url.PathEscape(execID)+"/events?follow=false", nil, "", -1)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	normalized := strings.ReplaceAll(strings.ReplaceAll(string(body), "\r\n", "\n"), "\r", "\n")
	err = stream.Read(strings.NewReader(normalized+"\n\n"), func(e stream.Event) error {
		var id *string
		if e.ID != "" {
			id = &e.ID
		}
		return onEvent(ExecLogEvent{ID: id, Type: e.Type, Data: e.Data})
	}, func() {}, func(time.Duration) {})
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return result, err
}
