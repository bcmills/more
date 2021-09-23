// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package morebytes_test

import (
	"fmt"
	"io"
	"sync"

	"github.com/bcmills/more/morebytes"
)

func ExampleNewFixedFile() {
	// morebytes.NewFixedFile can be used in conjunction with fmt.Fprintf
	// to get something like C's snprintf.

	w := morebytes.NewFixedFile(make([]byte, 5))
	fmt.Fprintf(w, "Hello, world!")
	fmt.Printf("%q\n", w.Bytes())

	// Output:
	// "Hello"
}

func ExampleNewFixedFile_suffix() {
	// To adjust where writing starts within a slice, either wrap the suffix to be
	// overwritten, or explicitly Seek to the desired offset.

	buf := []byte("Hello, world!")

	w := morebytes.NewFixedFile(buf[7:])
	w.WriteString("gopher and others")
	fmt.Printf("%q\n", buf)

	w2 := morebytes.NewFile(buf)
	w2.Seek(3, io.SeekStart)
	w2.WriteString("per")
	fmt.Printf("%q\n", buf)

	// Output:
	// "Hello, gopher"
	// "Helper gopher"
}

func ExampleFile_Truncate() {
	// The Truncate method works just like os.File.Truncate:
	// it changes the size of the buffer, but doesn't change
	// its offset or contents.

	buf := []byte("Hello, world!")

	w := morebytes.NewFile(buf)
	fmt.Printf("%q\n", w.Bytes())

	w.Truncate(5)
	fmt.Printf("%q\n", w.Bytes())

	// Output:
	// "Hello, world!"
	// "Hello"
}

func ExampleFile_WriteAt() {
	// WriteAt can be called concurrently, and the buffer is resized to
	// accommodate that highest offset written.

	w := morebytes.NewFile(make([]byte, 0, 14))
	fmt.Printf("%q\n", w.Bytes())

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		w.WriteAt([]byte("Hello, "), 0)
		wg.Done()
	}()
	go func() {
		w.WriteAt([]byte("world!"), 7)
		wg.Done()
	}()
	wg.Wait()

	fmt.Printf("%q\n", w.Bytes())

	// Output:
	// ""
	// "Hello, world!"
}
