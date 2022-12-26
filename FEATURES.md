

## Better syncing

### Time pulse error

UBX-TIM-TP gives the quantization error of time pulse: i.e. how many nanoseconds the PPS is off
from the top of the second.

(I think the fTOW field of UBX-NAV-TIMEGPS has similar information.)

### Get time from GPS in TAI

GPS works internally using a timescale that is a fixed offset from TAI. The current UTC offset
is provided as additional metadata. PTP works in the same way. The UBX protocol can give us the time
in TAI, whereas NMEA provides it only in UTC. By using UBX, we can sync the PHC to GPS without needing
to know the current UTC offset (which is sent only every 12 minutes or so).

### Deal automatically with NIC limitations

We can see what NIC we have and know about its limitations.

- for rPI CM4 deal with
   - loss of carrier (listen to netlink events)
   - losing pulses when reading from PHC
- for i210 deal with timestamping both edges (we can use UBX to get the length of the time pulse)

### Deal with GPS flakiness

Be robust in the presence of bad/missing pulses from GPS.

Using a higher-level language with better concurrency features should make this easier.

## Dynamically adjust grandmaster settings

The grandmaster settings advertised by the PTP daemon need to be dynamically adjusted so they correspond to the current synchronization state. This ensures

- clients will appropriately fallback to an alternative grandmaster
- clients that use something like fbclock to estimate time window will get sensible results
- clients that use chrony with `phc2sys -E nptshm` will ignore a grandmaster than has lost synchronization 

### GPS lock

Do we have a GPS lock? When we do, then clockClass in grandmaster settings should be 6. But if we lose the
lock, then we should change it.

### Time accuracy

UBX-NAV-TIMEGPS gives a time accuracy estimate in nanoseconds (in the tAcc field).
This could feed into the clockAccuracy field in grandmaster settings

### Leap seconds

UBX-NAV-TIMEGPS gives us the UTC-GPS offset. We can feed this into the currentUtcOffset of grandmaster settings

UBX-NAV-TIMELS gives us information about any upcoming scheduled leapsecond event. We can feed this into leap59 and leap61 of grandmaster settings.

### Holdover

If we are talking to a GPSDO, then when we lose a GPS lock we should go into holdover mode for some period of time
and adjust grandmaster settings accordingly.

M8F has some limited features like this. Need to explore further.

## Monitoring

We can collect additional information using UBX and make it available via

* logging
* through an HTTP interface

### Antenna supervision

UBX-MOD-RF allows monitoring of antenna status. We can collect stats and also warn if something changes
for the worse in the antenna status (loss of power, jamming, short).

### GPS signal quality

Keep track of how many satellites in view from various constellations and what the signal quality is.

## Configuration

We should be able to do some configuration of the GPS, in particular to enable the periodic messages that we understand.

Advanced users who have manually configured their GPS should be able to turn this off.

I am not sure how far to go here.

Maybe leave some of this to a separate tool such as https://github.com/phkehl/ubloxcfg

## Enable TCP connection to serial port

U-blox's u-center tool allows a network connection to a GPS on a remote machine. Typically this would be used in conjunction with ser2net, which would be used on the remote machine to expose the serial port over TCP.

We could include this functionality so that we can configure and monitor the GPS using UBX tool at the same time as we are using it for time sync. In this mode, we would do some initial configuration of the GPS, but thereafter we would not send any messages to the GPS; instead we would monitor the messages being sent to the remote tool and interpret those we know about.

