// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package moreexec_test

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strings"
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

func TestMain(m *testing.M) {
	flag.Parse()

	if *probe > 0 {
		ticker := time.NewTicker(*probe)
		go func() {
			for range ticker.C {
				if _, err := fmt.Fprintln(os.Stderr, "ok"); err != nil {
					os.Exit(1)
				}
			}
		}()
	}

	if *subsleep != 0 {
		exe, err := os.Executable()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		cmd := moreexec.Command(exe, "-sleep", subsleep.String(), "-probe", "1ms")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Start()
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
		case <-c:
		}
	}

	if *probe != 0 || *subsleep != 0 || *sleep != 0 {
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func start(t *testing.T, ctx context.Context, interrupt os.Signal, killDelay time.Duration, args ...string) *moreexec.Cmd {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	cmd := moreexec.CommandContext(ctx, exe, args...)
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

func TestWait(t *testing.T) {
	ctx := context.Background()

	t.Run("Wait", func(t *testing.T) {
		cmd := start(t, ctx, os.Interrupt, 0, "-sleep=1ms")

		if err := cmd.Wait(); err != nil {
			t.Errorf("Wait: %v; want <nil>", err)
		}
		t.Logf("stderr:\n%s", cmd.Stderr)
		if ps := cmd.ProcessState; !ps.Exited() {
			t.Errorf("cmd did not exit: %v", ps)
		} else if code := ps.ExitCode(); code != 0 {
			t.Errorf("cmd.ProcessState.ExitCode() = %v; want 0", code)
		}
	})

	t.Run("hang", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		cmd := start(t, ctx, nil, 10*time.Millisecond, "-subsleep=1s", "-probe=1ms")

		cancel()
		if err := cmd.Wait(); err != ctx.Err() {
			t.Errorf("Wait: %v; want %v", err, ctx.Err())
			t.Logf("%v\n%s", cmd.Args, cmd.Stderr)
		}
		if ps := cmd.ProcessState; !ps.Exited() {
			t.Errorf("cmd did not exit")
		} else if code := ps.ExitCode(); code != 0 {
			t.Errorf("cmd.ProcessState.ExitCode() = %v; want 0", code)
		}
	})

	t.Run("SIGINT-ignored", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		cmd := start(t, ctx, os.Interrupt, 100*time.Millisecond, "-sleep=10m", "-interrupt=false")

		cancel()
		err := cmd.Wait()
		if err != ctx.Err() {
			if runtime.GOOS == "windows" {
				// We expect Wait with os.Interrupt to error out on Windows
				// because Windows does not implement os.Interrupt.
				t.Logf("Wait error = %v (as expected on Windows)", err)
			} else {
				t.Errorf("Wait error = %v; want %v", err, ctx.Err())
			}
		}
		t.Logf("stderr:\n%s", cmd.Stderr)
		if ps := cmd.ProcessState; ps.Exited() {
			t.Errorf("cmd unexpectedly exited: %v", ps)
		} else if ps.Success() {
			t.Errorf("cmd.ProcessState.Success() = true; want false")
		} else if sys, ok := ps.Sys().(interface{ Signal() syscall.Signal }); ok && sys.Signal() != os.Kill {
			t.Errorf("cmd.ProcessState.Sys().Signal() = %v; want %v", sys.Signal(), os.Kill)
		}
	})

	t.Run("SIGINT-handled", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skipf("skipping: os.Interrupt is not implemented on Windows")
		}

		ctx, cancel := context.WithCancel(ctx)
		cmd := start(t, ctx, os.Interrupt, 0, "-sleep=10m", "-interrupt=true")

		cancel()
		err := cmd.Wait()
		if err != ctx.Err() {
			t.Errorf("Wait error = %v; want %v", err, ctx.Err())
		}
		t.Logf("stderr:\n%s", cmd.Stderr)

		if ps := cmd.ProcessState; !ps.Exited() {
			t.Errorf("cmd did not exit: %v", ps)
		} else if code := ps.ExitCode(); code != 0 {
			t.Errorf("cmd.ProcessState.ExitCode() = %v; want 0", code)
		}
	})

	t.Run("SIGQUIT", func(t *testing.T) {
		if moreexec.QuitSignal == os.Kill {
			t.Skipf("skipping: SIGQUIT is not supported on %v", runtime.GOOS)
		}

		ctx, cancel := context.WithCancel(ctx)
		cmd := start(t, ctx, moreexec.QuitSignal, 0, "-sleep=10m")

		cancel()
		err := cmd.Wait()
		if err != ctx.Err() {
			t.Errorf("Wait error = %v; want %v", err, ctx.Err())
		}
		t.Logf("stderr:\n%s", cmd.Stderr)

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
