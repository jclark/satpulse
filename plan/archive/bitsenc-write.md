# bitsenc serialization

## Context

`gps/lib/bitsenc` decodes bit-packed binary data into Go structs via `Reader`
and `Read`. Serialization requires the symmetric `Writer` and `Write`. The
usage pattern is the same: the caller passes a struct with `bits:"N"` tags and
gets packed bytes back.

All new code goes in `gps/lib/bitsenc/bitsenc.go`.

## Design

### Writer type

Exported, symmetric to `Reader`.

```go
type Writer struct {
    buf []byte
    pos int // bit position
}
```

```go
// NewWriter returns a Writer that appends to buf.
// pos is initialized to len(buf)*8 so writes append after existing content.
// If buf is nil, the Writer allocates as needed.
func NewWriter(buf []byte) *Writer

// PutUint writes the low n bits of v in MSB-first order.
// Panics if n <= 0 or n > 64.
func (w *Writer) PutUint(v uint64, n int)

// PutInt writes the low n bits of v (two's complement) in MSB-first order.
// Equivalent to PutUint(uint64(v), n).
func (w *Writer) PutInt(v int64, n int)

// PutBool writes a single bit: 1 for true, 0 for false.
func (w *Writer) PutBool(v bool)

// Bytes returns the accumulated buffer. The final byte is zero-padded
// if the bit position is not byte-aligned.
func (w *Writer) Bytes() []byte

// BitLen returns the number of bits written.
func (w *Writer) BitLen() int
```

PutUint/PutInt panic on invalid n (programmer error, like index out of
bounds). The buffer grows as needed, so no other errors are possible.

### Bit writing logic

Symmetric to `Reader.Uint`. For each of n bits (MSB first), set the
corresponding bit in the output byte:

    byteIdx := (w.pos + i) / 8
    bitIdx  := 7 - (w.pos + i) % 8

Grow `w.buf` with `append` when `byteIdx >= len(w.buf)`.

### Writer.Write method

```go
// Write encodes the bit-tagged fields of the struct pointed to by v
// into the writer.
func (w *Writer) Write(v any) error
```

Symmetric to `Reader.Read`. Validates v is a pointer to a struct, sets up
`VarBits` iterator if implemented, calls the unexported `writeStruct`.

Handles every case that `readStruct` handles:

- **Struct fields** (embedded and named): recurse.
- **`bits:"N"` tagged fields**: write N bits via PutUint/PutInt/PutBool.
- **Untagged fields with native size** (uint8=8, uint16=16, etc.): write at
  native width.
- **`bits:"var"` fields**: the root struct must implement `VarBits`. Pull the
  next width from the iterator. If width is 0, skip the field (matching the
  read side). Otherwise write that many bits.
- **Slice fields**: write `len(slice)` elements. Bit width per element from
  `bits:"N"` tag or native element type size.
- **Bool fields**: PutBool (1 bit, tag value ignored).
- **Untagged fields without native size** (e.g. int, string): skip.

Returns error only for bad tags or unsupported field types.

No `SliceSizer` equivalent is needed: slices are already allocated and sized
when writing.

### Write convenience function

```go
// Write encodes the bit-tagged fields of the struct pointed to by v
// and returns the packed bytes.
func Write(v any) ([]byte, error)
```

Equivalent to `NewWriter(nil)` + `Writer.Write` + `Bytes()`.

## Error handling

The Writer is a low-level bit packing primitive. It writes the low n bits of
any value without range checking, symmetric to how `Reader.Uint` reads n bits
without validating the result fits the destination type. Protocol-level
validation (e.g. RTCM field ranges, mask/slice consistency) is the caller's
responsibility, not the Writer's.

## Verification

- Round-trip test: for existing Read test cases where the struct consumes all
  input bits, Read the data into a struct, Write it back, verify the bytes
  match the original input. Skip fixtures that leave trailing unread bits
  (the Writer only serializes what the struct contains).
- Round-trip MT1005 and MT1006 using real packet data from existing tests
  in `gps/lib/rtcmbin/`.
- Test Writer.PutUint/PutInt directly: write known bit patterns, verify bytes.
- Test edge cases: 64-bit fields, bool fields, partial final byte padding,
  slices, `bits:"var"` fields.
- `go test -v ./gps/lib/bitsenc/`
