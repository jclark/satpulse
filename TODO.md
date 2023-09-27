# Things to do

## General

High priority

* More documentation
   * config file
   * http monitoring
   * how GPS should be configured
* Merge FEATURES.md into README.md/TODO.md
* Control over logging in the non-sdlog case
* Handle losing sync properly
   * Could just message the grandmaster and exit?
* Better error for no PPS
* Check that the interface is up
* Stats about time sync quality
   * Maximum offset
   * RMS offset
   * Something on frequency
   * Did we miss any pulses?

Others
* More testing (include fuzzing)
* Figure out how to run with reduced privileges (Linux capabilities I think)
* Try to get rid of the logctx package.
* Allow SIGHUP to reload config: useful for disabling TCP connections.

## Grandmaster management

High priority

* Robustly detect loss of sync
   * Need make periodic calls to the Grandmaster even when there is no valid GPS time message
* Estimate clock accuracy and dynamically update grandmaster settings. In particular, on startup, update with low accuracy and then improve it as we settle down.
* More sophisticated, configurable detection of whether we are synced (mostly covered by previous point)
* Set a read/write deadline on the conn for the Unix domain socket

Others
* Enable configuration of non-synced settings
* Support ptp4u by creating dynamic config file
* Should we update the grandmaster to non-synced while starting up?
* Should we update the grandmaster settings to a non-synced status when we shutdown?

## TCP server

* Test with Lady Heather
* Control over IPv4/IPv6

## Time sync

High priority
* Better handle the case where we lose sync: may need to step clock
* Log some statistics about the servo (similar to what ptp4l does)
* Make correlator and servo configurable from TOML
   * kp, ki constants
   * initial observe time
   * when to step clock
   * various correlator constants

Others

* Can we improve how UBX-TIM-TP is handled and the corresponding PulseOffset message? Problem is that on CM4 there can be one then 1 sec between receiving the PulseOffset message and the PHC time stamp event being delivered to user space. This is because the CM4 PHY kernel driver can delay delivering the PHC timestamp by up to 0.25s
* Investigate UBX-TIM-TP further
    * Does it use GPS week numbers when synced to BeiDou?
    * Does it work on non-timing receivers?
    * Does it work on clones?
* Should prefer a message in UBX that provides TAI directly to NMEA.
    * Use UBX-NAV-EOE to wait for right message
    * Need configuration to tell us whether we have UBX-NAV-EOE
* Experiment with different ki/kp coefficients for PI controller
* PI controller starts off with a large integral term, which I suspect is not optimal
* Sanity check with system clock
* Mode where we use system clock rather than GPS messages for time of day (like `ts2phc -s generic`)
* See what we can learn from the Meta time library servo package.
* Be more intelligent about pulse sanity check
   * keep track of standard deviation of pulse interval
   * bounds should make use of max amount of frequency adjustment
* Should the servo do something different if there is more than 1 second since last sample? Interpolate?
* Handle UBX-TIM-TOS on LEA-M8F

## Leap seconds

High priority
* Enable continuous reporting of leap seconds, so we can update the grandmaster
* Testing of whole leap-second chain

Others
* When we get notification of a new leap second, we should store that in a file in /var somewhere, and then read that on startup.
* How should we hook up with NTP-centric kernel support for leap seconds?
  * If the TAI-UTC offset is set in the kernel (presumably from NTP), we could get that and use it as our default.
  * If we get info about the leap second from GPS, we can use that to update the kernel.

## Serial IO

* Problem with writing to serial device that is not connected to anything
   * Attempting to Close (after doing write) hangs for 30s (I guess waiting to drain)
   * Possible fixes (not mutually exclusive)
      * run close in a goroutine with a timeout
      * change closing_wait
      * do not write until some input is received
* Deal with absence of CLOCAL potentially causing open to block
* Cancelled write should complete UBX message but not drain
* Deal with read of 0 from term (caused by timeout)
* Can we be detect when baud rate is too low for information passed?
* Should we support other styles of locking in addition to flock
  * UUCP-style serial-port locking
  * TIOCEXCL - at least use TIOCGEXCL to check if it is already locked
* Workaround kernel/HW [problem](https://github.com/raspberrypi/linux/issues/4453#issuecomment-1709315332) with framing errors on PL011.
  We can use TIOCGICOUNT ioctl to keep track of count of framing errors, and return a separate kind of packet when we detect
  a framing error.


## Scanning

* If we get the start of a frame with a specified length, then we should have a timeout reading the rest of the frame.
* Recognizing RTCM is a bit dangerous when we might have invalid data at the beginning, because it's only
a single byte; we really cannot tell whether we have valid RTCM data until we have validated the checksum; maybe enable RTCM only once we have had a valid message of another type. Maybe have a separate state.
* Support other framing protocols
   * Allystar (same as UBX but uses 0xF1 0xD9)
   * Trimble TSIP
   * SkyTraq binary
   * CASIC (China Aerospace Science & Industry Corporation) Standard Interface Protocol (CSIP) - used by Zhongke Microelectronics

## GPS configuration

Semi-functional currently.

* UBX parsing error probably means multiple readers
* Configure time mode
* Configure time pulse
* Configure GNSS constellations
* More info from MonVer
   * Flash vs ROM
   * Category: timing vs high-precision etc
* Better pairing of of ACKs with messages sent
* Disable unwanted message

## HTTP monitoring

* More cards
   * Leap seconds
   * GPS time mode configuration
   * GPS time pulse configuration
   * GPS constellation/bands configuration
   * GPS antenna status
* Time DoP
* In current-time card, include GPS self-reported accuracy
* In PHC card, having something showing how long ago the sync happened
* Information about visible satellites
   * Could do sky view diagram using SVG or canvas
* Prettier display of current time of day
* Statistics about time-sync quality
    * Maybe query parameter on to give time period over which stats would be summarized
* Provide JSON API (instead of just SSE)
* Should we use this for configuration?
   * Need to think about how we deal with concurrent requests.
   * Would need to deal with authentication
* Can use UBX-NAV-EOE to collect everything from a single navigation epoch into a single message?

## Netlink events

* Integrate experimental carrier program. Need to be able to stop and restart timesync goroutine.
* This can also help when booting with systemd: be able to start up right away, and wait for interface to appear (but we also need to wait for serial device)