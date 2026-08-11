//go:build ignore
// +build ignore

package timeprov

// Input to cgo -godefs; see the go:generate line in timeprov.go.
// wire.h is a verbatim copy of pipe-timeprov's src/wire.h, the
// normative definition of the pipe record.

/*
#include "wire.h"
*/
import "C"

type pipeSample C.struct_PipeSample

const (
	recSize  = C.sizeof_struct_PipeSample
	recMagic = C.PTP_MAGIC
)
