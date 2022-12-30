Things to make minimal working program

1. Reading extts events
   1. Configure pin for output
   2. Send the ioctl
   3. Read the event
   4. Send over a channel
2. Type to represent instant of time (64-bit int with nanoseconds since 1970 TAI)
   1. convert GPS time to this
   2. convert from a struct timespec
   3. use duration
3. Catch signals and cleanup
4. Command-line args (at least for serial)
5. Correlate pulse edges with GPS readings
   * work with one edge per pulse
   * work with two edges per pulse
6. Adjust PHC
   1. initial adjust phase
   2. initial adjust freq
   3. proportional-integral controller

Improvements in existing functionality

1. Compute checksums for
   1. UBX
   2. NMEA
2. Proper logging strategy (get rid of printf's)
3. Proper cleanup when wiring up all the objects
4. Testing
5. Use NMEA is that's all we get

New functionality
1. Quantify error
2. Send messages to GPS
   * Enable messages we need
   * Get configuration; verify/adapt
3. Extend ubx reading to deal with variable length messages (have fixed part and repeated part)
4. Config file
5. ser2net functionality
