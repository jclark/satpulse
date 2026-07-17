"""Serial input disappears: the daemon must shut down on its own.

Exercises the scan-worker-exit -> daemon-shutdown path. When the serial input
goes away the scan worker exits; the daemon then has no GPS data and must exit
so systemd can restart it (issue #172). The risk is that nothing links the scan
worker exiting to cancelling the daemon's context, so the HTTP server (and the
other context-bound goroutines) keep the process alive and shutdown hangs.

This needs a transport that can actually disconnect, which is a pty: a FIFO
cannot, because satpulsed opens it O_RDWR and holds its own write end, so an
idle FIFO stays "connected" by design. After a short replay (the receiver was
streaming), closing the pty master makes the slave reads fail and the scan
worker exit. The runner then asserts the daemon exits by itself, with no signal,
and with a restartable failure code.
"""

import common

SELF_SHUTDOWN = True
PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon.jsonl"
FACTOR = 10

# Disconnecting the serial input legitimately produces these: the in-flight
# read fails, and termios cannot be restored on a device that is gone.
ALLOWED_ERRORS = (
    "read error while scanning",
    "error closing the serial port",
)


def run(ctx: common.SmokeContext) -> None:
    ctx.wait_replay()    # let some data flow first, like a live receiver
    ctx.disconnect()     # unplug: close the pty master -> slave reads fail
    ctx.wait_exit()      # the daemon must exit with no signal from us
