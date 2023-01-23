Things to make minimal working program

1. Reading extts events
   1. Configure pin for output
   2. Send the ioctl
   3. Read the event
   4. Send over a channel
2. Type to represent instant of time (64-bit int with nanoseconds since 1970 TAI)
   1. convert GPS time to this
   2. convert from a struct timespec
   3. use time.Duration
3. Catch signals and cleanup
4. Command-line option for serial port
5. Correlate pulse edges with GPS readings
6. Adjust PHC
   1. initial adjust phase
   2. initial adjust freq
   3. proportional-integral controller
7. Quantization error

Improvements in existing functionality

* Occasionally ITOW is not an integral number of seconds
* Proper cleanup when wiring up all the objects
* Testing (include fuzzing)
* Make use NMEA (if we don't have UBX)
   * compute checksum
   * parse message
   * handle some PUBX messages
   * deal with leap seconds
* Use system clock
   * sanity check
   * for time of day (like `ts2phc -s generic`)
* Link bounds for pulse sanity check to amount of frequency adjustment
* On F9T, using ubx-tim-tp, tow seems to be wrong unless time base is set using tp5 to GPS
* Experiment with different ki/kp coefficients for PI controller
* PI controller starts off with a large integral term, which I suspect is not optimal
* Catch SIGTERM
* Introduce configuration phase, before syncing
* More info from MonVer
  * Flash vs ROM
  * Category: timing vs high-precision etc
* Deal with read of 0 from term (caused by timeout)
* Improved handling of writes
   * Use separate goroutine
   * Wait until each message has drained before sending next
   * Avoid any race between term.Close and writing

New functionality

1. ser2net functionality
2. Send messages to GPS
   * Enable messages we need
   * Get configuration; verify/adapt
   * Poll for some things (e.g. UBX-MON-VER)
3. Config file
4. HTTP server