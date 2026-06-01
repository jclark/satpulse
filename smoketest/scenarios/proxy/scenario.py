"""Read-only serial proxies (TCP and Unix socket), filtered to UBX.

The Unix-socket proxy is read with satpulsetool gps --socket --capture
--packet-log, which also exercises that tool. Both checks confirm the
forwarded packets exist in the source packet log.
"""

import checks

PACKET_LOG = "gps/testdata/packets/u-blox/ZED-F9P/daemon.jsonl"
FACTOR = 3


def run(ctx):
    checks.check_proxy_tcp(ctx, protocol="UBX")
    checks.check_proxy_socket_capture(ctx, protocol="UBX")
    ctx.wait_replay()
