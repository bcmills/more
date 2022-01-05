// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package moreexec_test

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"

	"github.com/bcmills/more/os/moreexec"
)

func TestStartRejectsUnsupportedInterrupt(t *testing.T) {
	for _, sig := range []os.Signal{
		os.Interrupt, // explicitly not implemented

		// “invented values” as described by the syscall package.
		syscall.SIGHUP,
		syscall.SIGQUIT,

		// Note that os.Kill actually is supported, and is tested separately.
	} {
		t.Run(sig.String(), func(t *testing.T) {
			cmd := moreexec.CommandContext(context.Background(), exePath(), "-sleep=1ms")
			cmd.Interrupt = sig
			err := cmd.Start()

			if err == nil {
				t.Errorf("Start succeeded unexpectedly")
				cmd.Wait()
			} else if !errors.Is(err, syscall.EWINDOWS) {
				t.Errorf("Start: %v\nwant %v", err, syscall.EWINDOWS)
			}
		})
	}
}
