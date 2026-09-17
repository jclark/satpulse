//go:build !windows

package daemon

import "log/syslog"

// Syslog severity levels, from log/syslog (RFC 5424).
const (
	SYSLOG_CRIT    = int(syslog.LOG_CRIT)
	SYSLOG_ERR     = int(syslog.LOG_ERR)
	SYSLOG_WARNING = int(syslog.LOG_WARNING)
	SYSLOG_NOTICE  = int(syslog.LOG_NOTICE)
	SYSLOG_INFO    = int(syslog.LOG_INFO)
	SYSLOG_DEBUG   = int(syslog.LOG_DEBUG)
)
