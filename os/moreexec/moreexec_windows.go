// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package moreexec

import (
	"os"
	"syscall"
)

var quitSignal os.Signal = nil

var errWindows error = syscall.EWINDOWS
