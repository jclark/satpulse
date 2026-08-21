//go:build ignore
// +build ignore

package serialenum

/*
#include <sys/sysctl.h>
*/
import "C"

const (
	ctlSysctl         = C.CTL_SYSCTL
	ctlSysctlName     = C.CTL_SYSCTL_NAME
	ctlSysctlNext     = C.CTL_SYSCTL_NEXT
	ctlSysctlName2oid = C.CTL_SYSCTL_NAME2OID
)
