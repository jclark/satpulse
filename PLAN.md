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

* Command-line arg/option for ethernet interface
* Occasionally ITOW is not an integral number of seconds
* Generate multiple samples when initial correlation phase completes (but mark as delayed)
* Use system clock for sanity checking
* Compute checksums for
   * UBX
   * NMEA
* Proper logging strategy (get rid of printf's)
* Proper cleanup when wiring up all the objects
* Testing
* Use NMEA if that's all we have
* Link bounds for pulse sanity check to amount of frequency adjustment
* On F9T, using ubx-tim-tp, tow seems to be wrong unless time base is set using tp5 to GPS
* Experiment with different ki/kp coefficients for PI controller
* Serial stuff should use higher level `term` module

New functionality

1. ser2net functionality
2. Send messages to GPS
   * Enable messages we need
   * Get configuration; verify/adapt
   * Poll for some things
3. Extend ubx reading to deal with variable length messages (have fixed part and repeated part)
4. Config file

