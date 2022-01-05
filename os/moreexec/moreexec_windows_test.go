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

func TestStartRejectsInterruptOnWindows(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	cmd := moreexec.CommandContext(context.Background(), exe, "-sleep=1ms")
	cmd.Interrupt = os.Interrupt
	err = cmd.Start()
	if err == nil {
		t.Errorf("Start succeeded unexpectedly")
		cmd.Wait()
	} else if !errors.Is(err, syscall.EWINDOWS) {
		t.Errorf("Start: %v\nwant %v", err, syscall.EWINDOWS)
	}
}
