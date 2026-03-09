# NMEA epoch boundary ambiguity

## Problem

NMEA epoch boundaries are currently detected by time-of-day changes in RMC, GGA, VTG, or ZDA sentences. Sentences that lack a time-of-day field (GSA, GSV) are attributed to whichever epoch is active when they arrive. This works correctly when the receiver emits time-bearing sentences before non-time-bearing ones within each epoch (e.g. RMC, GGA, GSA, GSV), but not when the order is reversed.

If a receiver outputs GSA before RMC/GGA in each epoch, the GSA is attributed to the previous epoch because the time-of-day change that marks the new epoch hasn't been seen yet. There is no delimiter between NMEA epochs in the serial stream -- sentences arrive as a continuous sequence of bytes with no guaranteed pause or marker at the epoch boundary. This makes the association of non-time-bearing sentences inherently ambiguous when sentence order is unknown.

The same ambiguity affects GSV and any other sentence without a time-of-day field. For satellite data (GSV/GSA satellite IDs), the existing `satellitesBuffer` uses its own accumulation and flush lifecycle that is independent of the nav epoch, so the impact is limited to data written directly to the `NavEpoch`: currently FixDim and DOPs from GSA.

In practice, the most common NMEA sentence order places GGA and RMC early in the epoch (before GSA/GSV), so the current approach works for most receivers. The ambiguity is most visible at startup (when no epoch exists yet) and with receivers that emit GSA/GSV before any time-bearing sentence.

## Timing as a disambiguation hint

Serial-stream timing can help resolve the ambiguity. GNSS receivers typically emit each epoch's sentences in a burst, with a pause between epochs corresponding to the navigation solution computation time. The inter-epoch gap is usually tens to hundreds of milliseconds, while the intra-epoch gap between consecutive sentences is small (limited by baud rate).

By observing the arrival time of each sentence (or byte), an idle gap above a threshold suggests an epoch boundary. This would allow non-time-bearing sentences to be grouped with their epoch regardless of sentence order, since the burst structure reveals which sentences belong together.

Timing must be treated as a hint to resolve ambiguity, not as an authoritative epoch marker. The NMEA specification makes no guarantees about inter-sentence timing, and receivers are free to vary their transmission patterns. Specific limitations:

- The idle gap depends on baud rate and the number/size of enabled sentences. At high baud rates with few sentences, the gap may be small relative to timing jitter.
- Some receivers may not produce a clean pause between epochs (e.g. if computation overlaps with transmission of the previous epoch's sentences).
- The threshold must be calibrated or adaptive, adding complexity.

The time-of-day change in RMC/GGA/VTG/ZDA remains the authoritative epoch boundary signal. Timing information should only influence how non-time-bearing sentences are associated with an epoch when the time-of-day boundary has not yet been observed.

## GLL as an additional epoch participant

Of the standard NMEA sentences defined in `NMEAMsgFlags` (RMC, GGA, GSA, GSV, ZDA, VTG, GLL), all time-bearing sentences participate in epoch boundary detection except GLL. GLL (Geographic Position - Latitude/Longitude) carries a time-of-day field (field 4) and latitude/longitude, but is not currently parsed by the `nmea` package. Adding GLL parsing would provide another time-bearing sentence that can establish or confirm epoch boundaries, reducing the window in which non-time-bearing sentences like GSA are ambiguous. This is a straightforward addition independent of the timing-based disambiguation.

## Implementation strategy

### Add GLL participation

Add GLL parsing and make its time-of-day participate in epoch boundary detection.

This is small, low-risk change that reduces ambiguity window on receivers that emit GLL.
However, it does not fully solve reversed ordering where GSA/GSV still arrive before all time-bearing sentences.

### Timing-hint plus pending untimed buffer

Buffer the GSA-derived NavEpoch fields in the satellitesBuffer, and commit them when the satellites buffer is flushed.
This allows these NavEpoch fields to leverage the timing hints that satellite buffer flushing uses.

## Limitations

If VTG (or any sentence that has to be emitted as a Msg without delay and does not have a time of day in it) starts a burst, then it will be systematically assigned to the wrong epoch. A VTG starting a burst that occurs while epoch n is active cannot safely start epoch n+1, because it might be followed by a sentence with time of day for epoch n (because NMEA makes no guarantees about bursts).
