//go:build ignore
// +build ignore

/*
Input to cgo -godefs.
*/
package unix2

/*
#include <sys/timex.h>
#include <linux/ptp_clock.h>
#include <linux/ethtool.h>
#include <net/if.h>
*/
import "C"

// timex.h

// Flags for timex.Modes
const (
	ADJ_OFFSET            = C.ADJ_OFFSET
	ADJ_FREQUENCY         = C.ADJ_FREQUENCY
	ADJ_MAXERROR          = C.ADJ_MAXERROR
	ADJ_ESTERROR          = C.ADJ_ESTERROR
	ADJ_STATUS            = C.ADJ_STATUS
	ADJ_TIMECONST         = C.ADJ_TIMECONST
	ADJ_TAI               = C.ADJ_TAI
	ADJ_SETOFFSET         = C.ADJ_SETOFFSET
	ADJ_MICRO             = C.ADJ_MICRO
	ADJ_NANO              = C.ADJ_NANO
	ADJ_TICK              = C.ADJ_TICK
	ADJ_OFFSET_SINGLESHOT = C.ADJ_OFFSET_SINGLESHOT
	ADJ_OFFSET_SS_READ    = C.ADJ_OFFSET_SS_READ
)

// ptp_clock.h
type PTPClockTime C.struct_ptp_clock_time
type PTPClockCaps C.struct_ptp_clock_caps

const (
	PTP_ENABLE_FEATURE = C.PTP_ENABLE_FEATURE
	PTP_RISING_EDGE    = C.PTP_RISING_EDGE
	PTP_FALLING_EDGE   = C.PTP_FALLING_EDGE
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
)

type EthtoolTsInfo C.struct_ethtool_ts_info

const sizeofIFreq = C.sizeof_struct_ifreq
