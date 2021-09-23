// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package moreatomic contains plausible additions to the standard "sync/atomic" package.
package moreatomic

import (
	"sync/atomic"
	"unsafe"
)

func AddInt(addr *int, delta int) (new int) {
	switch unsafe.Sizeof(*addr) {
	case 4:
		return int(atomic.AddInt32((*int32)(unsafe.Pointer(addr)), int32(delta)))
	case 8:
		return int(atomic.AddInt64((*int64)(unsafe.Pointer(addr)), int64(delta)))
	default:
		panic("int is neither 4 nor 8 bytes")
	}
}

func CompareAndSwapInt(addr *int, old, new int) (swapped bool) {
	switch unsafe.Sizeof(*addr) {
	case 4:
		return atomic.CompareAndSwapInt32((*int32)(unsafe.Pointer(addr)), int32(old), int32(new))
	case 8:
		return atomic.CompareAndSwapInt64((*int64)(unsafe.Pointer(addr)), int64(old), int64(new))
	default:
		panic("int is neither 4 nor 8 bytes")
	}
}

func LoadInt(addr *int) (val int) {
	switch unsafe.Sizeof(*addr) {
	case 4:
		return int(atomic.LoadInt32((*int32)(unsafe.Pointer(addr))))
	case 8:
		return int(atomic.LoadInt64((*int64)(unsafe.Pointer(addr))))
	default:
		panic("int is neither 4 nor 8 bytes")
	}
}

func StoreInt(addr *int, val int) {
	switch unsafe.Sizeof(*addr) {
	case 4:
		atomic.StoreInt32((*int32)(unsafe.Pointer(addr)), int32(val))
	case 8:
		atomic.StoreInt64((*int64)(unsafe.Pointer(addr)), int64(val))
	default:
		panic("int is neither 4 nor 8 bytes")
	}
}

func SwapInt(addr *int, new int) (old int) {
	switch unsafe.Sizeof(*addr) {
	case 4:
		return int(atomic.SwapInt32((*int32)(unsafe.Pointer(addr)), int32(new)))
	case 8:
		return int(atomic.SwapInt64((*int64)(unsafe.Pointer(addr)), int64(new)))
	default:
		panic("int is neither 4 nor 8 bytes")
	}
}

func AddUint(addr *uint, delta uint) (new uint) {
	switch unsafe.Sizeof(*addr) {
	case 4:
		return uint(atomic.AddUint32((*uint32)(unsafe.Pointer(addr)), uint32(delta)))
	case 8:
		return uint(atomic.AddUint64((*uint64)(unsafe.Pointer(addr)), uint64(delta)))
	default:
		panic("uint is neither 4 nor 8 bytes")
	}
}

func CompareAndSwapUint(addr *uint, old, new uint) (swapped bool) {
	switch unsafe.Sizeof(*addr) {
	case 4:
		return atomic.CompareAndSwapUint32((*uint32)(unsafe.Pointer(addr)), uint32(old), uint32(new))
	case 8:
		return atomic.CompareAndSwapUint64((*uint64)(unsafe.Pointer(addr)), uint64(old), uint64(new))
	default:
		panic("uint is neither 4 nor 8 bytes")
	}
}

func LoadUint(addr *uint) (val uint) {
	switch unsafe.Sizeof(*addr) {
	case 4:
		return uint(atomic.LoadUint32((*uint32)(unsafe.Pointer(addr))))
	case 8:
		return uint(atomic.LoadUint64((*uint64)(unsafe.Pointer(addr))))
	default:
		panic("uint is neither 4 nor 8 bytes")
	}
}

func StoreUint(addr *uint, val uint) {
	switch unsafe.Sizeof(*addr) {
	case 4:
		atomic.StoreUint32((*uint32)(unsafe.Pointer(addr)), uint32(val))
	case 8:
		atomic.StoreUint64((*uint64)(unsafe.Pointer(addr)), uint64(val))
	default:
		panic("uint is neither 4 nor 8 bytes")
	}
}

func SwapUint(addr *uint, new uint) (old uint) {
	switch unsafe.Sizeof(*addr) {
	case 4:
		return uint(atomic.SwapUint32((*uint32)(unsafe.Pointer(addr)), uint32(new)))
	case 8:
		return uint(atomic.SwapUint64((*uint64)(unsafe.Pointer(addr)), uint64(new)))
	default:
		panic("uint is neither 4 nor 8 bytes")
	}
}
