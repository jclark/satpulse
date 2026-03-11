# Uncovered items from old TODO

Items from `TODO.md` that are not yet implemented, and not covered by
a GitHub issue or plan file.

## General

* Run with reduced privileges (Linux capabilities)
* Reload config on signal (e.g. to disable TCP connections without restart)

## Grandmaster management

* Enable configuration of non-synced settings or read current settings
  at startup
* Support ptp4u by creating dynamic config file

## TCP server

* Control over IPv4/IPv6

## Sync

* Could we work with just UBX-TIM-TP?
* ATGM332D may be using fTOW field of UBX-NAV-TIMEGPS to convey
  quantization error (rather than solution time)
* Mode where we use system clock rather than GPS for time of day
  (like `ts2phc -s generic`)

## Leap seconds

* End-to-end testing of whole leap-second chain
* Make use of UTC-TAI offset in gpsmsg.Time (from GPS receiver through
  to ptp4l)
* Include source of leap second information in gpsprot leap second
  message (receiver vs config file vs compiled-in default)
* Store new leap second notification in a file in /var, read on startup
* Kernel leap second integration: if TAI-UTC offset is set in kernel
  (from NTP), use as default; if we get info from GPS, update the kernel

## Serial port network access

* Write error when user disconnects (also with `satpulsetool --socket`)

## Serial IO

* Cancelled write should complete UBX message but not drain
* Detect when baud rate is too low for information passed

## GPS configuration

* Progressive surveys: series of surveys with progressively longer
  observation times and tighter deviations
* Time pulse sync mode for FTS products (e.g. M8F)
* Option for satpulsetool to get survey result
* Might need to set CFG-NAVSPG-WKNROLLOVER
* NMEA TXT message handling
* Surface UBX-INF messages (parsed but not logged or reported)
* UBX-MGA-GPS-UTC to tell GPS about stored leap seconds (part of
  broader AGNSS topic: providing info to receiver to aid acquisition
  after startup or loss of connection)

## HTTP monitoring

* GPS self-reported time accuracy in current-time card
* How long ago the last PHC sync happened
* Statistics about time-sync quality (possibly with query parameter
  for time period)
* SSL support
* Basic authentication (needs SSL)
* HTTP-based configuration (needs auth, concurrent request handling)

## Chrony integration

* For hardware timestamping without PTP, generate samples without
  modifying the PHC (track phase/frequency difference to correct time)
* Reverse direction: if PHC is not synchronized, read samples from
  system clock (assumed NTP-controlled) and feed into PHC
