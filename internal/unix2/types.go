//go:build ignore
// +build ignore

/*
Input to cgo -godefs.
*/
package unix2

/*
#include <linux/ptp_clock.h>
#include <linux/serial.h>
#include <linux/ethtool.h>
#include <net/if.h>
*/
import "C"

// ptp_clock.h
type PTPClockTime C.struct_ptp_clock_time
type PTPClockCaps C.struct_ptp_clock_caps

const (
	PTP_ENABLE_FEATURE = C.PTP_ENABLE_FEATURE
	PTP_RISING_EDGE    = C.PTP_RISING_EDGE
	PTP_FALLING_EDGE   = C.PTP_FALLING_EDGE
	PTP_STRICT_FLAGS   = C.PTP_STRICT_FLAGS
	PTP_EXTTS_EDGES    = C.PTP_EXTTS_EDGES
)

type PTPExttsRequest C.struct_ptp_extts_request

const PTP_MAX_SAMPLES = C.PTP_MAX_SAMPLES

type PTPSysOffset C.struct_ptp_sys_offset
type PTPSysOffsetExtended C.struct_ptp_sys_offset_extended
type PTPSysOffsetPrecise C.struct_ptp_sys_offset_precise
type PTPPeroutRequest C.struct_ptp_perout_request

const (
	PTP_PF_NONE    = C.PTP_PF_NONE
	PTP_PF_EXTTS   = C.PTP_PF_EXTTS
	PTP_PF_PEROUT  = C.PTP_PF_PEROUT
	PTP_PF_PHYSYNC = C.PTP_PF_PEROUT
)

type PTPPinDesc C.struct_ptp_pin_desc

type PTPExttsEvent C.struct_ptp_extts_event

const SizeofPTPExttsEvent = C.sizeof_struct_ptp_extts_event

// ioctl requests

const (
	PTP_CLOCK_GETCAPS       = C.PTP_CLOCK_GETCAPS
	PTP_EXTTS_REQUEST       = C.PTP_EXTTS_REQUEST
	PTP_PEROUT_REQUEST      = C.PTP_PEROUT_REQUEST
	PTP_ENABLE_PPS          = C.PTP_ENABLE_PPS
	PTP_SYS_OFFSET          = C.PTP_SYS_OFFSET
	PTP_PIN_GETFUNC         = C.PTP_PIN_GETFUNC
	PTP_PIN_SETFUNC         = C.PTP_PIN_SETFUNC
	PTP_SYS_OFFSET_PRECISE  = C.PTP_SYS_OFFSET_PRECISE
	PTP_SYS_OFFSET_EXTENDED = C.PTP_SYS_OFFSET_EXTENDED

	PTP_CLOCK_GETCAPS2       = C.PTP_CLOCK_GETCAPS2
	PTP_EXTTS_REQUEST2       = C.PTP_EXTTS_REQUEST2
	PTP_PEROUT_REQUEST2      = C.PTP_PEROUT_REQUEST2
	PTP_ENABLE_PPS2          = C.PTP_ENABLE_PPS2
	PTP_SYS_OFFSET2          = C.PTP_SYS_OFFSET2
	PTP_PIN_GETFUNC2         = C.PTP_PIN_GETFUNC2
	PTP_PIN_SETFUNC2         = C.PTP_PIN_SETFUNC2
	PTP_SYS_OFFSET_PRECISE2  = C.PTP_SYS_OFFSET_PRECISE2
	PTP_SYS_OFFSET_EXTENDED2 = C.PTP_SYS_OFFSET_EXTENDED2
)

// serial.h
type SerialICounter C.struct_serial_icounter_struct

// ethtool.h
type EthtoolTsInfo C.struct_ethtool_ts_info

// if.h
const sizeofIFreq = C.sizeof_struct_ifreq
