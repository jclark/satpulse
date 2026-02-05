// Package opt provides an optional value type for struct fields.
// Val[T] represents a value that may or may not be present,
// useful for parsing formats where fields can be empty.
package opt

import (
	"encoding"
	"encoding/json"
	"fmt"
	"strconv"
)

// Val represents an optional value of type T.
// The zero value is an unset optional.
type Val[T any] struct {
	val T
	set bool
}

// Make creates a Val with the given value set.
func Make[T any](v T) Val[T] {
	return Val[T]{val: v, set: true}
}

func (v *Val[T]) Set(x T)      { v.set, v.val = true, x }
func (v *Val[T]) Get() T       { return v.val }
func (v *Val[T]) Clear()       { var zero T; v.set, v.val = false, zero }
func (v *Val[T]) IsSet() bool  { return v.set }
func (v *Val[T]) IsZero() bool { return !v.set }

// UnmarshalText implements encoding.TextUnmarshaler.
// An empty string leaves the value unset.
func (v *Val[T]) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		return nil
	}
	if err := ParseText(string(text), &v.val); err != nil {
		return err
	}
	v.set = true
	return nil
}

// MarshalText implements encoding.TextMarshaler.
// Returns empty string if unset.
func (v Val[T]) MarshalText() ([]byte, error) {
	if !v.set {
		return nil, nil
	}
	return FormatText(v.val)
}

// UnmarshalJSON implements json.Unmarshaler.
// Null is not accepted; use omitzero to omit unset fields.
func (v *Val[T]) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return fmt.Errorf("null not allowed for optional field")
	}
	if err := json.Unmarshal(data, &v.val); err != nil {
		return err
	}
	v.set = true
	return nil
}

// MarshalJSON implements json.Marshaler.
// Returns null if unset. Prefer using `json:",omitzero"` to omit unset fields entirely.
func (v Val[T]) MarshalJSON() ([]byte, error) {
	if !v.set {
		return []byte("null"), nil
	}
	return json.Marshal(v.val)
}

// ParseText parses a string into a value.
// It handles types implementing encoding.TextUnmarshaler and built-in types.
func ParseText(text string, v any) error {
	if tu, ok := v.(encoding.TextUnmarshaler); ok {
		return tu.UnmarshalText([]byte(text))
	}
	switch v := v.(type) {
	case *string:
		*v = text
	case *int:
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return err
		}
		*v = int(n)
	case *int8:
		n, err := strconv.ParseInt(text, 10, 8)
		if err != nil {
			return err
		}
		*v = int8(n)
	case *int16:
		n, err := strconv.ParseInt(text, 10, 16)
		if err != nil {
			return err
		}
		*v = int16(n)
	case *int32:
		n, err := strconv.ParseInt(text, 10, 32)
		if err != nil {
			return err
		}
		*v = int32(n)
	case *int64:
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return err
		}
		*v = int64(n)
	case *uint:
		n, err := strconv.ParseUint(text, 0, 64)
		if err != nil {
			return err
		}
		*v = uint(n)
	case *uint8:
		n, err := strconv.ParseUint(text, 0, 8)
		if err != nil {
			return err
		}
		*v = uint8(n)
	case *uint16:
		n, err := strconv.ParseUint(text, 0, 16)
		if err != nil {
			return err
		}
		*v = uint16(n)
	case *uint32:
		n, err := strconv.ParseUint(text, 0, 32)
		if err != nil {
			return err
		}
		*v = uint32(n)
	case *uint64:
		n, err := strconv.ParseUint(text, 0, 64)
		if err != nil {
			return err
		}
		*v = uint64(n)
	case *float32:
		n, err := strconv.ParseFloat(text, 32)
		if err != nil {
			return err
		}
		*v = float32(n)
	case *float64:
		n, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return err
		}
		*v = float64(n)
	case *bool:
		b, err := strconv.ParseBool(text)
		if err != nil {
			return err
		}
		*v = b
	default:
		return fmt.Errorf("unsupported type %T", v)
	}
	return nil
}

// FormatText formats a value as text.
// It handles types implementing encoding.TextMarshaler and built-in types.
// Floats use shortest representation that round-trips exactly.
func FormatText(v any) ([]byte, error) {
	if tm, ok := v.(encoding.TextMarshaler); ok {
		return tm.MarshalText()
	}
	switch v := v.(type) {
	case float32:
		return []byte(strconv.FormatFloat(float64(v), 'g', -1, 32)), nil
	case float64:
		return []byte(strconv.FormatFloat(v, 'g', -1, 64)), nil
	default:
		return fmt.Appendf(nil, "%v", v), nil
	}
}
