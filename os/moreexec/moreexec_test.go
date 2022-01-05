// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package moreexec_test

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/bcmills/more/os/moreexec"
)

var (
	sleep           = flag.Duration("sleep", 0, "amount of time to sleep instead of running tests")
	exitOnInterrupt = flag.Bool("interrupt", false, "if true, exit 0 on os.Interrupt")
	subsleep        = flag.Duration("subsleep", 0, "amount of time to leave an orphaned subprocess sleeping with stderr open")
	probe           = flag.Duration("probe", 0, "if nonzero, period at which to print to stderr to check for liveness")
)

var exeOnce struct {
	path string
	sync.Once
}

// exePath returns the path to the running executable.
func exePath() string {
	exeOnce.Do(func() {
		var err error
		exeOnce.path, err = os.Executable()
		if err != nil {
			exeOnce.path = os.Args[0]
		}
	})

	return exeOnce.path
}

func TestMain(m *testing.M) {
	flag.Parse()

	pid := os.Getpid()

	if *probe != 0 {
		ticker := time.NewTicker(*probe)
		go func() {
			for range ticker.C {
				if _, err := fmt.Fprintln(os.Stderr, pid, "ok"); err != nil {
					os.Exit(1)
				}
			}
		}()
	}

	if *subsleep != 0 {
		cmd := moreexec.Command(exePath(), "-sleep", subsleep.String(), "-probe", probe.String())
		cmd.Stderr = os.Stderr
		out, err := cmd.StdoutPipe()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		cmd.Start()

		buf := new(strings.Builder)
		if _, err := io.Copy(buf, out); err != nil {
			fmt.Fprintln(os.Stderr, err)
			cmd.Process.Kill()
			cmd.Wait()
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, pid, "started", cmd.Process.Pid)
	}

	if *sleep != 0 {
		c := make(chan os.Signal, 1)
		if *exitOnInterrupt {
			signal.Notify(c, os.Interrupt)
		} else {
			signal.Ignore(os.Interrupt)
		}

		// Signal that the process is set up by closing stdout.
		os.Stdout.Close()

		select {
		case <-time.After(*sleep):
			fmt.Fprintln(os.Stderr, pid, "slept", *sleep)
		case sig := <-c:
			fmt.Fprintln(os.Stderr, pid, "received", sig)
		}
	}

	if *probe != 0 || *subsleep != 0 || *sleep != 0 {
		fmt.Fprintln(os.Stderr, pid, "exiting")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func start(t *testing.T, ctx context.Context, interrupt os.Signal, killDelay time.Duration, args ...string) *moreexec.Cmd {
	t.Helper()

	cmd := moreexec.CommandContext(ctx, exePath(), args...)
	cmd.Stderr = new(strings.Builder)
	cmd.Interrupt = interrupt
	cmd.WaitDelay = killDelay
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	t.Log(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// Wait for cmd to close stdout to signal that its handlers are installed.
	buf := new(strings.Builder)
	if _, err := io.Copy(buf, out); err != nil {
		t.Error(err)
		cmd.Process.Kill()
		cmd.Wait()
		t.FailNow()
	}
	if buf.Len() > 0 {
		t.Logf("stdout %v:\n%s", cmd.Args, buf)
	}

	return cmd
}

func TestWaitContext(t *testing.T) {
	t.Run("Wait", func(t *testing.T) {
		cmd := start(t, context.Background(), os.Kill, 0, "-sleep=1ms")
		err := cmd.Wait()
		t.Logf("stderr:\n%s", cmd.Stderr)
		t.Logf("[%d] %v", cmd.Process.Pid, err)

		if err != nil {
			t.Errorf("Wait: %v; want <nil>", err)
		}
		if ps := cmd.ProcessState; !ps.Exited() {
			t.Errorf("cmd did not exit: %v", ps)
		} else if code := ps.ExitCode(); code != 0 {
			t.Errorf("cmd.ProcessState.ExitCode() = %v; want 0", code)
		}
	})

	t.Run("WaitDelay", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skipf("skipping: os.Interrupt is not implemented on Windows")
		}

		ctx, cancel := context.WithCancel(context.Background())
		cmd := start(t, ctx, nil, 10*time.Minute, "-sleep=10m", "-interrupt=true")
		cancel()

		time.Sleep(1 * time.Millisecond)
		cmd.Process.Signal(os.Interrupt)

		err := cmd.Wait()
		t.Logf("stderr:\n%s", cmd.Stderr)
		t.Logf("[%d] %v", cmd.Process.Pid, err)

		// This program exits with status 0,
		// but pretty much always does so during the wait delay.
		// Since the Cmd itself didn't do anything to stop the process when the
		// context expired, a successful exit is valid (even if late) and does
		// not merit a non-nil error.
		if err != nil {
			t.Errorf("Wait: %v; want %v", err, ctx.Err())
		}
		if ps := cmd.ProcessState; !ps.Exited() {
			t.Errorf("cmd did not exit: %v", ps)
		} else if code := ps.ExitCode(); code != 0 {
			t.Errorf("cmd.ProcessState.ExitCode() = %v; want 0", code)
		}
	})

	t.Run("SIGKILL-hang", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cmd := start(t, ctx, os.Kill, 10*time.Millisecond, "-sleep=10m", "-subsleep=10m", "-probe=1ms")
		cancel()
		err := cmd.Wait()
		t.Logf("stderr:\n%s", cmd.Stderr)
		t.Logf("[%d] %v", cmd.Process.Pid, err)

		// This test should kill the child process after 10ms,
		// leaving a grandchild process writing probes in a loop.
		// The child process should be reported as failed,
		// and the grandchild will exit (or die by SIGPIPE) once the
		// stderr pipe is closed.
		if ee := new(*exec.ExitError); !errors.As(err, ee) {
			t.Errorf("Wait error = %v; want %T", err, *ee)
		}
	})

	t.Run("Exit-hang", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cmd := start(t, ctx, nil, 10*time.Millisecond, "-subsleep=10m", "-probe=1ms")
		cancel()
		err := cmd.Wait()
		t.Logf("stderr:\n%s", cmd.Stderr)
		t.Logf("[%d] %v", cmd.Process.Pid, err)

		// This child process should exit immediately,
		// leaving a grandchild process writing probes in a loop.
		// Since the child has no ExitError to report but we did not
		// read all of its output, Wait should return ErrWaitDelay.
		if !errors.Is(err, moreexec.ErrWaitDelay) {
			t.Errorf("Wait error = %v; want %T", err, moreexec.ErrWaitDelay)
		}
	})

	t.Run("SIGINT-ignored", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skipf("skipping: os.Interrupt is not implemented on Windows")
		}

		ctx, cancel := context.WithCancel(context.Background())
		cmd := start(t, ctx, os.Interrupt, 10*time.Millisecond, "-sleep=10m", "-interrupt=false")
		cancel()
		err := cmd.Wait()
		t.Logf("stderr:\n%s", cmd.Stderr)
		t.Logf("[%d] %v", cmd.Process.Pid, err)

		// This command ignores SIGINT, sleeping until it is killed.
		// Wait should return the usual error for a killed process.
		if ee := new(*exec.ExitError); !errors.As(err, ee) {
			t.Errorf("Wait error = %v; want %T", err, *ee)
		}
		if ps := cmd.ProcessState; ps.Exited() {
			t.Errorf("cmd unexpectedly exited: %v", ps)
		} else if sys, ok := ps.Sys().(interface{ Signal() syscall.Signal }); ok && sys.Signal() != os.Kill {
			t.Errorf("cmd.ProcessState.Sys().Signal() = %v; want %v", sys.Signal(), os.Kill)
		}
	})

	t.Run("SIGINT-handled", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skipf("skipping: os.Interrupt is not implemented on Windows")
		}

		ctx, cancel := context.WithCancel(context.Background())
		cmd := start(t, ctx, os.Interrupt, 0, "-sleep=10m", "-interrupt=true")
		cancel()
		err := cmd.Wait()
		t.Logf("stderr:\n%s", cmd.Stderr)
		t.Logf("[%d] %v", cmd.Process.Pid, err)

		if !errors.Is(err, ctx.Err()) {
			t.Errorf("Wait error = %v; want %v", err, ctx.Err())
		}
		if ps := cmd.ProcessState; !ps.Exited() {
			t.Errorf("cmd did not exit: %v", ps)
		} else if code := ps.ExitCode(); code != 0 {
			t.Errorf("cmd.ProcessState.ExitCode() = %v; want 0", code)
		}
	})

	t.Run("SIGQUIT", func(t *testing.T) {
		if moreexec.QuitSignal == nil {
			t.Skipf("skipping: SIGQUIT is not supported on %v", runtime.GOOS)
		}

		ctx, cancel := context.WithCancel(context.Background())
		cmd := start(t, ctx, moreexec.QuitSignal, 0, "-sleep=10m")
		cancel()
		err := cmd.Wait()
		t.Logf("stderr:\n%s", cmd.Stderr)
		t.Logf("[%d] %v", cmd.Process.Pid, err)

		if ee := new(*exec.ExitError); !errors.As(err, ee) {
			t.Errorf("Wait error = %v; want %v", err, ctx.Err())
		}

		if ps := cmd.ProcessState; !ps.Exited() {
			t.Errorf("cmd did not exit: %v", ps)
		} else if code := ps.ExitCode(); code != 2 {
			// The default os/signal handler exits with code 2.
			t.Errorf("cmd.ProcessState.ExitCode() = %v; want 2", code)
		}

		if !strings.Contains(fmt.Sprint(cmd.Stderr), "\n\ngoroutine ") {
			t.Errorf("cmd.Stderr does not contain a goroutine dump")
		}
	})
}
