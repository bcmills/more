// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build go1.18

package morebytes_test

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
	"unicode/utf8"

	"github.com/bcmills/more/morebytes"
	"github.com/bcmills/more/moreio"
)

// A bufferFile mimics a subset of the behavior of a File using a bytes.Buffer
// and moreio.LimitedReader.
type bufferFile struct {
	MinSize int
	*bytes.Buffer
	moreio.LimitedWriter
}

func (f *bufferFile) Reset(b []byte) {
	f.MinSize = len(b)
	f.Buffer = bytes.NewBuffer(b[:0])
	f.LimitedWriter = moreio.LimitedWriter{
		W:   f.Buffer,
		N:   int64(cap(b)),
		Err: morebytes.ErrFileSizeLimit,
	}
}

func (f *bufferFile) Write(b []byte) (int, error) {
	return f.LimitedWriter.Write(b)
}

func (f *bufferFile) WriteString(s string) (int, error) {
	return f.LimitedWriter.WriteString(s)
}

func (f *bufferFile) WriteByte(c byte) error {
	return f.LimitedWriter.WriteByte(c)
}

func (f *bufferFile) WriteRune(r rune) (int, error) {
	return f.LimitedWriter.WriteRune(r)
}

func (f *bufferFile) Bytes() []byte {
	b := f.Buffer.Bytes()
	if len(b) < f.MinSize {
		b = b[:f.MinSize]
	}
	return b
}

// newAwkwardWriter returns an io.Writer that is like a morebytes.Writer
// but doesn't allow seeking or truncation.
func newAwkwardWriter(b []byte) *bufferFile {
	w := new(bufferFile)
	w.Reset(b)
	return w
}

func FuzzFixedFileWrite(f *testing.F) {
	hello := []byte("Hello, fuzzer!")
	f.Add(0, 0, hello)
	f.Add(0, 5, hello)
	f.Add(0, 50, hello)
	f.Add(5, 5, hello)
	f.Add(5, 50, hello)
	f.Add(50, 50, hello)

	f.Fuzz(func(t *testing.T, bLen, bCap int, in []byte) {
		if bLen < 0 {
			bLen = -(bLen + 1)
		}
		if bCap < 0 {
			bCap = -(bCap + 1)
		}

		bCap &= 2 << 10 // 2 KiB max
		if bLen > bCap {
			bLen &= bCap
		}

		t.Logf("make([]byte, %d, %d)", bLen, bCap)

		b1 := make([]byte, bLen, bCap)
		w1 := morebytes.NewFixedFile(b1)

		b2 := make([]byte, bLen, bCap)
		w2 := newAwkwardWriter(b2)

		for _, b := range bytes.Split(in, []byte{0}) {
			switch {
			case len(b) == 0:
				fallthrough
			default:
				n1, err1 := w1.Write(b)
				t.Logf("%T:\tWrite(%d bytes) = %v, %v", w1, len(b), n1, err1)
				n2, err2 := w2.Write(b)
				if n1 != n2 || (err1 != err2 && len(b) > 0) {
					t.Fatalf("%T:\tWrite(%d bytes) = %v, %v", w2, len(b), n2, err2)
				}

			case len(b) == 1:
				c := b[0]
				err1 := w1.WriteByte(c)
				t.Logf("%T:\tWriteByte(0x%x) = %v", w1, c, err1)
				err2 := w2.WriteByte(c)
				if err2 != err1 {
					t.Fatalf("%T:\tWriteByte(0x%x) = %v", w2, c, err2)
				}

			case len(b) <= utf8.UTFMax && utf8.FullRune(b):
				r, _ := utf8.DecodeRune(b)
				n1, err1 := w1.WriteRune(r)
				t.Logf("%T:\tWriteRune(0x%x) = %v, %v", w1, r, n1, err1)
				n2, err2 := w2.WriteRune(r)
				if n2 != n1 || err2 != err1 {
					t.Fatalf("%T:\tWriteRune(0x%x) = %v, %v", w2, r, n2, err2)
				}

			case b[0]&0x1 != 0:
				s := string(b)
				n1, err1 := io.WriteString(w1, s)
				t.Logf("%T:\tWriteString(%d bytes) = %v, %v", w1, len(s), n1, err1)
				n2, err2 := io.WriteString(w2, s)
				if n2 != n1 || err2 != err1 {
					t.Fatalf("%T:\tWriteString(%d bytes) = %v, %v", w2, len(s), n2, err2)
				}
			}

			if c1, c2 := w1.Cap(), w2.Cap(); c2 != c1 {
				t.Fatalf("Cap not equal.\n%T:\t%v\n%T:\t%v", w1, c1, w2, c2)
			}

			out1 := w1.Bytes()
			out2 := w2.Bytes()
			if !bytes.Equal(out2, out1) {
				t.Fatalf("Contents not equal.\n%T:\t%d bytes\n%T:\t%d bytes", w1, len(out1), w2, len(out2))
			}
		}
	})
}

func FuzzFileRead(f *testing.F) {
	f.Fuzz(func(t *testing.T, in, opBytes []byte) {
		r1 := morebytes.NewFile(in)
		buf1 := make([]byte, 1<<16)
		r2 := bytes.NewReader(in)
		buf2 := make([]byte, 1<<16)
		ops := bytes.NewReader(opBytes)

		afterReadRune := false
		didReadRune := false
		for {
			afterReadRune, didReadRune = didReadRune, false

			var op [2]byte
			if _, err := ops.Read(op[:]); err == io.EOF {
				break
			} else if err != nil {
				t.Fatal(err)
			}
			switch string(op[:]) {
			case "s0":
			case "s1":
			case "s2":

			case "rb":
				c1, err1 := r1.ReadByte()
				t.Logf("%T:\tReadByte(): %q, %v", r1, c1, err1)
				c2, err2 := r2.ReadByte()
				if c2 != c1 || err2 != err1 {
					t.Fatalf("%T:\tReadByte(): %q, %v", r2, c2, err2)
				}

			case "rr":
				c1, n1, err1 := r1.ReadRune()
				t.Logf("%T:\tReadRune(): %q, %v, %v", r1, c1, n1, err1)
				c2, n2, err2 := r2.ReadRune()
				if c2 != c1 || n2 != n1 || err2 != err1 {
					t.Fatalf("%T:\tReadByte(): %q, %v, %v", r2, c2, n2, err2)
				}
				if err1 == nil {
					didReadRune = true
				}

			case "ub":
				err1 := r1.UnreadByte()
				t.Logf("%T:\tUnreadByte(): %v", r1, err1)
				err2 := r2.UnreadByte()
				if (err2 == nil) != (err1 == nil) {
					t.Fatalf("%T:\tUnreadByte(): %v", r2, err2)
				}

			case "ur":
				if !afterReadRune {
					c1, n1, err1 := r1.ReadRune()
					t.Logf("%T:\tReadRune(): %q, %v, %v", r1, c1, n1, err1)
					c2, n2, err2 := r2.ReadRune()
					if c2 != c1 || n2 != n1 || err2 != err1 {
						t.Fatalf("%T:\tReadByte(): %q, %v, %v", r2, c2, n2, err2)
					}
					if err1 != nil {
						continue // Not after a successful ReadRune.
					}
				}
				err1 := r1.UnreadRune()
				t.Logf("%T:\tUnreadRune(): %v", r1, err1)
				err2 := r2.UnreadRune()
				if (err2 == nil) != (err1 == nil) {
					t.Fatalf("%T:\tUnreadRune(): %v", r2, err2)
				}

			default:
				n := binary.LittleEndian.Uint16(op[:])
				n1, err1 := r1.Read(buf1[:n])
				t.Logf("%T:\tRead(_[:%d]): %v, %v", r1, n, n1, err1)
				n2, err2 := r2.Read(buf2[:n])
				if n2 != n1 || err2 != err1 {
					t.Fatalf("%T:\tRead(_[:%d]): %v, %v", r2, n, n2, err2)
				}
				if !bytes.Equal(buf1[:n], buf2[:n]) {
					t.Fatalf("Contents not equal.\n%T:\t%q\n%T:\t%q", r1, buf1[:n], r2, buf2[:n])
				}
			}
		}
	})
}
