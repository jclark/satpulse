# Things to do

These are things that have not yet been made into GitHub issues.

## General

High priority

* More documentation
   * http monitoring
   * how GPS should be configured
* Control over logging in the non-sdlog case
* Better error for no PPS
* Stats about time sync quality
   * Maximum offset
   * RMS offset
   * Something on frequency
   * Did we miss any pulses?

Others
* More testing (include fuzzing)
* Figure out how to run with reduced privileges (Linux capabilities I think)
* Allow SIGHUP to reload config: useful for disabling TCP connections.

## Grandmaster management

High priority

* Estimate clock accuracy and dynamically update grandmaster settings. In particular, on startup, update with low accuracy and then improve it as we settle down.
* More sophisticated, configurable detection of whether we are synced (mostly covered by previous point)

Others
* Enable configuration of non-synced settings or read current settings at startup
* Support ptp4u by creating dynamic config file
* Take into account GPS reported accuracy in estimating clock accuracy
* Should have proper concept of holdover period/accuracy (particularly with M8F and GPSDOs)

## TCP server

* Test with Lady Heather
* Control over IPv4/IPv6

## Servo

High priority
* Better handle the case where we lose sync: may need to step clock
* Log some statistics about the servo (similar to what ptp4l does)
* Make servo configurable from TOML
   * kp, ki constants
   * initial observe time
   * when to step clock

Others

* Experiment with different ki/kp coefficients for PI controller
* PI controller starts off with a large integral term, which I suspect is not optimal
* Should the servo do something different if there is more than 1 second since last sample? Interpolate?
* See what we can learn from the Meta time library servo package.
* See what we can learn from NTP standard

## Combine

* TimeMsg method should use gpsprot.TimeMsg
  * runner.go should just pass time messages on
* Implement auto-detection of number of edges per pulse
* Add some package docs
* We should give an error if we have a 50% duty cycle and two edges per pulse
* Improve how we generate TAI from UTC time messages
  * Should prefer a message in UBX that provides TAI directly to NMEA
  * Should use GPS receiver's idea of what the leap second offset is
* More tests
  * upgrading sample
  * waiting for pulse correction
  * unit tests for secMsgList
  * test that we are emitting the sample at the right time (i.e. immediately after pulse)
  * test with missing, extra pulses/messages
* Configurable option to specify pulse width
* Perform measurements to make sure we are applying the quantization error in the right direction
  * Configurable option not to use pulse offset/correction
* Sanity check first sample against system clock and log warning
  * Can we figure out when system clock is supposed to be set?
* Add separate optional stage to filter outliers (no need to use this when generating samples for chrony)
  * This probably needs to keep track of what frequency adjustments we have made
  * Distinguish choosing the right second (which is quite coarse-grained) from filtering outliers
  * Need to investigate the right statistical approach
  * Maybe only do this when we are in sync

Lower priority
* Bulletproof against invalid GPS message sequences
* Investigate UBX-TIM-TP further
    * Does it work (ie provide qErr) on non-timing receivers?
    * Does it work on clones?
* Currently we require both navigation message rate and pulse rate to be 1Hz? Is there any point in relaxing this?
* Could we work with just UBX-TIM-TP? Should be OK with one edge per pulse.
* Can we leverage UBX-NAV-EOE (for end of navigation era)?
* Looks like ATGM332D may be using fTOW field of UBX-NAV-TIMEGPS to convey quantization error (rather than solution time)
* More flexible approach to edge detection
  * With 50% duty cycle could generate two samples
  * Problem is that it makes it harder to align pulses
* Mode where we use system clock rather than GPS messages for time of day (like `ts2phc -s generic`)
  * Could just generate time messages from system clock.
  * This should work reasonably well now we are mostly basing things on previous sample
* Can we support idea of a UTC correction that would allow clients to align to UTC rather than GNSS time?
  * GPS/BeiDou/Galileo all have concept of steering their system time to a UTC variant.
  * But we align the pulse to GNSS system time (seems to work better)

## Leap seconds

High priority
* Testing of whole leap-second chain
* Make use of UTC-TAI offset in gpsmsg.Time (from GPS receiver through to ptp4l)

Others
* When we get notification of a new leap second, we should store that in a file in /var somewhere, and then read that on startup.
* Include source of leap second information in gpsmsg leap second message
* Work with a leap-seconds.list format file
  * fetch from specified URL
  * use to initialize current leap second if up to date
  * update from leap second notifications
* We could tell the GPS about stored leap seconds using UBX-MGA-GPS-UTC (but this has other stuff that is hard to specify)
* How should we hook up with NTP-centric kernel support for leap seconds?
  * If the TAI-UTC offset is set in the kernel (presumably from NTP), we could get that and use it as our default.
  * If we get info about the leap second from GPS, we can use that to update the kernel.

## Serial port network access

* There's a bug: we get a write error when user disconnects
  * Same when using `satpulsetool --socket``

## Serial IO

* Problem with writing to serial device that is not connected to anything
   * Attempting to Close (after doing write) hangs for 30s
   * Why does happen? If there's no flow control, the UART should transmit the data even if the serial device is not connected.
   * Possible fixes (not mutually exclusive)
      * run close in a goroutine with a timeout
      * change closing_wait by using the same Linux API as setserial (TIOCSSERIAL/TIOCGSERIAL and serial_struct); but this probably needs root
      * (current approach) do not write until some input is received
      * make sure we aren't restoring serial port settings to something with flow control
* Deal with absence of CLOCAL potentially causing open to block
* Cancelled write should complete UBX message but not drain
* Can we be detect when baud rate is too low for information passed?
* Should we support other styles of locking in addition to flock 
  * UUCP-style serial-port locking
  * TIOCEXCL - at least use TIOCGEXCL to check if it is already locked
* Investigate kernel/HW [problem](https://github.com/raspberrypi/linux/issues/4453#issuecomment-1709315332) with framing errors on PL011. Possibly similar problem on M8T connnected to physical serial port.

## Scanning

* Recognizing RTCM is a bit dangerous when we might have invalid data at the beginning, because it's only
a single byte; we really cannot tell whether we have valid RTCM data until we have validated the checksum; maybe enable RTCM only once we have had a valid message of another type. Maybe have a separate state. This is alleviated by our timing out in the middle of a packet.
* Support other framing protocols
   * Allystar (similar to UBX but uses 0xF1 0xD9)
   * Trimble TSIP
   * SkyTraq binary
   * CASIC (China Aerospace Science & Industry Corporation) Standard Interface Protocol (CSIP) - used by Zhongke Microelectronics

## GPS configuration

* Bring legacy (<= UBX gen8) configuration to parity with cfgval configuration
   * choose GNSS time message based on primary GNSS constellation
   * implement setting GNSS constellation
   * implement baud rate property
   * saving to flash
   * poll only the things we need
* Figure out how to make U-blox receivers start a new survey, when they have already done one
* Log ongoing and completed survey progress
* Finish mapping from CfgVals to from ConfigMap
* Support for setting ECEF fixed position and accuracy
   * satpulsed TOML properties: `posAcc = 10`, `pos=[1,2,3]`
   * `satpulsetool config --pos X,Y,Z --pos-acc 10`
* Allow daemon TOML configuration of important things that we cannot determine for the user
   * primary GNSS constellation
   * antenna delay
* TOML configuration to control of what kind of configuration satpulsed does to the receiver
    * disable sending any message to receiver
    * disable use of UBX
    * disable modifying receiver configuration
    * forceUBXLegacyCfg
* TOML configuration for pulseWidth/dutyCycle
    * use in conjunction with no configuration for input to correlator
* Option for satpulsetool to get survey result
* satpulsetool should use NDELAY TCP option
* Need to check we got something back from a poll (not just check the ACK)
* Message enablement should be in ConfigMap rather than ConfigOptions so we can turn messages off
   * `satpulsetool config --nmea` should probably turn off time-related UBX messages
   * Do we model message rate in the property?
   * How do we deal with different kinds of time message (per GNSS, UTC, TP, TOS)?
   * In particular, how do we deal with GNSS messages for non-primary GNSS? Do we turn them off?
   * This ties into to how configuration should tell syncer about what message to use.
* Pay attention to version in CFG-UBX-TP5
* Make satpulsed recover better from configuration errors
* Add `satpulsetool config --binary` option to disable NMEA and enable UBX Time/LeapSecond/Survey messages
* Might need to set CFG-NAVSPG-WKNROLLOVER
* Add `satpulsetool config` option to show the configuration e.g. `--show`. Requires extending gpsprot interface.
* Configure UBX message interface

## Monitoring

* Antenna monitoring (loss of power, jamming, short)
  * report via HTTP or logging
  * requires configuration support
* Messages from receiver
  * NMEA TXT messages
  * UBX-INF-*
* On CM4, logging should account for normality of lost pulses

## HTTP monitoring

* More cards in HTML interface
   * Leap seconds
   * GPS time mode configuration
   * GPS time pulse configuration
   * GPS constellation/bands configuration
* Time DoP from UBX-NAV-DOP
* In current-time card, include GPS self-reported accuracy
* In PHC card, having something showing how long ago the sync happened
* Information about visible satellites
   * Number of satellites
   * Satellite elevations
   * Could do sky view diagram using SVG or canvas
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

Feed samples into chrony using the chrony SOCK refclock.

This is better than using PHC refclock because:
- we can feed samples only when PHC is synchronized
- SOCK samples include leap status flag so we can feed in leap second info.
- with CM4 we can avoid reading the PHC at a time which is likely to interfere with reading the time pulse

For hardware timestamping, chrony needs a free running PHC. One way to do this is with virtual clocks. But in case where user is not interested in PTP, it should be possible to generate samples without modifying the PHC (by keeping track of phase/frequency difference between PHC and the correct time).

We can also run the other way: if PHC is not synchronized, then we read samples from the system clock and
feed them into the PHC and change the clock type appropriately (assuming system clock is controlled with NTP).
