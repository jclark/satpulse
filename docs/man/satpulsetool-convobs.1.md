# NAME

satpulsetool-convobs - convert GNSS observation data

# SYNOPSIS

**satpulsetool** [*global options*] **convobs** [**\-h**\|**\-\-help**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-o**\|**\-\-output** *path*] [**\-H**\|**\-\-header\-file** *path*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-r**\|**\-\-from** **raw**\|**ubx**\|**rtcm**\|**uncb**\|**unca**\|**rinex**\|**obsj**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-packet\-log**] [**\-\-to** **rinex**\|**obsj**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-date** *YYYYMMDD*\|**\-\-recent**\|**\-f**\|**\-\-date\-from\-filename**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-interval** *seconds*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-rinex\-version** *version*] [**\-\-program** *name*] [**\-\-run\-by** *name*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-antenna** *type*] [**\-\-approx\-pos** *X,Y,Z*] [**\-\-comment** *text*]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-rtcm\-strict\-prr**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-\-ubx\-slip\-threshold** *n*]\
&nbsp;&nbsp;&nbsp;&nbsp;*file*...

# DESCRIPTION

The main purpose of the **satpulsetool** **convobs** command is to convert raw observation data emitted by a GNSS receiver
into a RINEX observation file, which can be sent to a PPP post-processing service such as CSRS-PPP in order to determine
the precise position of the receiver.

Currently, the following raw observation data formats are supported:

* u-blox UBX-RXM-RAWX
* Unicore OBSVM (in either binary or ASCII format)
* RTCM MSM7

The *convobs* command also supports a JSON Lines format called `obsj`,
designed for convenient processing of observation data with modern tools such as **jq**.
This format is supported for both input and output.

At least one *file* argument is required; **-** means standard input.
The output is produced by converting all of the input files in order.
Output is written to standard output unless **\-o** or **\-\-output** is specified.

The input and output formats are determined by the **\-\-from** and **\-\-to** options.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help for the **convobs** command.

**\-o**, **\-\-output** *path*
: Write output to *path*.

**\-r**, **\-\-from** *format*
: Select the input format. The default is **raw**.
The following formats are supported:

  **ubx**
  : u-blox UBX-RXM-RAWX binary messages

  **rtcm**
  : RTCM 3.x MSM7 messages; RTCM messages with metadata are used to generate header fields

  **uncb**, **unca**
  : Unicore OBSVM messages in binary or ASCII format

  **raw**
  : Auto-select between the above formats based on which kind of packet occurs in the stream first

  **rinex**
  : RINEX format

  **obsj**
  : `obsj` format

**\-\-packet\-log**
: Read a SatPulse JSONL packet log.
This cannot be used with a **\-\-from** option of **rinex** or **obsj**.

**\-\-to** **rinex**\|**obsj**
: Select the output format.  The default is **rinex**.

**\-\-interval** *seconds*
: Decimate observations to the specified interval.
The value must be at least 1 second and must divide one day exactly.
The default is 0, which disables decimation.

## RTCM week inference

An RTCM MSM7 observation message includes the time of the observation relative to the start of the week,
but does not say what week it is.
With **\-\-packet\-log**, the timestamps in the packet log are used to resolve the date.
When **\-\-packet\-log** is not used, the following options control how the week is determined.

**\-\-recent**
: Assume the time of all observations is within the last week.

**\-\-date** *YYYYMMDD*
: Assume the first observation occurs on the specified date.

**\-f**, **\-\-date\-from\-filename**
: Assume the name of the file includes the date in `YYYYMMDD` form.

In the absence of any of the above options, **\-\-recent** will be assumed, but a warning will be given.
This assumption will not be made when the file modification time is older than one week.

## Metadata options

These options set RINEX header metadata.
Command-line metadata options override values from the TOML header file.

**\-H**, **\-\-header\-file** *path*
: Read RINEX header metadata from a TOML file.
Unknown fields are errors.
The file may set any supported metadata field, including fields without a dedicated command-line option.

**\-\-rinex\-version** *version*
: Set the RINEX observation format version.
The default is `3.04`.

**\-\-program** *name*
: Set the RINEX program field.
The default is derived from the SatPulse version.

**\-\-run\-by** *name*
: Set the RINEX run-by field.
The default is the current user name.
Use an empty value to suppress the default.

**\-\-antenna** *type*
: Set the RINEX antenna type field.

**\-\-approx\-pos** *X,Y,Z*
: Set the RINEX approximate antenna position as Earth-Centered, Earth-Fixed coordinates in meters.

**\-\-comment** *text*
: Add a RINEX comment line.
This option may be repeated.
If any **\-\-comment** option is supplied, comments from the TOML header file are ignored.

## Format-specific options

**\-\-rtcm\-strict\-prr**
: Use the RTCM standard sign for MSM PhaseRangeRate when computing Doppler.
By default, **convobs** uses the sign needed by common RTCM logs.
This option is valid only with **raw** or **rtcm** input.

**\-\-rtcm\-omit\-zero\-doppler**
: Omit RTCM MSM Doppler observations with numeric value zero.
By default, **convobs** preserves explicit zero Doppler values.
This option is valid only with **raw** or **rtcm** input.

**\-\-ubx\-slip\-threshold** *n*
: Set the UBX-RXM-RAWX `cpStdev` index that marks a cycle slip.
The default is 15.
This option is valid only with **raw** or **ubx** input.

# HEADER FILE FORMAT

The file specified by **\-\-header\-file** is a TOML file.
It can contain the following keys:

* `version` - string for the version field of `RINEX VERSION / TYPE`
* `run.program` - string for the program field of `PGM / RUN BY / DATE`
* `run.by` - string for the run-by field of `PGM / RUN BY / DATE`
* `run.date` - datetime for the date field of `PGM / RUN BY / DATE`
* `comment` - string or array of strings for `COMMENT` records
* `marker.name` - string for `MARKER NAME`
* `marker.number` - string for `MARKER NUMBER`
* `marker.type` - string for `MARKER TYPE`
* `observer` - string for the observer field of `OBSERVER / AGENCY`
* `agency` - string for the agency field of `OBSERVER / AGENCY`
* `receiver.number` - string for the serial number field of `REC # / TYPE / VERS`
* `receiver.type` - string for the receiver type field of `REC # / TYPE / VERS`
* `receiver.version` - string for the firmware version field of `REC # / TYPE / VERS`
* `antenna.number` - string for the serial number field of `ANT # / TYPE`
* `antenna.type` - string for the antenna type field of `ANT # / TYPE`
* `approxPosition` - array of three numbers for `APPROX POSITION XYZ`, in meters
* `antennaDelta` - array of three numbers for `ANTENNA: DELTA H/E/N`, in meters
* `interval` - number for `INTERVAL`, in seconds
* `leapSeconds` - integer for `LEAP SECONDS`, in seconds

For example:

```
observer = "Jane Smith"
receiver.type = "ZED-F9P"
```

# OBSJ FORMAT

An `.obsj` file is a JSON Lines file.
Each line is either an observation record or a metadata record.
An observation record has a `t` field.
A metadata record does not have a `t` field.
It uses the JSON form of the header metadata fields described above; dotted TOML keys are represented as nested JSON objects.

Observation records can contain the following fields:

* `t` - string giving the observation time in GPST as an ISO 8601 date-time without a time zone designator; required
* `sat` - string giving the RINEX satellite identifier, such as `G03`; required
* `sig` - string giving the RINEX signal identifier, such as `1C`; required
* `frq` - integer giving the GLONASS FDMA frequency channel
* `pr` - number giving pseudorange in meters
* `cp` - number giving carrier phase in cycles
* `do` - number giving Doppler in Hz
* `cn0` - number giving carrier-to-noise density in dB-Hz
* `lli` - integer giving the RINEX loss-of-lock indicator
* `ssi` - integer giving the RINEX signal strength indicator

For example:

```
{"t":"2025-12-17T08:14:06.0080000","sat":"G07","sig":"1C","pr":23956830.529584773,"cp":125893980.17237933,"do":2059.716796875,"cn0":34}
```

# EXAMPLES

Convert a packet stream using auto-detection:

    satpulsetool convobs raw.bin | gzip >20260523.obs.gz

Convert a packet log using auto-detection:

    satpulsetool convobs --packet-log /var/log/satpulse/packet.ttyUSB0.jsonl >ttyUSB0.obs

Convert a u-blox packet stream:

    satpulsetool convobs -r ubx -o f9t-20260523.obs f9t-20260523.ubx

Convert a RTCM packet stream, getting the date from the filename:

    satpulsetool convobs -r rtcm -f -o f9p-20260523.obs f9p-20260523.rtcm

Convert `.obsj` to RINEX:

    satpulsetool convobs --from obsj -o um980.obs um980.obsj

Convert RINEX to `.obsj`:

    satpulsetool convobs --from rinex --to obsj -o um980.obsj um980.obs

# SEE ALSO

**satpulsetool(1)**, **satpulsetool-gps(1)**, **satpulsed(8)**, **jq(1)**
