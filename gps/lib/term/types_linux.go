//go:build ignore
// +build ignore

/*
Input to cgo -godefs.
*/
package term

/*
#include <linux/serial.h>
*/
import "C"

// serial.h
type serialICounter C.struct_serial_icounter_struct
