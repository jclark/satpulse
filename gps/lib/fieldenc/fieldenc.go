package fieldenc

import (
	"encoding"
	"fmt"
	"reflect"
	"strconv"
)

// PartialDecode parses string fields into a Go struct, similar to encoding/binary.Read.
// Fields are processed in struct field order. Custom types should implement encoding.TextUnmarshaler.
// Returns the number of fields consumed and an error.
func PartialDecode(fields []string, v interface{}) (int, error) {
	// Get reflection info
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return 0, fmt.Errorf("fieldenc: v must be a pointer to struct")
	}

	rv = rv.Elem() // Dereference pointer
	rt := rv.Type()

	// Process each struct field in order
	fieldIndex := 0
	for i := 0; i < rv.NumField(); i++ {
		field := rv.Field(i)

		// Performance optimization: check CanSet() before expensive StructField creation
		if field.CanSet() || rt.Field(i).Name == "_" {
			fieldType := rt.Field(i)

			// Error on fields named "_" - not supported
			if fieldType.Name == "_" {
				return 0, fmt.Errorf("fieldenc: blank identifier fields ('_') are not supported")
			}

			// Handle embedded structs
			if fieldType.Anonymous && field.Kind() == reflect.Struct {
				err := decodeStruct(fields, &fieldIndex, field)
				if err != nil {
					return fieldIndex, fmt.Errorf("fieldenc: embedded struct %s: %w", fieldType.Name, err)
				}
			} else {
				if fieldIndex >= len(fields) {
					break // Not enough fields in input
				}

				value := fields[fieldIndex]
				fieldIndex++

				err := setField(field, value)
				if err != nil {
					return fieldIndex, fmt.Errorf("fieldenc: field %s: %w", fieldType.Name, err)
				}
			}
		}
		// Skip unexported fields (no CanSet() and not "_")
	}

	return fieldIndex, nil
}

// Decode parses string fields into a Go struct, similar to encoding/binary.Read.
// Fields are processed in struct field order. Custom types should implement encoding.TextUnmarshaler.
// Returns an error if not all fields are consumed or if parsing fails.
func Decode(fields []string, v interface{}) error {
	fieldsConsumed, err := PartialDecode(fields, v)
	if err != nil {
		return err
	}
	if fieldsConsumed != len(fields) {
		return fmt.Errorf("fieldenc: expected to consume %d fields, consumed %d", len(fields), fieldsConsumed)
	}
	return nil
}

// decodeStruct recursively decodes fields into a struct value
func decodeStruct(fields []string, fieldIndex *int, rv reflect.Value) error {
	rt := rv.Type()

	for i := 0; i < rv.NumField(); i++ {
		field := rv.Field(i)

		// Performance optimization: check CanSet() before expensive StructField creation
		if field.CanSet() || rt.Field(i).Name == "_" {
			fieldType := rt.Field(i)

			// Error on fields named "_" - not supported
			if fieldType.Name == "_" {
				return fmt.Errorf("fieldenc: blank identifier fields ('_') are not supported")
			}

			// Handle nested embedded structs
			if fieldType.Anonymous && field.Kind() == reflect.Struct {
				err := decodeStruct(fields, fieldIndex, field)
				if err != nil {
					return fmt.Errorf("embedded struct %s: %w", fieldType.Name, err)
				}
			} else {
				if *fieldIndex >= len(fields) {
					break // Not enough fields in input
				}

				value := fields[*fieldIndex]
				*fieldIndex++

				err := setField(field, value)
				if err != nil {
					return fmt.Errorf("field %s: %w", fieldType.Name, err)
				}
			}
		}
		// Skip unexported fields (no CanSet() and not "_")
	}

	return nil
}

// setField sets a single struct field from a string value
func setField(field reflect.Value, value string) error {
	// Try encoding.TextUnmarshaler first for custom types
	if field.CanAddr() {
		if tu, ok := field.Addr().Interface().(encoding.TextUnmarshaler); ok {
			return tu.UnmarshalText([]byte(value))
		}
	}

	// Fall back to standard type parsing
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(value, 10, int(field.Type().Size())*8)
		if err != nil {
			return err
		}
		field.SetInt(i)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u, err := strconv.ParseUint(value, 0, int(field.Type().Size())*8)
		if err != nil {
			return err
		}
		field.SetUint(u)

	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(value, int(field.Type().Size())*8)
		if err != nil {
			return err
		}
		field.SetFloat(f)

	case reflect.Bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		field.SetBool(b)

	case reflect.Array:
		// Handle [N]byte as null-terminated string
		if field.Type().Elem().Kind() == reflect.Uint8 {
			bytes := []byte(value)
			arrayLen := field.Len()

			// Copy bytes into array, ensuring null termination
			for i := range arrayLen {
				if i < len(bytes) {
					field.Index(i).SetUint(uint64(bytes[i]))
				} else {
					field.Index(i).SetUint(0) // null terminate
				}
			}
		} else {
			return fmt.Errorf("unsupported array type: %v", field.Type())
		}

	default:
		return fmt.Errorf("unsupported type: %v", field.Type())
	}

	return nil
}

// Encode converts a Go struct to string fields, similar to encoding/binary.Write.
// Fields are processed in struct field order. Custom types should implement encoding.TextMarshaler.
func Encode(v interface{}) ([]string, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Struct {
		return nil, fmt.Errorf("fieldenc: v must be a struct or pointer to struct")
	}

	fields := []string{}

	// Process each struct field in order
	for i := 0; i < rv.NumField(); i++ {
		field := rv.Field(i)
		rt := rv.Type()

		// Performance optimization: check CanSet() before expensive StructField creation
		// For encoding, we also need to handle exported fields that can't be set
		if field.CanSet() || field.CanInterface() || rt.Field(i).Name == "_" {
			fieldType := rt.Field(i)

			// Error on fields named "_" - not supported
			if fieldType.Name == "_" {
				return nil, fmt.Errorf("fieldenc: blank identifier fields ('_') are not supported")
			}

			// Handle embedded structs
			if fieldType.Anonymous && field.Kind() == reflect.Struct {
				embeddedFields, err := encodeStruct(field)
				if err != nil {
					return nil, fmt.Errorf("fieldenc: embedded struct %s: %w", fieldType.Name, err)
				}
				fields = append(fields, embeddedFields...)
			} else {
				value, err := getFieldValue(field)
				if err != nil {
					return nil, fmt.Errorf("fieldenc: field %s: %w", fieldType.Name, err)
				}
				fields = append(fields, value)
			}
		}
		// Skip unexported fields that can't be accessed
	}

	return fields, nil
}

// encodeStruct recursively encodes a struct value to string fields
func encodeStruct(rv reflect.Value) ([]string, error) {
	fields := []string{}
	rt := rv.Type()

	for i := 0; i < rv.NumField(); i++ {
		field := rv.Field(i)

		// Performance optimization: check CanSet() before expensive StructField creation
		// For encoding, we also need to handle exported fields that can't be set
		if field.CanSet() || field.CanInterface() || rt.Field(i).Name == "_" {
			fieldType := rt.Field(i)

			// Error on fields named "_" - not supported
			if fieldType.Name == "_" {
				return nil, fmt.Errorf("fieldenc: blank identifier fields ('_') are not supported")
			}

			// Handle nested embedded structs
			if fieldType.Anonymous && field.Kind() == reflect.Struct {
				embeddedFields, err := encodeStruct(field)
				if err != nil {
					return nil, fmt.Errorf("embedded struct %s: %w", fieldType.Name, err)
				}
				fields = append(fields, embeddedFields...)
			} else {
				value, err := getFieldValue(field)
				if err != nil {
					return nil, fmt.Errorf("field %s: %w", fieldType.Name, err)
				}
				fields = append(fields, value)
			}
		}
		// Skip unexported fields that can't be accessed
	}

	return fields, nil
}

// getFieldValue gets the string representation of a single struct field
func getFieldValue(field reflect.Value) (string, error) {
	// Try encoding.TextMarshaler first for custom types
	if field.CanInterface() {
		if tm, ok := field.Interface().(encoding.TextMarshaler); ok {
			text, err := tm.MarshalText()
			return string(text), err
		}
	}

	// Fall back to standard type formatting
	switch field.Kind() {
	case reflect.String:
		return field.String(), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(field.Int(), 10), nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(field.Uint(), 10), nil

	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(field.Float(), 'g', -1, int(field.Type().Size())*8), nil

	case reflect.Bool:
		return strconv.FormatBool(field.Bool()), nil

	case reflect.Array:
		// Handle [N]byte as null-terminated string
		if field.Type().Elem().Kind() == reflect.Uint8 {
			bytes := make([]byte, 0, field.Len())
			for i := 0; i < field.Len(); i++ {
				b := byte(field.Index(i).Uint())
				if b == 0 {
					break // Stop at null terminator
				}
				bytes = append(bytes, b)
			}
			return string(bytes), nil
		} else {
			return "", fmt.Errorf("unsupported array type: %v", field.Type())
		}

	default:
		return "", fmt.Errorf("unsupported type: %v", field.Type())
	}
}
