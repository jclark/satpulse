//go:build ignore

/*
Input to cgo -godefs.
*/
package kpps

/*
#include <linux/pps.h>
*/
import "C"

// pps.h
const ppsMaxNameLen = C.PPS_MAX_NAME_LEN
