"""Transport interface for the smoke-test runner.

run.py is the OS-independent engine. The platform-specific primitives live in
a per-OS module of functions (platform_unix.py today; a platform_windows.py
drops in later), which run.py selects by os.name and calls as `plat.<fn>()`.
Because only one such module is ever live in a process, it needs no shared
type -- the module *is* the interface.

The serial-input transport is the exception: FIFO and pty transports coexist
in one process, chosen per scenario, and the runner holds one and dispatches
to it polymorphically. So they share this interface, which lives here (not in
a per-OS module) so run.py can name it without importing fcntl/pty. A Win32
named-pipe transport implements the same interface later.
"""

from __future__ import annotations

import subprocess
from typing import IO, Protocol, Sequence


class TransportUnsupported(Exception):
    """A platform cannot supply a transport with the requested capabilities.

    Raised by a platform module's make_transport when the scenario asks for a
    read-write or disconnectable transport the platform does not yet provide
    (e.g. the Windows read-write pipe before it is written). The runner reports
    it as SKIP, the same as a scenario missing an external dependency.
    """


class Transport(Protocol):
    """The serial-input sink the daemon reads.

    Created before the daemon starts (make_transport) with two orthogonal,
    scenario-requested capabilities: read-write, so it can capture the daemon's
    upstream writes (enable_write_capture), and disconnectable, so it can model
    the device vanishing (disconnect). A plain replay sink has neither. path()
    is what goes in SATPULSE_TEST_SERIAL. The runner feeds exactly one replay
    (feed_replay) per daemon lifetime and closes the transport at the end.
    """

    def path(self) -> str:
        """The device path the daemon opens (also SATPULSE_TEST_SERIAL)."""
        ...

    def enable_write_capture(self, dest_path: str) -> None:
        """Record the daemon's upstream writes to dest_path (duplex only).

        Call before attach() so no early write is missed.
        """
        ...

    def attach(self) -> None:
        """Take ownership before the daemon starts (e.g. start a drain thread)."""
        ...

    def feed_replay(
        self,
        cmd: Sequence[str],
        errf: IO[bytes],
        open_timeout: float,
        daemon: subprocess.Popen[bytes] | None,
    ) -> subprocess.Popen[bytes]:
        """Launch the single replay (pack) writing into the sink.

        pack writes to a stdout the transport owns rather than opening the
        device path itself, so a sink that cannot be opened twice (a Win32
        named pipe) can relay pack's output instead. errf receives pack's
        stderr; daemon, when set, is polled so a daemon that dies before the
        sink is ready fails fast rather than blocking.
        """
        ...

    def close_replay(self) -> None:
        """Close the replay sink write-end. Idempotent."""
        ...

    def disconnect(self) -> None:
        """Model the device being unplugged (disconnectable transports only).

        The platform only hands the runner a transport with this capability
        when the scenario asked for it, so a scenario reaching disconnect()
        always has a transport that supports it; a read-only sink raises.
        """
        ...

    def close(self) -> None:
        """Release all transport resources. Idempotent."""
        ...
