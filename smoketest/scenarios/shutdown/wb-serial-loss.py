"""satpulsewb device loss: the workbench stays up and reports disconnected.

The inverse of the daemon's shutdown/serial-loss. When the serial input goes
away, satpulsewb must NOT exit -- it is a UI server, not a device-bound daemon,
so it emits the disconnected state and keeps serving, letting a browser see the
receiver drop and reconnect later. A pty backs the device so it can be
unplugged; DISCONNECTABLE selects the pty without the daemon's self-shutdown
exit check, and the runner stops satpulsewb the normal way (SIGINT) afterwards.

The session tears the connection down (verdictLost -> disconnected) rather than
reconnecting, because no reset is in flight -- reconnection is reset-scoped.
"""

import common

PROGRAM = "satpulsewb"
DISCONNECTABLE = True
PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon.jsonl"
FACTOR = 10

# Closing the pty master fails the in-flight read; the session reports it and
# ends the connection.
ALLOWED_ERRORS = ("read error while scanning",)


def run(ctx: common.SmokeContext) -> None:
    common.check_wb_state(ctx, "connected")
    ctx.wait_replay()    # let some data flow first, like a live receiver
    ctx.disconnect()     # unplug: close the pty master -> reads fail
    common.check_wb_state(ctx, "disconnected")  # server stays up, state flips
