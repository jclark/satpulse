// Package bitsenc decodes bit-packed binary data into Go structs,
// analogous to encoding/binary for byte-aligned data.
// Struct fields are tagged with `bits:"N"` to specify their width in bits.
// Supported field types are unsigned integers, signed integers, and bool.
// Bool fields are always 1 bit wide (the tag value is ignored).
// Signed integers use two's complement sign extension.
// Bits are read MSB-first (big-endian bit order).
//
// Fields without a bits tag use the native size of their type (e.g. uint8=8,
// uint16=16, bool=1). Named struct fields are recursed into.
//
// Slice fields read len(slice) elements. The bit width per element comes from
// the bits tag if present, otherwise from the native element type size.
//
// The special tag bits:"var" reads a variable number of bits. The root struct
// passed to Read must implement VarBits to supply the widths.
//
// If a nil slice is encountered during reading and the root struct implements
// SliceSizer, SizeSlices is called to allocate all slices at once.
package bitsenc

import (
	"fmt"
	"iter"
	"reflect"
	"strconv"
)

const tagName = "bits"

// Reader reads bit-packed binary data. It maintains a bit position
// across multiple Read calls.
type Reader struct {
	data []byte
	pos  int // bit position
}

// NewReader returns a Reader that reads from data.
func NewReader(data []byte) *Reader {
	return &Reader{data: data}
}

// Uint reads n unsigned bits and returns them as a uint64.
func (r *Reader) Uint(n int) (uint64, error) {
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

// Int reads n bits and returns them sign-extended to int64.
func (r *Reader) Int(n int) (int64, error) {
	v, err := r.Uint(n)
	if err != nil {
		return 0, err
	}
	return signExtend(v, n), nil
}

// signExtend sign-extends an n-bit two's complement value to int64.
func signExtend(v uint64, n int) int64 {
	if n < 64 && v>>(n-1)&1 == 1 {
		v |= ^uint64(0) << n
	}
	return int64(v)
}

// VarBits yields the bit width for each bits:"var" field in declaration order.
// The root struct passed to Read must implement this interface.
type VarBits interface {
	VarBits() iter.Seq[int]
}

// SliceSizer allocates slices based on already-read fields.
// Called when readStruct encounters a nil slice on the root struct.
type SliceSizer interface {
	SizeSlices()
}

// Read decodes bit-packed data into a struct pointed to by v.
func Read(data []byte, v any) error {
	return NewReader(data).Read(v)
}

// Read decodes bit-packed data from the reader into a struct pointed to by v.
func (r *Reader) Read(v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("bitsenc: Read requires a pointer to a struct")
	}
	root := rv.Elem()
	var varNext func() (int, bool)
	if vb, ok := v.(VarBits); ok {
		next, stop := iter.Pull(vb.VarBits())
		defer stop()
		varNext = next
	}
	return readStruct(r, root, root, varNext)
}

// nativeBits returns the native bit width for a type, or 0 if unknown.
func nativeBits(k reflect.Kind) int {
	switch k {
	case reflect.Bool:
		return 1
	case reflect.Uint8, reflect.Int8:
		return 8
	case reflect.Uint16, reflect.Int16:
		return 16
	case reflect.Uint32, reflect.Int32:
		return 32
	case reflect.Uint64, reflect.Int64:
		return 64
	}
	return 0
}

func readStruct(r *Reader, sv, root reflect.Value, varNext func() (int, bool)) error {
	st := sv.Type()
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		fv := sv.Field(i)
		// Recurse into struct fields (both embedded and named).
		if f.Type.Kind() == reflect.Struct {
			if err := readStruct(r, fv, root, varNext); err != nil {
				return err
			}
			continue
		}
		// Skip unexported non-struct fields (not settable via reflect).
		if !f.IsExported() {
			continue
		}
		// Slice fields.
		if f.Type.Kind() == reflect.Slice {
			if fv.IsNil() {
				// Trigger SliceSizer on root struct.
				if ss, ok := root.Addr().Interface().(SliceSizer); ok {
					ss.SizeSlices()
				}
			}
			n := fv.Len()
			elemKind := f.Type.Elem().Kind()
			tag := f.Tag.Get(tagName)
			bits := 0
			if tag != "" {
				var err error
				bits, err = strconv.Atoi(tag)
				if err != nil {
					return fmt.Errorf("bitsenc: bad tag %q on field %s", tag, f.Name)
				}
			} else {
				bits = nativeBits(elemKind)
				if bits == 0 {
					return fmt.Errorf("bitsenc: unsupported slice element type %s for field %s", f.Type.Elem(), f.Name)
				}
			}
			for j := 0; j < n; j++ {
				ev := fv.Index(j)
				if err := readField(r, ev, elemKind, bits); err != nil {
					return err
				}
			}
			continue
		}
		tag := f.Tag.Get(tagName)
		if tag == "var" {
			if varNext == nil {
				return fmt.Errorf("bitsenc: bits:\"var\" on field %s but struct does not implement VarBits", f.Name)
			}
			width, ok := varNext()
			if !ok {
				return fmt.Errorf("bitsenc: VarBits iterator exhausted at field %s", f.Name)
			}
			if width == 0 {
				continue
			}
			bits, err := r.Uint(width)
			if err != nil {
				return err
			}
			switch fv.Kind() {
			case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				fv.SetUint(bits)
			case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				fv.SetInt(signExtend(bits, width))
			default:
				return fmt.Errorf("bitsenc: unsupported type %s for var field %s", f.Type, f.Name)
			}
			continue
		}
		bits := 0
		if tag != "" {
			var err error
			bits, err = strconv.Atoi(tag)
			if err != nil {
				return fmt.Errorf("bitsenc: bad tag %q on field %s", tag, f.Name)
			}
		} else {
			bits = nativeBits(fv.Kind())
			if bits == 0 {
				continue // skip unsupported untagged fields (e.g. int, string)
			}
		}
		if err := readField(r, fv, fv.Kind(), bits); err != nil {
			return err
		}
	}
	return nil
}

func readField(r *Reader, fv reflect.Value, kind reflect.Kind, bits int) error {
	if kind == reflect.Bool {
		bit, err := r.Uint(1)
		if err != nil {
			return err
		}
		fv.SetBool(bit != 0)
		return nil
	}
	v, err := r.Uint(bits)
	if err != nil {
		return err
	}
	switch kind {
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		fv.SetUint(v)
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		fv.SetInt(signExtend(v, bits))
	default:
		return fmt.Errorf("bitsenc: unsupported type %s", fv.Type())
	}
	return nil
}
