package check

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

var ruleRE = regexp.MustCompile(`^(>=?|<=?)(.+)$`)

// Validate validates all fields in a struct based on their "check" tags.
// Recursively validates nested structs, building qualified names using toml tags.
// Returns nil if all validations pass, or a slice of validation error messages.
// Panics if v is not a struct or if check tags are malformed.
func Validate(v any) []string {
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		panic(fmt.Sprintf("check.Validate: expected struct, got %s", val.Kind()))
	}
	return validateStruct(val, "")
}

func validateStruct(val reflect.Value, prefix string) []string {
	typ := val.Type()
	var errs []string
	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		fval := val.Field(i)

		// Build qualified name using toml tag or field name
		name := field.Tag.Get("toml")
		if name == "" {
			name = field.Name
		}
		if prefix != "" {
			name = prefix + "." + name
		}

		// Check for validation tag
		tag := field.Tag.Get("check")
		if tag != "" {
			errs = append(errs, validateField(name, fval, tag)...)
		}

		// Recurse into nested structs
		if fval.Kind() == reflect.Struct {
			errs = append(errs, validateStruct(fval, name)...)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func validateField(name string, val reflect.Value, tag string) []string {
	var errs []string
	parts := strings.Split(tag, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		m := ruleRE.FindStringSubmatch(part)
		if m == nil {
			panic(fmt.Sprintf("%s: invalid check tag: %q", name, part))
		}
		op, limit := m[1], m[2]
		if checkRule(val, op, limit) {
			errs = append(errs, fmt.Sprintf("%s: must be %s %s, got %v", name, op, limit, val.Interface()))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func checkRule(val reflect.Value, op, limit string) bool {
	// The tests use !(v op l) so that they work properly with NaN.
	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v := val.Int()
		l, err := strconv.ParseInt(limit, 0, 64)
		if err != nil {
			panic(fmt.Sprintf("invalid limit %q: %v", limit, err))
		}
		switch op {
		case ">":
			return !(v > l)
		case ">=":
			return !(v >= l)
		case "<":
			return !(v < l)
		case "<=":
			return !(v <= l)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v := val.Uint()
		l, err := strconv.ParseUint(limit, 0, 64)
		if err != nil {
			panic(fmt.Sprintf("invalid limit %q: %v", limit, err))
		}
		switch op {
		case ">":
			return !(v > l)
		case ">=":
			return !(v >= l)
		case "<":
			return !(v < l)
		case "<=":
			return !(v <= l)
		}
	case reflect.Float32, reflect.Float64:
		v := val.Float()
		l, err := strconv.ParseFloat(limit, 64)
		if err != nil {
			panic(fmt.Sprintf("invalid limit %q: %v", limit, err))
		}
		switch op {
		case ">":
			return !(v > l)
		case ">=":
			return !(v >= l)
		case "<":
			return !(v < l)
		case "<=":
			return !(v <= l)
		}
	default:
		panic(fmt.Sprintf("unsupported type %s for check tag", val.Kind()))
	}
	panic(fmt.Sprintf("unknown operator %q", op))
}
