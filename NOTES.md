Major things to to do:
1. Catching signals using channel/context
2. Reading extts events
   1. Configure pin for output
   2. Send the ioctl
   3. Read the event
   4. Send over a channel
3. Type to represent instant of time (64-bit int with nanoseconds since 1970 TAI); figure out whether we need
   CLOCK_MONOTONIC or CLOCK_MONOTONIC_RAW
   1. convert GPS time to this
   2. convert to Go time
   3. convert from a struct timespec
4. Adjust PHC
   1. step
   2. PI servo to adjust freq
5. Do sawtooth correction
6. For rPI, listen to netlink events and stop when there's no carrier

