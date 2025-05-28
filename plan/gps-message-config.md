# GPS message redesign

The basic idea is to divide messages into the following categories, and then have flags controlling which messages are enabled in each category. The categories are chosen so that GNSS messages typically do not span more than one category

- PVT - relating to the current PVT solution. Flags would say what information the user wants from the messages
  - TimePulse - the time of the time pulse
  - Time - time of navigation solution (not used currently)
  - TAI - want time in TAI (or constant  number of seconds from TAI) rather than UTC
  - FutureLeapSecond - this is the key metadata that satpulsed ideally wants: information about any upcoming leap second (or the last leap second if no upcoming leap second has been announce)
  - Survey - this means the ECEF position and accuracy determined so far as part of survey; in UBX there is a specific message for this, but with other GPSs we might need to use messages to get the ECEF position and accuracy
  - Pos - position from solution by default LLA
  - Vel - velocity from solutio by default in NED
  - ECEF - Position/velocity in ECEF (not used yet)
  -  TDOP - Time DOP (not used yet)
  - PDOP - Position DOP (not used yet)
- Raw - raw data, specifically
   - Observations/measurements, as needed to generate RINEX observations file e.g. UBX-RXM-RAWX
   - Navigation data - messages e.g. for GPS subframe data, as provided by UBX-RXM-SFRBX
   - Ephemeris - UBX doesn't do this, but Unicore and several other protocols have this and it is supported by RTKLIB
 - Satellites - per-satellite information
    - SV - position of each satellite, and signal level of each satellite (with indeterminacy about which signals this means)
    - Signal - information per-SV, and per-signal e.g. signal level of each satellite (uses more bandwidth)
  - Text - messages from the GPS receiver intended for humans to read e.g. UBX-INF-WARNING
  - NMEA - NMEA messages to be enabled
    - RMC
    - GGA
    - GSA
    - GSV
    - also maybe VTG, ZDA and GNS
    - Other - when this flag is missing together with all others, then it will disable NMEA on the port; if this flag only is specified; it will disable specific messages
 - RTCM - RTCM messages to be enabled; GLONASS code phase bias 1230 will be automatically enabled if an MSM message is enabled and GLONASS is enabled
     - MSM4 - enable MSM4 messages for all enabled constellations
     - MSM7 - enabled MSM7 messages for all enabled constellations
     - ARP - enable base station position messages i.e. 1005/1006/1007
     - something about number of seconds betweeen ARP messages maybe

As for messages rates, everything would be 1Hz initially. For PVT, we could add in the future flags for commonly supported rates e.g. 1Hz, 2Hz, 5Hz, 10Hz, 20Hz, 50Hz. This would be for rover use not base station.

In ConfigOptions, we can have a generic `Option` type, where `Option[T] is a bool saying whether T is present plus a T.  Then e.g. ` PVTMsg Option[PVTMsgFlags]`.

For each of these categories, satpulsetool would have an option, which takes a command separated list of flags for that category, with none meaning no flags. If the option is not specified, then messages in that category are not affected. If the option is specified, then the specified set will be enabled, and others will be disabled. So this would be suitable for satpulsed:

```
--pvt-out tp,tai,leap --nmea-out none --sats-out sv
```

although we would probably have a shortcut for that (e.g. `--satpulsed-msg`)

In the TOML file, the syntax would be a list of strings, maybe

```
[gps]
msg.rtcm = ["MSM4", "ARP"]
```

Existing `--nmea` option would mean only NMEA, and would enable RMC, if no nmea-out is specified.

Existing `--binary` option would disable NMEA completely; if `--pvt-out` is not specified will do `--pvt-out pos,vel,time`.

Option summary:
```
--nmea
--binary
--nmea-out rmc,gga,gsa,gsv or none
--pvt-out tp,time,pos,vel,ecef,tai,survey,tdop,pdop
--raw-out obs,eph,nav or none
--rtcm-out msm4,msm7,arp or none
--sats-out sv,sig
```

Decisions:
- user-facing terminology should be out/output rather than msg/message; so option will be `--raw-out` rather than `--raw-msg`
- CLI option syntax doesn't need to be identical to TOML syntax (CLI options can be shorter and TOML is using JSON), but should be generally consistent
- use msg rather than output internally for now
- if the receiver cannot implement something like raw output, then it will report back an error during the config stage

Issues:
 - is the sv/signal flags for satellites right? Maybe it should be SVPos vs signals? If we just a signal level bargraph, then we don't need satellite positions. We can do a skyview without signal levels (we would need to adjust the web UI to handle this)
- do we keep or change current `satellitesOutput` TOML option 
    - we could use e.g. `rtcmOutput = ["MSM4"]`
    - do we really need the sv/signal distinction for satellites in the TOML file? Could just make it a bool rather than a list of flags?


Staging:
1. change current gpsprot.ConfigOptions functionality to new style (no changes to behaviour)
    - EnableTimeMsg bool becomes PVTMsgTimePulse|PVTMsgTAI flag on PVTMsg
    - EnableLeapSecondMsg bool becomes PVTMsgLeapSecond on PVTMSG
    - SatellitesMsg becomes SatellitesMsgSV on SatellitesMsg
   - NMEA property becomes NMEAMsgOther on NMEAMsg
2. SolutionPeriod property would go away, and is instead set whenever any PVT messages are enabled
3. implement raw output feature end-to-end
4. implement correct behaviour of --binary option
   - enable UBX-NAV-PVT by default
   - implement --pvt-out with a few options
   - implement correct PVTOut semantics that disables non-selected PVT messages
5. implement full behaviour of --nmea option
   - be able to disable binary messages
   - be able to enable RMC
