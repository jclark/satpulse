# Things to do

These are things that have not yet been made into GitHub issues.

## General


* More testing (include fuzzing)
* Figure out how to run with reduced privileges (Linux capabilities I think)
* Reload config on signal: useful for disabling TCP connections.

## Documentation

* Man pages
* How we expect GPS to be configured with gps.config=false

## Grandmaster management

* Enable configuration of non-synced settings or read current settings at startup
* Support ptp4u by creating dynamic config file

## TCP server

* Control over IPv4/IPv6

## Sync

* Could we work with just UBX-TIM-TP?
* Looks like ATGM332D may be using fTOW field of UBX-NAV-TIMEGPS to convey quantization error (rather than solution time)
* Mode where we use system clock rather than GPS messages for time of day (like `ts2phc -s generic`)
* Can we support idea of a UTC correction that would allow clients to align to UTC rather than GNSS time?
  * GPS/BeiDou/Galileo all have concept of steering their system time to a UTC variant.
  * But we align the pulse to GNSS system time (seems to work better)

## Leap seconds

High priority
* Testing of whole leap-second chain
* Make use of UTC-TAI offset in gpsmsg.Time (from GPS receiver through to ptp4l)

Others
* When we get notification of a new leap second, we should store that in a file in /var somewhere, and then read that on startup.
* Include source of leap second information in gpsprot leap second message
* We could tell the GPS about stored leap seconds using UBX-MGA-GPS-UTC (but this has other stuff that is hard to specify)
* How should we hook up with NTP-centric kernel support for leap seconds?
  * If the TAI-UTC offset is set in the kernel (presumably from NTP), we could get that and use it as our default.
  * If we get info about the leap second from GPS, we can use that to update the kernel.

## Serial port network access

* There's a bug: we get a write error when user disconnects
  * Same when using `satpulsetool --socket``

## Serial IO

* Deal with absence of CLOCAL potentially causing open to block
* Cancelled write should complete UBX message but not drain
* Can we be detect when baud rate is too low for information passed?
* Investigate kernel/HW [problem](https://github.com/raspberrypi/linux/issues/4453#issuecomment-1709315332) with framing errors on PL011. Possibly similar problem on M8T connnected to physical serial port.

## GPS configuration

* Figure out how to make U-blox receivers start a new survey, when they have already done one
* Option for satpulsetool to get survey result
* Pay attention to version in CFG-UBX-TP5
* Might need to set CFG-NAVSPG-WKNROLLOVER
* Configure UBX message interface

## Monitoring

* Antenna monitoring (loss of power, jamming, short)
  * report via HTTP or logging
  * requires configuration support
* Messages from receiver
  * NMEA TXT messages
  * UBX-INF-*

## HTTP monitoring

* More cards in HTML interface
   * Leap seconds
   * GPS time mode configuration
   * GPS time pulse configuration
   * GPS constellation/bands configuration
* Time DoP from UBX-NAV-DOP
* In current-time card, include GPS self-reported accuracy
* In PHC card, having something showing how long ago the sync happened
* Prettier display of current time of day
* Statistics about time-sync quality
    * Maybe query parameter on to give time period over which stats would be summarized
* Provide JSON API (instead of just SSE)
* Should we use this for configuration?
   * Need to think about how we deal with concurrent requests.
   * Would require authentication
* Can use UBX-NAV-EOE to collect everything from a single navigation epoch into a single message?
* Support SSL
* Support basic authentication (needs SSL to be secure)

## Chrony integration

For hardware timestamping, chrony needs a free running PHC. One way to do this is with virtual clocks. But in case where user is not interested in PTP, it should be possible to generate samples without modifying the PHC (by keeping track of phase/frequency difference between PHC and the correct time).

We can also run the other way: if PHC is not synchronized, then we read samples from the system clock and
feed them into the PHC and change the clock type appropriately (assuming system clock is controlled with NTP).
