// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package morebytes

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"unicode/utf8"
)

// ErrFileSizeLimit indicates that a File operation would
// cause its buffer's length to exceed its maximum capacity.
var ErrFileSizeLimit = errors.New("morebytes: File size limit exceeded")

// A File is an io.ReadWriteSeeker (like os.File) that reads, writes, and seeks
// within a slice of bytes. The slice backing the File may be either fixed or
// reallocated on demand; the zero File reallocates on demand.
//
// Like an os.File, a File has a current read/write offset and a size.
//
// The File may be resized (explicitly by Truncate or implicitly by Write or
// WriteAt), up to the capacity of the backing slice. Operations on a File with
// a fixed backing slice that would grow the file past the end of the slice fail
// with ErrFileCap.
//
// A File is a drop-in replacement for a bytes.Reader.
//
// File also provides a substantial subset of bytes.Buffer methods; however,
// unlike bytes.Buffer (but like os.File), the Read and Write methods on a File
// both modify the same offset. (A File is often a drop-in replacement for
// write-only uses of a bytes.Buffer.)
//
// File intentionally does not support certain bytes.Buffer methods:
//
// 	- It does not provide a Len method because it would be unclear whether Len
// 	  reports the length of the backing slice or the number of bytes remaining
// 	  to be read after the current offset. Use Size for the former, and
// 	  subtract Seek(0, io.SeekCurrent) for the latter.
//
// 	- It does not implement io.ReaderFrom because a File with a fixed backing
// 	  slice would not be able to detect io.EOF when the backing slice is exactly
// 	  full.
//
// 	- It does not provide a Grow method because a File with a fixed backing
// 	  slice can fail to grow beyond its capacity; instead, use Truncate, which
// 	  returns an explicit error.
//
// 	- It does not provide a nilladic Reset method because that would be
// 	  redundant with Truncate — the Reset([]byte) method from bytes.Reader is
// 	  strictly more useful.
//
type File struct {
	buf       []byte
	offset    int64 // distinct from len(buf) because Seek is explicitly allowed to set it to an arbitrary positive int64
	fixed     bool
	writeAtMu sync.RWMutex
}

const (
	intSize = 32 << (^uint(0) >> 63) // 32 or 64

	maxInt = 1<<(intSize-1) - 1 // math.MaxInt as of Go 1.17
)

// NewFile returns a new File initially backed by slice b.
//
// The maximum size of the File is unlimited, and the backing slice
// is reallocated and copied whenever a write would cause it to grow
// beyond its current capacity.
// The maximum size of the File is the capacity of the slice.
//
// The initial offset is 0, size is len(b), and capacity is cap(b).
func NewFile(b []byte) *File {
	f := new(File)
	f.Reset(b)
	return f
}

// NewFixedFile returns a new File backed by slice b.
//
// Like an os.File, the File has a current offset and size.
// The maximum size of the File is the capacity of its backing slice.
//
// The initial offset is 0, size is len(b), and capacity is cap(b).
func NewFixedFile(b []byte) *File {
	f := &File{fixed: true}
	f.Reset(b)
	return f
}

// Reset resets the writer to be backed by b, also resetting
// the current offset to 0, size to len(b), and capacity to cap(b).
func (f *File) Reset(b []byte) {
	*f = File{
		buf:   b,
		fixed: f.fixed,
	}
}

// Bytes returns the File's current backing data, independent of the current
// offset, with its length equal to the current size.
//
// Further writes to the File will continue to overwrite the underlying data,
// but not the length of the returned slice.
func (f *File) Bytes() []byte {
	return f.buf[:f.Size()]
}

// Cap returns the capacity of the File's underlying byte slice;
// that is, the size to which the File can grow without reallocating.
func (f *File) Cap() int {
	return cap(f.buf)
}

// SizeLimit returns the maximum allowed size of the File's data.
//
// The result can always be represented without overflow as an int:
// SizeLimit returns an int64 only to match the return type of Size.
func (f *File) SizeLimit() int64 {
	if f.fixed {
		return int64(cap(f.buf))
	}
	return int64(maxInt)
}

// Size returns the current size of the File's data.
//
// The result can always be represented without overflow as an int:
// Size returns an int64 only to mimic the API of bytes.Reader.
func (f *File) Size() int64 {
	return int64(len(f.buf))
}

// String returns the contents of the complete file (up to its size)
// as a string. If the *File is a nil pointer, it returns "<nil>".
func (f *File) String() string {
	if f == nil {
		return "<nil>" // mimic bytes.Buffer.String
	}
	return string(f.Bytes())
}

// Next returns the portion of the File's backing slice containing the next n
// bytes starting at the current offset, or nil if the current offset is greater
// than the capacity of the file, advancing the offset as if the bytes had been
// returned by Read. If there are fewer than n bytes between the current offset
// and size, Next returns whatever is available.
func (f *File) Next(n int) []byte {
	buf := f.next()
	if n > len(buf) {
		n = len(buf)
	}
	f.offset += int64(n)
	return buf[:n]
}

// next returns the portion of the backing store in the range [offset, size).
func (f *File) next() []byte {
	size := f.Size()
	if f.offset >= size {
		return nil
	}
	return f.buf[f.offset:size]
}

// Read implements the io.Reader interface.
func (f *File) Read(b []byte) (n int, err error) {
	buf := f.next()
	if len(buf) == 0 {
		return 0, io.EOF
	}
	n = copy(b, buf)
	f.offset += int64(n)
	return n, nil
}

// ReadByte implements the io.ByteReader interface.
func (f *File) ReadByte() (byte, error) {
	buf := f.next()
	if len(buf) < 1 {
		return 0, io.EOF
	}
	b := buf[0]
	f.offset += 1
	return b, nil
}

// UnreadByte implements the io.ByteScanner interface.
func (f *File) UnreadByte() error {
	if f.offset <= 0 {
		return errors.New("UnreadByte: no bytes to unread")
	}
	f.offset -= 1
	return nil
}

// ReadRune implements the io.RuneReader interface.
func (f *File) ReadRune() (r rune, rSize int, err error) {
	buf := f.next()
	if len(buf) < 1 {
		return 0, 0, io.EOF
	}
	r, rSize = utf8.DecodeRune(buf)
	f.offset += int64(rSize)
	return r, rSize, nil
}

// UnreadRune implements the io.RuneScanner interface.
func (f *File) UnreadRune() error {
	if f.offset == 0 {
		return errors.New("UnreadRune: no runes to unread")
	}
	_, n := utf8.DecodeLastRune(f.buf[:f.offset])
	f.offset -= int64(n)
	return nil
}

// ReadAt implements the io.ReaderAt interface.
func (f *File) ReadAt(b []byte, off int64) (n int, err error) {
	if off < 0 {
		return 0, errors.New("ReadAt: invalid offset")
	}

	size := f.Size()
	if off >= size {
		return 0, io.EOF
	}
	n = copy(b, f.buf[off:size])
	if n < len(b) {
		return n, io.EOF
	}
	return n, nil
}

// ReadBytes reads until the next occurrence of delim in the input,
// returning a copy of the data up to and including the delimiter.
// If ReadBytes encounters the end of the file before finding the delimiter,
// it returns the data read before the error and io.EOF.
// ReadBytes returns err != nil if and only if the returned data does not end in
// delim.
func (f *File) ReadBytes(delim byte) (line []byte, err error) {
	slice, err := f.readSlice(delim)
	// For compatibility with bytes.Buffer.ReadBytes, make a copy of the slice.
	// It's perhaps needlessly inefficient in the common case (where the returned
	// slice is not modified), but also less prone to aliasing bugs in the
	// uncommon cases.
	return append([]byte(nil), slice...), err
}

// ReadStrings reads until the next occurrence of delim in the input,
// returning a string containing the data up to and including the delimiter.
// If ReadString encounters the end of the file before finding the delimiter,
// it returns the data read before the error and io.EOF.
// ReadBytes returns err != nil if and only if the returned data does not end in
// delim.
func (f *File) ReadString(delim byte) (line string, err error) {
	slice, err := f.readSlice(delim)
	return string(slice), err
}

func (f *File) readSlice(delim byte) (line []byte, err error) {
	buf := f.next()
	i := bytes.IndexByte(buf, delim)
	if i >= 0 {
		buf = buf[:i+1]
	} else {
		err = io.EOF
	}
	f.offset += int64(len(buf))
	return line, err
}

// WriteTo implements the io.WriterTo interface.
func (f *File) WriteTo(w io.Writer) (n int64, err error) {
	b := f.next()
	if len(b) == 0 {
		return 0, nil
	}

	dn, err := w.Write(b)
	n = int64(dn)
	f.offset += n
	if n < int64(len(b)) && err == nil {
		return n, io.ErrShortWrite
	}
	return n, err
}

// Seek sets the offset for the next Read or Write to offset, interpreted
// according to whence: SeekStart means relative to the start of the file,
// SeekCurrent means relative to the current offset, and SeekEnd means relative
// to the end. Seek returns the new offset relative to the start of the file and
// an error, if any.
func (f *File) Seek(offset int64, whence int) (ret int64, err error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		if offset < 0 {
			return 0, errors.New("Seek: invalid offset")
		}
		abs = offset
	case io.SeekCurrent:
		if offset < -int64(f.offset) {
			return 0, errors.New("Seek: invalid offset")
		}
		abs = int64(f.offset) + offset
	case io.SeekEnd:
		size := f.Size()
		if offset < -size {
			return 0, errors.New("Seek: invalid offset")
		}
		abs = size + offset
	default:
		return 0, errors.New("Seek: invalid whence")
	}

	// We already checked that the offset should not be negative through
	// non-overflowing addition, so a negative absolute offset indicates overflow.
	//
	// The io.Seeker interface requires that Seek accept “any positive offset”,
	// but we can at least reject the negative ones.
	if abs < 0 {
		return 0, errors.New("Seek: offset overflows int64")
	}

	f.offset = abs
	return f.offset, nil
}

// Truncate changes the size of the File.
// It does not change the offset or allocate a new backing slice.
//
// If the indicated size is larger than f's size limit,
// Truncate returns ErrFileSizeLimit and leaves the size unchanged.
func (f *File) Truncate(size int64) error {
	if size < 0 {
		return errors.New("Truncate: negative size")
	}
	if size > f.SizeLimit() {
		return ErrFileSizeLimit
	}
	if growth := int(size) - len(f.buf); growth > 0 {
		// To provide the same semantics as os.File.Truncate, sero-fill the trailing
		// bytes of f.buf even if we don't have to reallocate it.
		f.buf = append(f.buf, make([]byte, growth)...)
	}
	f.buf = f.buf[:size]
	return nil
}

// Write writes len(b) bytes to the File.
//
// If the new offset is higher than the previous size of the File
// Write updates the size to be equal to the new offset.
//
// If the new size would exceed f's size limit, Write updates the length and
// offset to be equal to the limit and writes as many bytes as will fit, and
// returns the number of bytes actually written along with ErrFileSizeLimit.
func (f *File) Write(b []byte) (n int, err error) {
	buf, err := f.growAt(f.offset, 0, len(b))
	if err != nil {
		return 0, err
	}
	n = copy(buf, b)
	f.offset += int64(n)
	if n < len(b) {
		return n, ErrFileSizeLimit
	}
	return n, nil
}

// WriteByte implements the io.ByteWriter interface.
func (f *File) WriteByte(c byte) error {
	buf, err := f.growAt(f.offset, 1, 1)
	if err != nil {
		return err
	}
	if len(buf) < 1 {
		return ErrFileSizeLimit
	}
	buf[0] = c
	f.offset += 1
	return nil
}

// WriteRune implements the io.RuneWriter interface.
func (f *File) WriteRune(r rune) (n int, err error) {
	var arr [utf8.UTFMax]byte
	n = utf8.EncodeRune(arr[:], r)
	buf, err := f.growAt(f.offset, n, n)
	if err != nil {
		return 0, err
	}
	copy(buf[:n], arr[:n])
	f.offset += int64(n)
	return n, nil
}

// WriteString is like Write, but writes the contents of string s rather than a
// slice of bytes.
func (f *File) WriteString(s string) (n int, err error) {
	buf, err := f.growAt(f.offset, 0, len(s))
	if err != nil {
		return 0, err
	}
	n = copy(buf, s)
	f.offset += int64(n)
	if n < len(s) {
		return n, ErrFileSizeLimit
	}
	return n, nil
}

// WriteAt writes len(b) bytes to the File at the indicated offset.
//
// If the highest offset to be written is higher than the current size of the
// file, WriteAt updates the size to be equal to the highest offset
// or the maximum capacity of the backing slice, whichever is smaller.
//
// If any byte to be written is above f's size limit, WriteAt writes any bytes
// that do fit within the limit and returns the number of bytes written along
// with ErrFileSizeLimit.
func (f *File) WriteAt(b []byte, offset int64) (n int, err error) {
	n = len(b)

	// os.File.WriteAt implicitly grows the file to the maximum offset written.
	// We want to do the same here, but growing a slice means reallocating it,
	// and we don't want to drop the data from concurrent WriteAt calls.
	// So we at least need to lock the File enough to prevent a new buffer from
	// being allocated while the old one is still being written to.
	f.writeAtMu.RLock()
	if int64(len(f.buf)-n) < offset {
		f.writeAtMu.RUnlock()
		f.writeAtMu.Lock()
		// When we drop the write-lock, f.buf may grow again (invalidating
		// references to the buffer) before we can reacquire a read-lock.
		// Record only the limit on the number of bytes to be written.
		buf, err := f.growAt(offset, 0, len(b))
		n = len(buf)
		f.writeAtMu.Unlock()

		if err != nil {
			return 0, err
		}
		f.writeAtMu.RLock()
	}
	copy(f.buf[offset:][:n], b[:n])
	f.writeAtMu.RUnlock()

	if n < len(b) {
		return 0, ErrFileSizeLimit
	}
	return n, err
}

// growAt grows f's backing array so that it can hold up to maxN bytes,
// or as close to that as is allowed by f's size limit.
//
// growAt returns the subslice of up to maxN bytes beginning at offset.
func (f *File) growAt(offset int64, minN, maxN int) (buf []byte, err error) {
	if int64(len(f.buf))-offset >= int64(maxN) {
		return f.buf[offset:][:maxN], nil
	}

	n := f.SizeLimit() - offset
	if n < int64(minN) {
		return nil, ErrFileSizeLimit
	}
	if n > int64(maxN) {
		n = int64(maxN)
	}

	size := int(offset + n)
	if cap(f.buf) >= size {
		f.buf = f.buf[:size]
	} else {
		f.buf = append(f.buf, make([]byte, size-len(f.buf))...)
	}
	return f.buf[offset:size], nil
}
