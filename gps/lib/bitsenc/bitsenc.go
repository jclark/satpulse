// Package bitsenc decodes bit-packed binary data into Go structs,
// analogous to encoding/binary for byte-aligned data.
// Struct fields are tagged with `bits:"N"` to specify their width in bits.
// Supported field types are unsigned integers, signed integers, and bool.
// Bool fields are always 1 bit wide (the tag value is ignored).
// Signed integers use two's complement sign extension.
// Bits are read MSB-first (big-endian bit order).
package bitsenc

import (
	"fmt"
	"reflect"
	"strconv"
)

const tagName = "bits"

type bitReader struct {
	data []byte
	pos  int // bit position
}

func (r *bitReader) uint(n int) (uint64, error) {
	if n <= 0 || n > 64 {
		return 0, fmt.Errorf("bitsenc: invalid bit width %d", n)
	}
	if (r.pos+n+7)/8 > len(r.data) {
		return 0, fmt.Errorf("bitsenc: read past end of data at bit %d", r.pos)
	}
	var v uint64
	for i := 0; i < n; i++ {
		byteIdx := (r.pos + i) / 8
		bitIdx := 7 - (r.pos+i)%8
		v = v<<1 | uint64((r.data[byteIdx]>>bitIdx)&1)
	}
	r.pos += n
	return v, nil
}

// signExtend sign-extends an n-bit two's complement value to int64.
func signExtend(v uint64, n int) int64 {
	if n < 64 && v>>(n-1)&1 == 1 {
		v |= ^uint64(0) << n
	}
	return int64(v)
}

// Read decodes bit-packed data into a struct pointed to by v.
// Fields with a `bits:"N"` tag are read sequentially from data.
// Fields without the tag are skipped, unless they are embedded structs,
// which are recursed into.
func Read(data []byte, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("bitsenc: Read requires a pointer to a struct")
	}
	r := &bitReader{data: data}
	return readStruct(r, rv.Elem())
}

func readStruct(r *bitReader, sv reflect.Value) error {
	st := sv.Type()
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		fv := sv.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			if err := readStruct(r, fv); err != nil {
				return err
			}
			continue
		}
		tag := f.Tag.Get(tagName)
		if tag == "" {
			continue
		}
		if f.Type.Kind() == reflect.Bool {
			bit, err := r.uint(1)
			if err != nil {
				return err
			}
			fv.SetBool(bit != 0)
			continue
		}
		n, err := strconv.Atoi(tag)
		if err != nil {
			return fmt.Errorf("bitsenc: bad tag %q on field %s", tag, f.Name)
		}
		bits, err := r.uint(n)
		if err != nil {
			return err
		}
		switch fv.Kind() {
		case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			fv.SetUint(bits)
		case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			fv.SetInt(signExtend(bits, n))
		default:
			return fmt.Errorf("bitsenc: unsupported type %s for field %s", f.Type, f.Name)
		}
	}
	return nil
}
