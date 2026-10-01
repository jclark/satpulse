# Serial corruption on speed change

## Problem

Configuring a receiver over a serial port sometimes fails because bytes are sent or received at the wrong speed just after the port is opened and its speed is changed. Two distinct failures were found, both triggered by changing the serial speed while the receiver is transmitting.

The first is common: the first few bytes of a command written right after the speed change go out at the old speed. The receiver never sees a well-formed command, so the setting is silently not applied. Reading is affected the same way, but that was already known and tolerated as a few garbage bytes at the start of a session.

The second is rare and needs a quick reopen: the whole session, both transmit and receive, runs at the old speed and does not recover. Reads return framing errors and roughly the old-speed fraction of each burst; writes go out entirely at the old speed. Only closing and reopening, or changing the speed away and back on an idle line, clears it.

This is what makes the systest message-file configuration on a Raspberry Pi intermittently fail: a per-start `gpscfg` script closes the port and `satpulsed` reopens it moments later, mid-burst, changing the speed each time.

## Root cause

The UART has a single baud-rate generator shared by the transmitter and the receiver, and its divisor plus the frame format (word length, parity, stop bits) form one internal register that the hardware only commits when the line-control register is written. The ARM PL011 documentation forbids writing that register while the UART is enabled or while a character is completing.

The Linux driver violates this on open. It enables the receiver with the previous session's divisor, and the serial core then rewrites the divisor and line control immediately afterwards. If a character is arriving at that moment, the update lands under a character in progress. The visible registers read back as the new speed, but the hardware keeps running from the old divisor. Because transmit shares that divisor, anything transmitted before the divisor actually switches also goes out at the old speed.

This is a generic PL011 issue, not specific to one board. It was reproduced on a Compute Module 4 (BCM2711), a CM5 and a Raspberry Pi 5 (RP1), on stock and custom kernels. A separate kernel investigation proposes a driver fix; the changes here are the userspace mitigation and do not depend on it.

## Evidence

The failure needs both a real divisor change and the port's own receiver mid-burst when the change is written. With the receiver idle at the change, or with the divisor unchanged, no corruption occurred. Opening early in a long burst, so that much reception remains after the open, fails most of the time and often sticks the whole session; opening near the end of a burst is almost always clean.

The lost transmit bytes go out at the old speed, confirmed by both the received bytes and by the transmit drain taking the old-speed time. The number lost is one to three characters, not a fixed count or a fixed duration; it is how many bytes were pushed into the transmit FIFO during the short window between the speed change and the divisor actually switching.

Delaying the first write after the speed change by two character times computed from the old speed eliminated the transient loss across many writes at several old speeds. The only residual failures under that delay were stuck sessions, which no write delay can help.

## Fix

Two independent changes. Together they cover the common case; the rare stuck-session case is left for later work.

### Restore only the interpretation, preserve the physical link

Change the close policy so the port is left describing the real state of the link, and only the current reader's private view is put back.

There is a two-way distinction between serial settings:

- Physical reality of the link: speed, word length, parity, stop bits, and hardware flow control. These are facts about the wire and the device on it, true whether or not any program has the port open. Leave them at their current values on close.
- The reader's lens: raw versus cooked mode, VMIN and VTIME, the input and output mapping flags, echo, line discipline. These are how one program chooses to parse the byte stream. Restore them to what was found, as a good citizen.

Today the close restores everything, including the speed, so the port is left at the kernel default (usually 9600). That default is a placeholder the tty layer invents, not the truth of a link to a receiver running at 115200. Leaving the real speed and frame in place means a back-to-back reopen of the same receiver changes neither, so no divisor change occurs and neither failure can arise.

Hardware flow control is off for now because RTS/CTS is not supported, and on the common receivers the modem-control lines carry PPS rather than flow control. It is part of the physical set, so it is left off rather than restored to whatever was found. Septentrio COM ports can be configured for real RTS/CTS flow control; if that is ever supported, hardware flow control is already in the right bucket and would be left at the link's real value.

The masking belongs in the term layer, which understands termios: a restore variant that puts back everything except the speed and frame bits. The policy of using it belongs in gpsio, which owns the connection lifecycle: the physical settings are left in place only if a valid packet was received since they were last changed, since only then are they known to describe the link; otherwise close restores everything, as before.

This does change the tool from leaving the port at 9600 to leaving it at the receiver's speed and frame. A program that sets its own speed is unaffected; only one relying on inheriting the default would notice, which is fragile regardless. Note this in the change.

### Delay the first write until the divisor has switched

Have the term layer compute, when it changes the speed or frame, the monotonic time at which it is safe to write: the instant of the change plus a margin of a small number of character times at the old speed.

`term.Open` returns that time, and a speed change through `term.Change` exposes it the same way. gpsio hides the coupling: `SerialConn` sleeps until that time before issuing the first write, the same way it already hides the settle delay before a speed change. Callers (the daemon, gpscfg, gpscmd) get it for free.

Return the zero value of `time.Time` when no wait is needed, rather than the current time, so gpsio simply checks `IsZero` and skips the sleep. No wait is needed when the speed and frame did not change, and it should only be non-zero for a PL011, where the bug lives.

The fix is Linux-only. Identifying a PL011 needs a check narrower than `DevKind`, which maps both the 8250 (`ttyS*`, majors 4 and 5) and the PL011 (`ttyAMA`, majors 204 and 205) to `DevUART` and so cannot tell them apart. Gating on the ARM or ARM64 architecture was considered but is only a proxy; the character major is the direct signal. Add a small dedicated predicate in `term_linux.go`, alongside the existing `DevKind` fstat, that reports a PL011 from `unix.Major(s.Rdev)` being 204 or 205 with the minor in the `ttyAMA` range (64 and up). The safe-to-write margin is non-zero only when that predicate holds. A genuine 8250 or a USB-serial adapter returns the zero value and is never delayed, and the other term backends (macOS, BSD, Windows) return the zero value unconditionally, so the mitigation is confined to Linux and to the hardware where the bug was demonstrated.

The margin is two old-speed character times as an observed floor, padded a little rather than treated as a proven bound, since one 9600 case lost three characters. The switch window itself was inferred, not measured; a fine delay sweep at a low speed could replace the inference with a measured threshold before fixing the constant.

### Revert the interim gpscmd guard

An earlier uncommitted change to `internal/gpscmd/gpscmd.go` (`waitForQuietLine`, delaying the first message until the line is quiet) was a guess made before the mechanism was understood. It acts too late to prevent the corruption, which is set at open; it only helps gpscmd, not the daemon or gpscfg; and the two changes above do its job precisely. Revert it as part of this work.

## The stuck session is a kernel bug

The stuck-session case, where the whole session runs at the old divisor and does not recover, is a kernel driver defect and should be fixed there, not fully worked around in userspace. The driver commits a divisor and line-control change under a character in progress, which the PL011 documentation forbids; the proper fix is to not enable the receiver until the new line settings are in place, so no change is ever made under a character. That fix is tracked separately in the kernel investigation and would remove both failures at the source.

The two changes here do not claim to fix the stuck case. The restore policy makes it rare in practice, because the common back-to-back reopen no longer changes speed, so no divisor change occurs to get stuck. What remains is the cold case where the speed genuinely changes, such as the first open after boot, and that belongs to the kernel fix.

## Testing

The mechanism was characterised with a two-UART jumper reproducer that reopens one port mid-burst, changes its speed, writes a command, and checks the bytes at a fixed-speed peer; and confirmed against a real Allystar TAU1201, where a command written right after a mid-burst reopen failed to take effect while the reply looked healthy.

For the code changes: unit-test the term restore mask (only speed and frame preserved, everything else restored) and the safe-to-write time (zero margin when the speed is unchanged, the old-speed margin when it changes). The end-to-end behaviour is exercised by the existing systest message-file configuration on the Raspberry Pi that first showed the failure.
