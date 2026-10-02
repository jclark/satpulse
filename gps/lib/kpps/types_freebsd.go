//go:build ignore

/*
Input to cgo -godefs.
*/
package kpps

/*
#include <sys/types.h>
#include <sys/timepps.h>
*/
import "C"

// timepps.h
type ppsInfo C.pps_info_t

type ppsParams C.pps_params_t

type ppsFetchArgs C.struct_pps_fetch_args

type timespec C.struct_timespec

const (
	ppsIocCreate    = C.PPS_IOC_CREATE
	ppsIocDestroy   = C.PPS_IOC_DESTROY
	ppsIocSetParams = C.PPS_IOC_SETPARAMS
	ppsIocGetCap    = C.PPS_IOC_GETCAP
	ppsIocFetch     = C.PPS_IOC_FETCH
	ppsAPIVers1     = C.PPS_API_VERS_1
	ppsTsfmtTspec   = C.PPS_TSFMT_TSPEC
	ppsCanWait      = C.PPS_CANWAIT
)
