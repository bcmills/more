// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package moreexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
)

// QuitSignal is syscall.SIGQUIT if it is defined and supported, or nil otherwise.
var QuitSignal os.Signal = quitSignal

var ErrWaitDelay = errors.New("moreexec: WaitDelay expired before I/O complete")

// A Cmd is like an exec.Cmd, but with additional fields as proposed in
// https://go.dev/issue/50436.
type Cmd struct {
	Path         string
	Args         []string
	Env          []string
	Dir          string
	Stdin        io.Reader
	Stdout       io.Writer
	Stderr       io.Writer
	ExtraFiles   []*os.File
	SysProcAttr  *syscall.SysProcAttr
	Process      *os.Process
	ProcessState *os.ProcessState

	// Context is the context that controls the lifetime of the command
	// (typically the one passed to CommandContext).
	Context context.Context

	// If Interrupt is non-nil, Context must also be non-nil and Interrupt will be
	// sent to the child process when Context is done.
	//
	// If the command exits with a success code after the Interrupt signal has
	// been sent, Wait and similar methods will return Context.Err()
	// instead of nil.
	//
	// If the Interrupt signal is not supported on the current platform
	// (for example, if it is os.Interrupt on Windows), Start may fail
	// (and return a non-nil error).
	Interrupt os.Signal

	// If WaitDelay is non-zero, the command's I/O pipes will be closed after
	// WaitDelay has elapsed after either the command's process has exited or
	// (if Context is non-nil) Context is done, whichever occurs first.
	// If the command's process is still running after WaitDelay has elapsed,
	// it will be terminated with os.Kill before the pipes are closed.
	//
	// If the command exits with a success code after pipes are closed due to
	// WaitDelay and no Interrupt signal has been sent, Wait and similar methods
	// will return ErrWaitDelay instead of nil.
	//
	// If WaitDelay is zero (the default), I/O pipes will be read until EOF,
	// which might not occur until orphaned subprocesses of the command have
	// also closed their descriptors for the pipes.
	WaitDelay time.Duration

	statec <-chan *os.ProcessState
	err    error // Set before statec receives the process state.

	runningPipes sync.WaitGroup
	pipeCopiers  []func()
	localPipes   []io.Closer
	remotePipes  []io.Closer
}

func Command(name string, args ...string) *Cmd {
	c := &Cmd{
		Path: name,
		Args: append([]string{name}, args...),
	}
	if filepath.Base(name) == name {
		if path, err := exec.LookPath(name); err == nil {
			c.Path = path
		}
	}
	return c
}

func CommandContext(ctx context.Context, name string, args ...string) *Cmd {
	c := Command(name, args...)
	c.Context = ctx
	c.Interrupt = os.Kill
	return c
}

func (c *Cmd) String() string {
	return (&exec.Cmd{Path: c.Path, Args: c.Args}).String()
}

func (c *Cmd) Start() (err error) {
	if c.Interrupt != nil {
		if c.Context == nil {
			return errors.New("moreexec: Interrupt requires a non-nil Context")
		}
		if runtime.GOOS == "windows" && c.Interrupt != os.Kill {
			return fmt.Errorf("moreexec: signal %q: %w", c.Interrupt, errWindows)
		}
	}

	if c.statec != nil {
		return errors.New("moreexec: already started")
	}
	statec := make(chan *os.ProcessState, 1)

	defer func() {
		// The remote ends of the pipes are either connected to the process or
		// unneeded, so we can close and collect them.
		for _, f := range c.remotePipes {
			f.Close()
		}
		c.remotePipes = nil

		if err == nil {
			c.statec = statec
		} else {
			// Since the process didn't start, we can also close and collect
			// the local ends of the pipes: nothing will be writing to them.
			for _, f := range c.localPipes {
				f.Close()
			}
			c.localPipes = nil
			c.runningPipes.Wait()
		}
	}()

	cmd := exec.Command(c.Path)
	cmd.Args = c.Args
	if c.Dir != "" {
		cmd.Dir = c.Dir
		if c.Env == nil {
			cmd.Env = append(os.Environ(), "PWD="+c.Dir)
		} else {
			cmd.Env = append(c.Env[:len(c.Env):len(c.Env)], "PWD="+c.Dir)
		}
	} else {
		cmd.Env = c.Env
	}
	cmd.ExtraFiles = c.ExtraFiles
	cmd.SysProcAttr = c.SysProcAttr

	// As a workaround for https://go.dev/issue/23019, we inject our own I/O pipes
	// as needed. If we need to forcibly terminate the process, we can also close
	// those pipes to cause the copying goroutines to exit.

	if _, ok := c.Stdin.(*os.File); ok || c.Stdin == nil {
		cmd.Stdin = c.Stdin
	} else {
		r, w, err := c.newInputPipe()
		if err != nil {
			return err
		}
		cmd.Stdin = r
		c.startPipe(w, c.Stdin, w)
	}

	if _, ok := c.Stdout.(*os.File); ok || c.Stdout == nil {
		cmd.Stdout = c.Stdout
	} else {
		r, w, err := c.newOutputPipe()
		if err != nil {
			return err
		}
		if c.Stderr == c.Stdout {
			cmd.Stderr = w
		}
		cmd.Stdout = w
		c.startPipe(c.Stdout, r, r)
	}

	if c.Stderr != c.Stdout {
		if _, ok := c.Stderr.(*os.File); ok || c.Stderr == nil {
			cmd.Stderr = c.Stderr
		} else {
			r, w, err := c.newOutputPipe()
			if err != nil {
				return err
			}
			cmd.Stderr = w
			c.startPipe(c.Stderr, r, r)
		}
	}

	err = cmd.Start()
	c.Process = cmd.Process
	if err == nil {
		go c.wait(statec, cmd)
	}
	return err
}

func (c *Cmd) wait(statec chan<- *os.ProcessState, cmd *exec.Cmd) {
	var (
		cancel context.CancelFunc
		errc   chan error
	)
	if c.Interrupt != nil || c.WaitDelay != 0 {
		ctx := c.Context
		if ctx == nil {
			ctx = context.Background()
		}
		if c.WaitDelay != 0 {
			ctx, cancel = context.WithCancel(ctx)
		}

		errc = make(chan error)
		go func() {
			select {
			case errc <- nil:
				return
			case <-ctx.Done():
			}

			var err error
			if c.Interrupt != nil {
				if signalErr := c.Process.Signal(c.Interrupt); signalErr == nil {
					// We appear to have successfully delivered c.Interrupt, so any
					// program behavior from this point may be due to ctx.
					err = ctx.Err()
				} else if !isProcessDone(signalErr) {
					err = fmt.Errorf("moreexec: error sending signal to Cmd: %w", signalErr)
				}
			}

			if c.WaitDelay != 0 {
				timer := time.NewTimer(c.WaitDelay)
				select {
				case errc <- err:
					timer.Stop()
					return
				case <-timer.C:
				}

				// Either Wait still hasn't returned or the I/O goroutines are still running.
				//
				// Kill the process to make sure that it exits.
				// Ignore any error from Kill: if cmd.Process has already terminated, we
				// still want to send ctx.Err() (or the error from Signal) to inform the
				// caller that we needed to terminate the output pipes.
				if err == nil {
					err = ErrWaitDelay
				}
				_ = cmd.Process.Kill()

				// Close the pipes to which the process writes, in case the process
				// abandoned any subprocesses that are still running. Terminate the
				// pipes after the process itself: we would prefer for the process to
				// die of SIGKILL, not SIGPIPE. (However, this may cause any orphaned
				// subprocessed to terminate with SIGPIPE.)
				for _, p := range c.localPipes {
					p.Close()
				}
			}

			errc <- err
		}()
	}

	c.err = cmd.Wait()
	if cancel != nil {
		cancel() // Start the WaitDelay timer, if applicable.
	}
	c.runningPipes.Wait()

	if errc != nil {
		interruptErr := <-errc
		// If Wait returned an error, prefer that. Otherwise,
		// report any error from the interrupt goroutine, such
		// as a Context cancellation or a WaitDelay overrun.
		if interruptErr != nil && c.err == nil {
			c.err = interruptErr
		}
	}

	for _, p := range c.localPipes {
		p.Close()
	}
	c.localPipes = nil

	statec <- cmd.ProcessState
	close(statec)
}

func (c *Cmd) StdinPipe() (io.WriteCloser, error) {
	if c.Stdin != nil {
		return nil, errors.New("moreexec: Stdin already set")
	}
	if c.Process != nil {
		return nil, errors.New("moreexec: StdinPipe after process started")
	}
	r, w, err := c.newInputPipe()
	c.Stdin = r
	return w, err
}

func (c *Cmd) StdoutPipe() (io.ReadCloser, error) {
	if c.Stdout != nil {
		return nil, errors.New("moreexec: Stdout already set")
	}
	if c.Process != nil {
		return nil, errors.New("moreexec: StdoutPipe after process started")
	}
	r, w, err := c.newOutputPipe()
	c.Stdout = w
	return r, err
}

func (c *Cmd) StderrPipe() (io.ReadCloser, error) {
	if c.Stderr != nil {
		return nil, errors.New("moreexec: Stderr already set")
	}
	if c.Process != nil {
		return nil, errors.New("moreexec: StderrPipe after process started")
	}
	r, w, err := c.newOutputPipe()
	c.Stderr = w
	return r, err
}

func (c *Cmd) newInputPipe() (io.ReadCloser, io.WriteCloser, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	c.remotePipes = append(c.remotePipes, r)
	c.localPipes = append(c.localPipes, w)
	return r, w, nil
}

func (c *Cmd) newOutputPipe() (io.ReadCloser, io.WriteCloser, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	c.localPipes = append(c.localPipes, r)
	c.remotePipes = append(c.remotePipes, w)
	return r, w, nil
}

func (c *Cmd) startPipe(dst io.Writer, src io.Reader, local io.Closer) {
	c.runningPipes.Add(1)
	go func() {
		io.Copy(dst, src)
		local.Close()
		c.runningPipes.Done()
	}()
}

// Wait waits for the already-started command cmd.
func (c *Cmd) Wait() error {
	if c.statec == nil {
		return errors.New("moreexec: not started")
	}

	state, ok := <-c.statec
	if !ok {
		return errors.New("moreexec: Wait was already called")
	}
	c.ProcessState = state
	return c.err
}

// CombinedOutput runs the command and returns its combined standard output and
// standard error.
func (c *Cmd) CombinedOutput() ([]byte, error) {
	if c.Stdout != nil {
		return nil, errors.New("moreexec: Stdout already set")
	}
	if c.Stderr != nil {
		return nil, errors.New("moreexec: Stderr already set")
	}

	b := new(bytes.Buffer)
	c.Stdout = b
	c.Stderr = b
	err := c.Run()
	return b.Bytes(), err
}

func (c *Cmd) Run() error {
	err := c.Start()
	if err == nil {
		err = c.Wait()
	}
	return err
}
