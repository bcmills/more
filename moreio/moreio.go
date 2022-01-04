// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package moreio contains plausible additions to the standard "io" package.
package moreio

import (
	"io"
)

func WriteByte(w io.Writer, c byte) error {
	bw, ok := w.(io.ByteWriter)
	if ok {
		return bw.WriteByte(c)
	}

	n, err := w.Write([]byte{c})
	if n < 1 && err == nil {
		return io.ErrShortWrite
	}
	return err
}

const utfMax = 4 // equal to utf8.UTFMax, but without importing utf8.

func WriteRune(w io.Writer, r rune) (n int, err error) {
	rw, ok := w.(interface {
		WriteRune(rune) (int, error)
	})
	if ok {
		return rw.WriteRune(r)
	}

	var arr [utfMax]byte
	size := copy(arr[:], string(r))
	n, err = w.Write(arr[:size])
	if n != size && err == nil {
		return n, io.ErrShortWrite
	}
	return n, err
}
