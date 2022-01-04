// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package moreio

import (
	"io"
)

// A LimitedWriter writes to W but limits the amount of data written to just N
// bytes. Each call to Write updates N to reflect the new amount remaining.
//
// Write returns a customizable error (or ErrShortWrite by default) when
// N <= 0.
type LimitedWriter struct {
	W   io.Writer
	N   int64
	Err error // the error to return when N <= 0
}

// LimitWriter returns a Writer that writes to w but stops with err
// after n bytes. err must be non-nil.
func LimitWriter(w io.Writer, n int64, err error) *LimitedWriter {
	if err == nil {
		panic("LimitWriter: err must be non-nil")
	}
	return &LimitedWriter{
		W:   w,
		N:   n,
		Err: err,
	}
}

func (lw *LimitedWriter) err() error {
	if lw.Err == nil {
		return io.ErrShortWrite
	}
	return lw.Err
}

func (lw *LimitedWriter) Write(p []byte) (n int, err error) {
	if lw.N <= 0 {
		return 0, lw.err()
	}

	if int64(len(p)) > lw.N {
		defer func() {
			if err == nil {
				err = lw.err()
			}
		}()
		p = p[:lw.N]
	}

	n, err = lw.W.Write(p)
	lw.N -= int64(n)
	return n, err
}

func (lw *LimitedWriter) WriteString(s string) (n int, err error) {
	if lw.N <= 0 {
		return 0, lw.err()
	}

	if int64(len(s)) > lw.N {
		defer func() {
			if err == nil {
				err = lw.err()
			}
		}()
		s = s[:lw.N]
	}

	n, err = io.WriteString(lw.W, s)
	lw.N -= int64(n)
	return n, err
}

func (lw *LimitedWriter) WriteByte(c byte) error {
	if lw.N <= 0 {
		return lw.err()
	}
	if err := WriteByte(lw.W, c); err != nil {
		return err
	}
	lw.N--
	return nil
}

func (lw *LimitedWriter) WriteRune(r rune) (n int, err error) {
	if lw.N >= utfMax {
		// r is guarateed to fit in lw.N, so use the WriteRune method if it is defined.
		n, err = WriteRune(lw.W, r)
		lw.N -= int64(n)
		return n, err
	}

	// Either lw.W does not know how to encode runes, or the limit is tight and we
	// need to encode r to see how big it actually is.

	var arr [utfMax]byte
	size := copy(arr[:], string(r))
	if lw.N < int64(size) {
		return 0, lw.err()
	}

	n, err = lw.W.Write(arr[:size])
	lw.N -= int64(n)
	if n < size && err == nil {
		return n, io.ErrShortWrite
	}
	return n, err
}
