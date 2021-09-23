// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package moreio_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/bcmills/more/moreio"
)

var errArbitrary = errors.New("arbitrary error")

func TestLimitedWriterWriteLimits(t *testing.T) {
	b := new(strings.Builder)
	w := moreio.LimitWriter(b, 9, errArbitrary)
	t.Logf(`w := moreio.LimitWriter(b, 9, errArbitrary)`)

	n, err := w.Write([]byte("Hello"))
	t.Logf(`w.Write("Hello") = %v, %v`, n, err)
	if n != 5 || err != nil {
		t.Fatalf("want 5, <nil>")
	}

	n, err = w.Write([]byte(", moreio!"))
	t.Logf(`w.Write(", moreio!") = %v, %v`, n, err)
	if n != 4 || err != errArbitrary {
		t.Fatalf("want 3, errArbitrary")
	}

	n, err = w.Write([]byte("Hello, again!"))
	t.Logf(`w.Write("Hello, again!") = %v, %v`, n, err)
	if n != 0 || err != errArbitrary {
		t.Fatalf("want 0, errArbitrary")
	}

	if b.String() != "Hello, mo" {
		t.Fatalf(`output = %q; want "Hello, mo"`, b.String())
	}
}

func TestLimitedWriterWriteZeroValue(t *testing.T) {
	var w moreio.LimitedWriter
	n, err := w.Write([]byte("Hello, moreio!"))
	if n != 0 || err != io.ErrShortWrite {
		t.Fatalf(`Write("Hello, moreio!") = %v, %v; want 0, ErrShortWrite`, n, err)
	}

	n, err = w.Write(nil)
	if n != 0 || err != io.ErrShortWrite {
		t.Fatalf(`Write(nil) = %v, %v; want 0, ErrShortWrite`, n, err)
	}
}

func TestLimitedWriterWriteStringLimits(t *testing.T) {
	b := new(strings.Builder)
	w := moreio.LimitWriter(b, 9, errArbitrary)
	t.Logf(`w := moreio.LimitWriter(b, 9, errArbitrary)`)

	n, err := w.WriteString("Hello")
	t.Logf(`w.WriteString("Hello") = %v, %v`, n, err)
	if n != 5 || err != nil {
		t.Fatalf("want 5, <nil>")
	}

	n, err = w.WriteString(", moreio!")
	t.Logf(`w.WriteString(", moreio!") = %v, %v`, n, err)
	if n != 4 || err != errArbitrary {
		t.Fatalf("want 3, errArbitrary")
	}

	n, err = w.WriteString("Hello, again!")
	t.Logf(`w.WriteString("Hello, again!") = %v, %v`, n, err)
	if n != 0 || err != errArbitrary {
		t.Fatalf("want 0, errArbitrary")
	}

	if b.String() != "Hello, mo" {
		t.Fatalf(`output = %q; want "Hello, mo"`, b.String())
	}
}

func TestLimitedWriterWriteStringZeroValue(t *testing.T) {
	var w moreio.LimitedWriter
	n, err := w.WriteString("Hello, moreio!")
	if n != 0 || err != io.ErrShortWrite {
		t.Fatalf(`WriteString("Hello, moreio!") = %v, %v; want 0, ErrShortWrite`, n, err)
	}

	n, err = w.WriteString("")
	if n != 0 || err != io.ErrShortWrite {
		t.Fatalf(`WriteString("") = %v, %v; want 0, ErrShortWrite`, n, err)
	}
}
