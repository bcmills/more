// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || zos || windows)
// +build !aix,!darwin,!dragonfly,!freebsd,!linux,!netbsd,!openbsd,!solaris,!zos,!windows

package moreexec

import "os"

var quitSignal os.Signal = nil

var errWindows error = nil
