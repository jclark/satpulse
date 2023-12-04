# Things to do

## General

High priority

* More documentation
   * config file
   * http monitoring
   * how GPS should be configured
* Control over logging in the non-sdlog case
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
* Allow SIGHUP to reload config: useful for disabling TCP connections.

## Grandmaster management

High priority

* Estimate clock accuracy and dynamically update grandmaster settings. In particular, on startup, update with low accuracy and then improve it as we settle down.
* More sophisticated, configurable detection of whether we are synced (mostly covered by previous point)
* Set a read/write deadline on the conn for the Unix domain socket
* Should update grandmaster settings to a non-synced status when we shutdown.

Others
* Enable configuration of non-synced settings or read current settings at startup
* Support ptp4u by creating dynamic config file
* Take into account GPS reported accuracy in estimating clock accuracy
* Should have proper concept of holdover period/accuracy (particularly with M8F and GPSDOs)

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
    * Does it work (ie provide qErr) on non-timing receivers?
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
* Use sawtooth correction from UBX-TIM-TOS on LEA-M8F
* Take advantage of GPSDO-like features of LEA-M8F for holdover
* Be able to disable sawtooth correction
* Looks like ATGM332D may be using fTOW field of UBX-NAV-TIMEGPS to convey quantization error (rather than solution time)
* We can use CFG-TP5 to get time pulse interval
* Could we use the falling edge to get another sample?

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

## Antenna supervision

Monitor messages about antenna and report when something bad happens (loss of power, jamming, short) via HTTP or logging.

Requires configuration support.

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
- with CM4 we can avoid reading the PHC at a time which is likely to interfere with receiving the time pulse

For hardware timestamping, chrony needs a free running PHC. One way to do this is with virtual clocks. But in case where user is not interested in PTP, it should be possible to generate samples without modifying the PHC (by keeping track of phase/frequency difference between PHC and the correct time).

## Netlink events

* Integrate experimental carrier program. Need to be able to stop and restart timesync goroutine.
* This can also help when booting with systemd: be able to start up right away, and wait for interface to appear (but we also need to wait for serial device)

## Raspberry Pi CM4

Detect when we are on the CM4 and deal with its limitations.

Could get the driver name using ETHTOOL_GDRVINFO or look in sysfs somewhere.

Specifically deal with

- when carrier is lost, things stop working (ties into netlink events)
- we can lose pulses when reading from the PHC (ties into chrony integration)
- delay in reporting timestamp events (makes ubx-tim-tp problematic)