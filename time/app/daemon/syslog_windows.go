package daemon

// Syslog severity levels (RFC 5424), matching log/syslog's LOG_* constants.
// log/syslog is unavailable on Windows.
const (
	SYSLOG_CRIT    = 2
	SYSLOG_ERR     = 3
	SYSLOG_WARNING = 4
	SYSLOG_NOTICE  = 5
	SYSLOG_INFO    = 6
	SYSLOG_DEBUG   = 7
)
