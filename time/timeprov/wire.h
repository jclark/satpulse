// Wire format for the pipe-timeprov named pipe.
//
// A producer measures the offset of the system clock against a
// reference clock and writes one PipeSample per measurement to the
// pipe, as a single message-mode write. All fields are little-endian
// and use the units and epoch of the w32time time provider interface
// (timeprov.h): 100 ns intervals, NT epoch 1601-01-01 UTC.
//
// The format is versioned through the magic number: an incompatible
// change replaces the magic ("TPV1" -> "TPV2"), so a reader rejects
// records from the wrong version instead of misparsing them.

#ifndef PIPE_TIMEPROV_WIRE_H
#define PIPE_TIMEPROV_WIRE_H

#include <stdint.h>

// Default pipe path; a provider can override it with the PipeName
// registry value (see README).
#define PTP_PIPE_NAME_DEFAULT L"\\\\.\\pipe\\pipetimeprov"

// "TPV1" read as a little-endian uint32.
#define PTP_MAGIC 0x31565054u

// Leap values follow timeprov.h nLeapFlags.
#define PTP_LEAP_NONE 0
#define PTP_LEAP_INSERT 1
#define PTP_LEAP_DELETE 2
#define PTP_LEAP_UNSYNC 3

typedef struct PipeSample {
    uint32_t magic;       // PTP_MAGIC
    uint8_t leap;         // PTP_LEAP_*
    uint8_t reserved[3];  // must be zero
    uint64_t sysTime;     // system clock at the measurement, 100 ns since 1601 (FILETIME-compatible)
    int64_t offset;       // true time minus system clock, in 100 ns
    uint64_t dispersion;  // measurement error, in 100 ns
} PipeSample;

// The struct above has no compiler-dependent padding: 4+1+3 bytes,
// then three naturally aligned 8-byte fields.
typedef char ptp_assert_sample_size[sizeof(PipeSample) == 32 ? 1 : -1];

#endif // PIPE_TIMEPROV_WIRE_H
