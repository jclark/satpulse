//go:build !tinygo

package bcast

import "reflect"

func selectCases(cases []reflect.SelectCase) (int, reflect.Value, bool) {
	return reflect.Select(cases)
}
