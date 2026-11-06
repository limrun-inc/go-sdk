package ios

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// SimctlCmd represents a simctl command to be run remotely.
// Its API mirrors os/exec.Cmd for familiarity.
type SimctlCmd struct {
	// Args holds the command arguments (not including "simctl" itself).
	Args []string

	// Stdin is sent as base64 in the command frame.
	Stdin            []byte
	DisconnectOnExit bool

	// Stdout specifies the process's standard output.
	// If Stdout is nil, output is discarded.
	// If Stdout is an *os.File, output is written to that file.
	Stdout io.Writer

	// Stderr specifies the process's standard error.
	// If Stderr is nil, output is discarded.
	// If Stderr is the same as Stdout, both are written to the same writer.
	Stderr io.Writer

	client         *Client
	ctx            context.Context
	id             string
	started        bool
	finished       bool
	mu             sync.Mutex
	done           chan struct{}
	err            error
	exitCode       int
	stdoutPipe     *io.PipeWriter
	stderrPipe     *io.PipeWriter
	closeAfterWait []io.Closer
	queue          []simctlOutput
	wake           chan struct{}
}

// Run starts the command and waits for it to complete.
// This is equivalent to calling Start followed by Wait.
func (c *SimctlCmd) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}

// Start starts the command but does not wait for it to complete.
// The Wait method will return the exit code and release resources once the command exits.
func (c *SimctlCmd) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return errors.New("simctl: already started")
	}
	c.started = true

	if c.client.closed.Load() || c.client.transport == nil {
		c.done = make(chan struct{})
		c.err = ErrNotConnected
		c.finished = true
		close(c.done)
		return ErrNotConnected
	}

	c.id = fmt.Sprintf("go-%d-%d", time.Now().UnixNano(), c.client.requestID.Add(1))
	c.done = make(chan struct{})
	c.client.simctlExecutions.Store(c.id, c)

	req := map[string]any{"type": "simctl", "id": c.id, "args": c.Args}
	if c.Stdin != nil {
		req["stdin"] = base64.StdEncoding.EncodeToString(c.Stdin)
	}
	c.wake = make(chan struct{}, 1)
	if err := c.client.transport.Send(req); err != nil {
		c.client.simctlExecutions.Delete(c.id)
		c.err = err
		c.finished = true
		close(c.done)
		return err
	}
	go c.drain()

	// If context was provided, watch for cancellation
	if c.ctx != nil {
		go func() {
			select {
			case <-c.ctx.Done():
				c.Kill()
				c.handleError(c.ctx.Err())
			case <-c.done:
				// Command finished normally
			}
		}()
	}

	return nil
}

// Wait waits for the command to exit and waits for any copying to stdout or stderr to complete.
// Wait must be called after Start.
func (c *SimctlCmd) Wait() error {
	c.mu.Lock()
	if !c.started {
		c.mu.Unlock()
		return errors.New("simctl: not started")
	}
	c.mu.Unlock()

	<-c.done

	// Close any pipes
	for _, closer := range c.closeAfterWait {
		closer.Close()
	}

	if c.err != nil {
		return c.err
	}
	if c.exitCode != 0 {
		return fmt.Errorf("simctl: exit code %d", c.exitCode)
	}
	return nil
}

// ExitCode returns the exit code of the exited process.
// This should only be called after Wait returns.
func (c *SimctlCmd) ExitCode() int {
	return c.exitCode
}

// StdoutPipe returns a pipe that will be connected to the command's standard output when the command starts.
// Wait will close the pipe after seeing the command exit.
func (c *SimctlCmd) StdoutPipe() (io.ReadCloser, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return nil, errors.New("simctl: StdoutPipe after Start")
	}
	if c.Stdout != nil {
		return nil, errors.New("simctl: Stdout already set")
	}

	pr, pw := io.Pipe()
	c.Stdout = pw
	c.stdoutPipe = pw
	c.closeAfterWait = append(c.closeAfterWait, pw)
	return pr, nil
}

// StderrPipe returns a pipe that will be connected to the command's standard error when the command starts.
// Wait will close the pipe after seeing the command exit.
func (c *SimctlCmd) StderrPipe() (io.ReadCloser, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return nil, errors.New("simctl: StderrPipe after Start")
	}
	if c.Stderr != nil {
		return nil, errors.New("simctl: Stderr already set")
	}

	pr, pw := io.Pipe()
	c.Stderr = pw
	c.stderrPipe = pw
	c.closeAfterWait = append(c.closeAfterWait, pw)
	return pr, nil
}

// Output runs the command and returns its standard output.
func (c *SimctlCmd) Output() ([]byte, error) {
	if c.Stdout != nil {
		return nil, errors.New("simctl: Stdout already set")
	}
	var buf bytes.Buffer
	c.Stdout = &buf
	err := c.Run()
	return buf.Bytes(), err
}

// CombinedOutput runs the command and returns its combined standard output and standard error.
func (c *SimctlCmd) CombinedOutput() ([]byte, error) {
	if c.Stdout != nil {
		return nil, errors.New("simctl: Stdout already set")
	}
	if c.Stderr != nil {
		return nil, errors.New("simctl: Stderr already set")
	}
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	err := c.Run()
	return buf.Bytes(), err
}

type simctlOutput struct {
	stdout, stderr []byte
	exitCode       *int
}

func (c *SimctlCmd) handleOutput(stdout, stderr []byte, exitCode *int) {
	c.mu.Lock()
	if !c.finished {
		c.queue = append(c.queue, simctlOutput{stdout, stderr, exitCode})
		select {
		case c.wake <- struct{}{}:
		default:
		}
	}
	c.mu.Unlock()
}
func (c *SimctlCmd) drain() {
	for {
		select {
		case <-c.done:
			return
		case <-c.wake:
		}
		for {
			c.mu.Lock()
			if c.finished || len(c.queue) == 0 {
				c.mu.Unlock()
				break
			}
			item := c.queue[0]
			c.queue = c.queue[1:]
			c.mu.Unlock()
			if len(item.stdout) > 0 && c.Stdout != nil {
				if _, err := c.Stdout.Write(item.stdout); err != nil {
					c.handleError(err)
					return
				}
			}
			if len(item.stderr) > 0 && c.Stderr != nil {
				if _, err := c.Stderr.Write(item.stderr); err != nil {
					c.handleError(err)
					return
				}
			}
			if item.exitCode != nil {
				c.finish(*item.exitCode, nil)
				return
			}
		}
	}
}
func (c *SimctlCmd) finish(code int, err error) {
	c.mu.Lock()
	if c.finished {
		c.mu.Unlock()
		return
	}
	c.finished = true
	c.exitCode = code
	c.err = err
	for _, p := range c.closeAfterWait {
		p.Close()
	}
	close(c.done)
	c.mu.Unlock()
	c.client.simctlExecutions.Delete(c.id)
	if c.DisconnectOnExit {
		c.client.Close()
	}
}
func (c *SimctlCmd) handleError(err error) { c.finish(-1, err) }

// Kill terminates the running command by sending a terminate request to the server.
// The process will exit and Wait will return with an error indicating termination.
func (c *SimctlCmd) Kill() error {
	c.mu.Lock()
	if !c.started {
		c.mu.Unlock()
		return errors.New("simctl: not started")
	}
	if c.finished {
		c.mu.Unlock()
		return nil // Already finished
	}
	id := c.id
	c.mu.Unlock()

	return c.client.transport.Send(map[string]any{"type": "simctlTerminate", "id": id})
}
