# Things to do

## General

* Rearrange source to properly follow Go conventions
* More testing (include fuzzing)
* Figure out how to run with reduced privileges (Linux capabilities I think)
* Control over logging
* Systemd service file
* Try to get rid of the logctx package.
* More robust handling of panics in goroutines. I think it hangs currently.

## Grandmaster management

* Robustly detect loss of sync
   * Need make periodic calls to the Grandmaster even when there is no valid GPS time message
* Estimate clock accuracy and dynamically update grandmaster settings. In particular, on startup, update with low accuracy and then improve it as we settle down.
* More sophisticated, configurable detection of whether we are synced
* Enable configuration of non-synced settings
* Support ptp4u by creating dynamic config file
* Should we update the grandmaster settings to a non-synced status when we shutdown?
* Set a read/write deadline on the conn for the Unix domain socket

## TCP server

* Test with Lady Heather
* Config for read-only NMEA socket (can be used with ts2phc)
* Config file should specify multiple ports, each with its own properties

## Time sync

* Can we improve how UBX-TIM-TP is handled and the corresponding PulseOffset message? Problem is that on CM4 there can be one then 1 sec between receiving the PulseOffset message and the PHC time stamp event being delivered to user space. This is because the CM4 PHY kernel driver can delay delivering the PHC timestamp by up to 0.25s
* Investigate UBX-TIM-TP further
    * Does it use GPS week numbers when synced to BeiDou?
    * Does it work on non-timing receivers>
    * Does it work on clones?
* Should prefer a message in UBX that provides TAI directly to NMEA.
    * Use UBX-NAV-EOE to wait for right message
    * Need configuration to tell us whether we have UBX-NAV-EOE
* Experiment with different ki/kp coefficients for PI controller
* PI controller starts off with a large integral term, which I suspect is not optimal
* Sanity check with system clock
* Log RMS of offset
* Mode where we use system clock rather than GPS messages for time of day (like `ts2phc -s generic`)
* See what we can learn from the Meta time library servo package.
* Step clock if we get out of sync
* Make correlator and servo configurable from TOML
   * kp, ki constants
   * initial observe time
   * when to step clock
   * various correlator constants
* Be more intelligent about pulse sanity check
   * keep track of standard deviation of pulse interval
   * bounds should make use of max amount of frequency adjustment
* Should the servo do something different if there is more than 1 second since last sample? Interpolate?

## Leap seconds

* Enable continuous reporting of leap seconds, so we can update the grandmaster
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
* Should we support UUCP-style serial-port locking, in addition to flock? Probably not.

## Scanning

* If we get the start of a frame with a specified length, then we should have a timeout reading the rest of the frame.
* Support other framing protocols
   * Allystar (same as UBX but uses 0xF1 0xD9)
   * Trimble TSIP
   * SkyTraq binary
   * CASIC (China Aerospace Science & Industry Corporation) Standard Interface Protocol (CSIP) - used by Zhongke Microelectronics

## GPS configuration

Semi-functional currently.

* Wait for some input before sending messages?
* UBX parsing error probably means multiple readers
* Configure time mode
* Configure time pulse
* Configure GNSS constellations
* More info from MonVer
   * Flash vs ROM
   * Category: timing vs high-precision etc
* Better pairing of of ACKs with messages sent
* Disable unwanted message


## HTTP server

* Provide JSON API for current status
* Human-readable page that repeatedly fetches current state
* Can draw a sky view using JS canvas
* Should we use this for configuration?
   * Need to think about how we deal with concurrent requests.
* Can use UBX-NAV-EOE to collect everything from a single navigation epoch into a single message

## Netlink events

* Integrate experimental carrier program. Need to be able to stop and restart timesync goroutine.


