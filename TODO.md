# Things to do

## General

* Rearrange source to properly for Go conventions
* More testing (include fuzzing)
* Figure out how to run with reduced privileges (Linux capabilities I think)
* Option for extts pin index
* Need layer the turns UBX messages into something more convenient to work with, and UBX-independent

## Time sync

* Occasionally ITOW is not an integral number of seconds
  * need to round (or is it truncate?)
  * also tim-tp?
* Experiment with different ki/kp coefficients for PI controller
* PI controller starts off with a large integral term, which I suspect is not optimal
* Sanity check with sytem clock
* Link bounds for pulse sanity check to amount of frequency adjustment
* Log RMS of offset
* Mode where we use system clock rather than GPS messages for time of day (like `ts2phc -s generic`)

## NMEA

* Fall back to NMEA (if we don't have UBX)
* RMC or ZDA
* What do we do about talker id? Can we get multiple copies for different talker?
* Requires leap-second support: could use PUBX, but not much point

## Leap seconds

* Use
  * for grandmaster settings utc-offset, leap59, leap61
  * use with NMEA messages
* Leap second data model:
   * date of last scheduled leap second (either in the past or up to 6 months in the future)
   * UTC offset before that
   * UTC offset after that
* Store that in a file somewhere (in /var/run)
* How does that hook up with NTP-centric kernel support for leap seconds?

## Serial IO

* Write to non-existent serial device waits indefinitely for output to drain (don't understand why)
* With non-existent serial device hangs in term.Restore (waiting for output to drain, I think)
* Maybe better to roll our own term package
* Cancelled write should complete UBX message but not drain
* Deal with read of 0 from term (caused by timeout)
* Option for baud rate
* Can we be detect when baud rate is too low for information passed?
* Serial port locking (flock seems the best way)

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

Does Facebook have something we can reuse?

## HTTP server

* Provide JSON API for current state
* Human-readable page that repeatedly fetches current state
