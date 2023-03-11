# Things to do

## General

* Think about right repo name
   * gpstime
   * gpspulse
   * gps4ptp
   * go-gps-time
   * gps-time
   * go-time
   * go-time-sync
   * gpsclock
* Rearrange source to properly follow Go conventions
* More testing (include fuzzing)
* Figure out how to run with reduced privileges (Linux capabilities I think)
* Control over logging
* Systemd service file

## TCP server

* Test with Lady Heather
* Config for read-only NMEA socket (can be used with ts2phc)
* Config file should specify multiple ports, each with its own properties

## Time sync

* Handling of UBX-TIM-TP is over-complex: if we have the time pulse, we can get the atomic time from that message; no need to use separate message with time
    * Does it use GPS week numbers when synced to Beidou?
    * Use UBX-NAV-EOE to wait for right message
* Should prefer a message in UBX that provides TAI directly to NMEA.
* Experiment with different ki/kp coefficients for PI controller
* PI controller starts off with a large integral term, which I suspect is not optimal
* Sanity check with system clock
* Step clock if we get out of sync
* Link bounds for pulse sanity check to amount of frequency adjustment
* Log RMS of offset
* Mode where we use system clock rather than GPS messages for time of day (like `ts2phc -s generic`)
* See what we can learn from the Meta time library servo package.
* Make correlator and servo more configurable.
* Be more intelligent about pulse sanity check
   * keep track of standard deviation of pulse interval
   * bounds should make use of max amount of freq adjustment

## Leap seconds

* Use
  * for grandmaster settings utc-offset, leap59, leap61
  * with messages that report time in GMT
* Leap second data model:
   * date of last scheduled leap second (either in the past or up to 6 months in the future)
   * UTC offset before that
   * UTC offset after that
* Store that in a file somewhere (in /var/run)
* How does that hook up with NTP-centric kernel support for leap seconds?
* Get current leap second from system.

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

## PTP management client

Does Facebook have something we can reuse? (No: they don't implement a full client.)
Key info
* Do we have a fix? Is the fix valid? (We will lose the time pulse if we don't have a valid fix.)
* Leap seconds: current TAI offset, date of next announced leap second
* Accuracy
   * as reported by GPS
   * offsets between PHC and time pulse

## HTTP server

* Provide JSON API for current status
* Human-readable page that repeatedly fetches current state
* Can draw a sky view using JS canvas
* Should we use this for configuration?
   * Need to think about how we deal with concurrent requests.
* Can use UBX-NAV-EOE to collect everything from a single navigation epoch into a single message

## Netlink events

* Integrate experimental carrier program. Need to be able to stop and restart timesync goroutine.


