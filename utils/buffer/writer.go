package buffer

import (
	"encoding/binary"
	"fmt"
	"unsafe"
)

// WriteInt writes an int c to w.
func WriteInt(w Writer, c int) (n int64, err error) {
	nint, err := WriteUint64(w, uint64(c))
	return int64(nint), err
}

// WriteUint8 writes a byte c to w.
func WriteUint8(w Writer, c uint8) (n int64, err error) {

	if w.Available() == 0 {
		if err = w.Flush(); err != nil {
			return
		}

		if w.Available() == 0 {
			return 0, fmt.Errorf("cannot WriteUint8: available buffer is zero even after flush")
		}
	}

	nint, err := w.Write([]byte{c})

	return int64(nint), err
}

// WriteUint8Slice writes a slice of bytes c to w.
func WriteUint8Slice(w Writer, c []uint8) (n int64, err error) {

	if len(c) == 0 {
		return
	}

	// Remaining available space in the internal buffer
	available := w.Available()

	if available == 0 {

		if err = w.Flush(); err != nil {
			return
		}

		available = w.Available()

		if available == 0 {
			return 0, fmt.Errorf("cannot WriteUint8Slice: available buffer/2 is zero even after flush")
		}
	}

	buf := w.AvailableBuffer()

	if N := len(c); N <= available { // If there is enough space in the available buffer
		buf = buf[:N]

		copy(buf, c)

		nint, err := w.Write(buf)

		return int64(nint), err
	}

	// First fills the space
	buf = buf[:available]
	
	copy(buf, c)

	var inc int
	if inc, err = w.Write(buf); err != nil {
		return n + int64(inc), err
	}

	n += int64(inc)

	// Flushes
	if err = w.Flush(); err != nil {
		return n, err
	}

	// Then recurses on itself with the remaining slice
	var inc64 int64
	inc64, err = WriteUint8Slice(w, c[available:])

	return n + inc64, err
}

// WriteUint16 writes a uint16 c to w.
func WriteUint16(w Writer, c uint16) (n int64, err error) {

	if w.Available()>>1 == 0 {
		if err = w.Flush(); err != nil {
			return
		}

		if w.Available()>>1 == 0 {
			return 0, fmt.Errorf("cannot WriteUint16: available buffer/2 is zero even after flush")
		}
	}

	buf := w.AvailableBuffer()[:2]

	binary.LittleEndian.PutUint16(buf, c)

	nint, err := w.Write(buf)

	return int64(nint), err
}

// WriteUint16Slice writes a slice of uint16 c to w.
func WriteUint16Slice(w Writer, c []uint16) (n int64, err error) {

	if len(c) == 0 {
		return
	}

	// Remaining available space in the internal buffer
	available := w.Available() >> 1

	if available == 0 {
		if err = w.Flush(); err != nil {
			return
		}

		available = w.Available() >> 1

		if available == 0 {
			return 0, fmt.Errorf("cannot WriteUint16Slice: available buffer/2 is zero even after flush")
		}
	}

	buf := w.AvailableBuffer()

	if N := len(c); N <= available { // If there is enough space in the available buffer
		buf = buf[:N<<1]
		for i := 0; i < N; i++ {
			binary.LittleEndian.PutUint16(buf[i<<1:], c[i])
		}

		nint, err := w.Write(buf)

		return int64(nint), err
	}

	buf = buf[:available<<1]

	// First fills the space
	for i := 0; i < available; i++ {
		binary.LittleEndian.PutUint16(buf[i<<1:], c[i])
	}

	var inc int
	if inc, err = w.Write(buf); err != nil {
		return n + int64(inc), err
	}

	n += int64(inc)

	// Flushes
	if err = w.Flush(); err != nil {
		return n, err
	}

	// Then recurses on itself with the remaining slice
	var inc64 int64
	inc64, err = WriteUint16Slice(w, c[available:])

	return n + inc64, err
}

// WriteUint32 writes a uint32 c into w.
func WriteUint32(w Writer, c uint32) (n int64, err error) {

	if w.Available()>>2 == 0 {
		if err = w.Flush(); err != nil {
			return
		}

		if w.Available()>>2 == 0 {
			return 0, fmt.Errorf("cannot WriteUint32: available buffer/4 is zero even after flush")
		}
	}

	buf := w.AvailableBuffer()[:4]
	binary.LittleEndian.PutUint32(buf, c)
	nint, err := w.Write(buf)
	return int64(nint), err
}

// WriteUint32Slice writes a slice of uint32 c into w.
func WriteUint32Slice(w Writer, c []uint32) (n int64, err error) {

	if len(c) == 0 {
		return
	}

	// Remaining available space in the internal buffer
	available := w.Available() >> 2

	if available == 0 {
		if err = w.Flush(); err != nil {
			return
		}

		available = w.Available() >> 2

		if available == 0 {
			return 0, fmt.Errorf("cannot WriteUint32Slice: available buffer/4 is zero even after flush")
		}
	}

	buf := w.AvailableBuffer()

	if N := len(c); N <= available { // If there is enough space in the available buffer
		buf = buf[:N<<2]
		for i := 0; i < N; i++ {
			binary.LittleEndian.PutUint32(buf[i<<2:], c[i])
		}

		nint, err := w.Write(buf)

		return int64(nint), err
	}

	// First fills the space
	buf = buf[:available<<2]
	for i := 0; i < available; i++ {
		binary.LittleEndian.PutUint32(buf[i<<2:], c[i])
	}

	var inc int
	if inc, err = w.Write(buf); err != nil {
		return n + int64(inc), err
	}

	n += int64(inc)

	// Flushes
	if err = w.Flush(); err != nil {
		return n, err
	}

	// Then recurses on itself with the remaining slice
	var inc64 int64
	inc64, err = WriteUint32Slice(w, c[available:])

	return n + inc64, err
}

// WriteUint64 writes a uint64 c into w.
func WriteUint64(w Writer, c uint64) (n int64, err error) {

	if w.Available()>>3 == 0 {
		if err = w.Flush(); err != nil {
			return
		}

		if w.Available()>>3 == 0 {
			return 0, fmt.Errorf("cannot WriteUint64: available buffer/8 is zero even after flush")
		}
	}

	buf := w.AvailableBuffer()[:8]

	binary.LittleEndian.PutUint64(buf, c)

	nint, err := w.Write(buf)

	return int64(nint), err
}

// WriteUint64Slice writes a slice of uint64 into w.
func WriteUint64Slice(w Writer, c []uint64) (n int64, err error) {

	if len(c) == 0 {
		return
	}

	// Remaining available space in the internal buffer
	available := w.Available() >> 3

	if available == 0 {
		if err = w.Flush(); err != nil {
			return
		}

		available = w.Available() >> 3

		if available == 0 {
			return 0, fmt.Errorf("cannot WriteUint64Slice: available buffer/8 is zero even after flush")
		}
	}

	buf := w.AvailableBuffer()

	if N := len(c); N <= available { // If there is enough space in the available buffer
		buf = buf[:N<<3]
		for i := 0; i < N; i++ {
			binary.LittleEndian.PutUint64(buf[i<<3:], c[i])
		}

		nint, err := w.Write(buf)

		return int64(nint), err
	}

	// First fills the space
	buf = buf[:available<<3]
	for i := 0; i < available; i++ {
		binary.LittleEndian.PutUint64(buf[i<<3:], c[i])
	}

	var inc int
	if inc, err = w.Write(buf); err != nil {
		return n + int64(inc), err
	}

	n += int64(inc)

	// Flushes
	if err = w.Flush(); err != nil {
		return n, err
	}

	// Then recurses on itself with the remaining slice
	var inc64 int64
	inc64, err = WriteUint64Slice(w, c[available:])

	return n + inc64, err
}

// WriteFloat32 writes a float32 c into w.
func WriteFloat32(w Writer, c float32) (n int64, err error) {
	/* #nosec G103 -- behavior and consequences well understood */
	return WriteUint32(w, *(*uint32)(unsafe.Pointer(&c)))
}

// WriteFloat32Slice writes a slice of float32 c into w.
func WriteFloat32Slice(w Writer, c []float32) (n int64, err error) {
	/* #nosec G103 -- behavior and consequences well understood */
	return WriteUint32Slice(w, *(*[]uint32)(unsafe.Pointer(&c)))
}

// WriteFloat64 writes a float64 c into w.
func WriteFloat64(w Writer, c float64) (n int64, err error) {
	/* #nosec G103 -- behavior and consequences well understood */
	return WriteUint64(w, *(*uint64)(unsafe.Pointer(&c)))
}


// WriteFloat64Slice writes a slice of float64 into w.
func WriteFloat64Slice(w Writer, c []float64) (n int64, err error) {
	/* #nosec G103 -- behavior and consequences well understood */
	return WriteUint64Slice(w, *(*[]uint64)(unsafe.Pointer(&c)))
}
