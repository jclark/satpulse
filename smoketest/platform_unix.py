"""Unix platform functions for the smoke-test runner (see platform_api).

This module owns every Unix-only serial/process primitive the runner needs:
the FIFO and pty transports (fcntl, pty, select, tty), POSIX shutdown
signalling (SIGINT -> SIGQUIT -> SIGKILL, os.killpg for a sudo process
group), and privilege handling (os.geteuid, sudo). run.py imports it as
`plat` only on os.name == "posix", so keeping these imports here is what lets
run.py import cleanly on an OS without them.
"""

from __future__ import annotations

import errno
import fcntl
import os
import select
import signal
import subprocess
import threading
import time
import tty
from typing import IO, Callable, Sequence

from platform_api import Transport


class FifoTransport:
    """Serial input over a FIFO: a read-only replay sink that cannot disconnect.

    satpulsed opens the FIFO O_RDWR and holds its own write end, so an idle
    FIFO never reaches EOF -- it reads as a silent-but-connected receiver, and
    the daemon and replayer can start in any order. By the same token a FIFO
    can never look disconnected, so it cannot drive the input-loss path, and
    it is read-only at the gpsio layer, so it cannot carry the daemon's writes.
    """

    def __init__(self, fifo_path: str) -> None:
        self._path = fifo_path
        os.mkfifo(fifo_path)
        self._replay_fifo: IO[bytes] | None = None

    def path(self) -> str:
        return self._path

    def enable_write_capture(self, dest_path: str) -> None:
        raise RuntimeError("write capture requires a read-write transport, not a FIFO")

    def attach(self) -> None:
        pass

    def feed_replay(
        self,
        cmd: Sequence[str],
        errf: IO[bytes],
        open_timeout: float,
        daemon: subprocess.Popen[bytes] | None,
    ) -> subprocess.Popen[bytes]:
        f = self._open_fifo_write(open_timeout, daemon)
        self._replay_fifo = f
        return subprocess.Popen(list(cmd), stdout=f, stderr=errf)

    def _open_fifo_write(self, timeout: float, daemon: subprocess.Popen[bytes] | None) -> IO[bytes]:
        """Open the FIFO write end, waiting for the daemon to open the read end.

        A blocking open(O_WRONLY) would hang forever if the daemon crashes
        before opening the FIFO. Open non-blocking instead: O_WRONLY on a
        FIFO with no reader yet fails with ENXIO, so poll until the daemon
        opens it, the daemon dies, or the deadline expires. Clear O_NONBLOCK
        once open so the replay's writes block normally.
        """
        deadline = time.time() + timeout
        while True:
            try:
                fd = os.open(self._path, os.O_WRONLY | os.O_NONBLOCK)
                break
            except OSError as e:
                if e.errno != errno.ENXIO:
                    raise
                if daemon is not None and daemon.poll() is not None:
                    raise RuntimeError(
                        f"daemon exited before opening FIFO (code {daemon.returncode})"
                    )
                if time.time() >= deadline:
                    raise RuntimeError(f"daemon did not open the FIFO within {timeout}s")
                time.sleep(0.05)
        flags = fcntl.fcntl(fd, fcntl.F_GETFL)
        fcntl.fcntl(fd, fcntl.F_SETFL, flags & ~os.O_NONBLOCK)
        return os.fdopen(fd, "wb")

    def close_replay(self) -> None:
        if self._replay_fifo is not None:
            self._replay_fifo.close()
            self._replay_fifo = None

    def disconnect(self) -> None:
        raise RuntimeError("disconnect() requires a disconnectable transport, not a FIFO")

    def close(self) -> None:
        self.close_replay()


class PtyTransport:
    """Serial input over a pty: a real TTY, full-duplex and disconnectable.

    A pty slave is a real TTY, so it takes the term.Term path a USB serial
    receiver uses, not the FIFO replay path. Writing to the master feeds the
    daemon; closing every master fd makes the slave reads fail (EIO), which is
    how a pty -- unlike a FIFO -- can model the device being unplugged. It is
    also the only transport that can carry the daemon's writes, used by the
    stream/pull-* write-path scenarios (see enable_write_capture).
    """

    def __init__(self) -> None:
        # The master is the write side we feed and later close to disconnect;
        # the slave is held open so the master never sees a transient "no
        # slave" hangup before the daemon opens the device by name. The slave
        # is put in raw mode so binary GPS packets pass through untranslated
        # and the daemon's own writes are not echoed back to the master.
        master, slave = os.openpty()
        tty.setraw(slave)
        self._master = master
        self._slave = slave
        self._name = os.ttyname(slave)
        self._drain: threading.Thread | None = None
        self._drain_stop = threading.Event()
        # When capture is enabled the drain thread writes the daemon's upstream
        # serial output here instead of discarding it, so a write-path scenario
        # (stream.pull) can scan what the daemon sent back to the receiver.
        self._capture: IO[bytes] | None = None

    def path(self) -> str:
        return self._name

    def enable_write_capture(self, dest_path: str) -> None:
        """Record the daemon's serial writes to dest_path.

        Call before attach() so the drain thread captures from the first byte.
        A write-path scenario (stream.pull) scans the result for the RTCM
        corrections the daemon wrote back to the receiver; the non-RTCM probe
        bytes the daemon emits during detection are filtered out by tag.
        """
        self._capture = open(dest_path, "wb")

    def attach(self) -> None:
        """Start draining the daemon's writes.

        The drain thread reads whatever the daemon writes upstream -- probe
        output during detection now, stream.pull corrections later -- into the
        capture file when set, otherwise discarding it. Draining also keeps
        the daemon's writes from blocking on a full pty buffer.
        """
        self._drain_stop.clear()
        self._drain = threading.Thread(target=self._drain_pty, daemon=True)
        self._drain.start()

    def _drain_pty(self) -> None:
        # select() with a timeout lets the loop notice the stop flag, so the fd
        # is never read after close() is about to close it.
        while not self._drain_stop.is_set():
            try:
                r, _, _ = select.select([self._master], [], [], 0.1)
            except OSError:
                return
            if not r:
                continue
            try:
                data = os.read(self._master, 4096)
            except OSError:
                return
            if not data:
                return
            if self._capture is not None:
                self._capture.write(data)
                self._capture.flush()

    def feed_replay(
        self,
        cmd: Sequence[str],
        errf: IO[bytes],
        open_timeout: float,
        daemon: subprocess.Popen[bytes] | None,
    ) -> subprocess.Popen[bytes]:
        # pack writes into its own dup of the pty master; the runner keeps the
        # original master for draining and for disconnect(), so pack exiting
        # (closing its dup) is not on its own a disconnect.
        wfd = os.dup(self._master)
        try:
            return subprocess.Popen(list(cmd), stdout=wfd, stderr=errf)
        finally:
            os.close(wfd)

    def close_replay(self) -> None:
        pass

    def disconnect(self) -> None:
        """Disconnect the serial input, as if the device were unplugged.

        Closing every master fd makes the daemon's slave reads fail (EIO), so
        the scan worker exits -- the same trigger as a vanished USB serial
        device. After this the daemon should shut down on its own; assert that
        with wait_exit().
        """
        self.close()

    def close(self) -> None:
        """Stop the drain thread and close the pty fds. Idempotent."""
        if self._drain is not None:
            self._drain_stop.set()
            self._drain.join(timeout=2)
            self._drain = None
        if self._capture is not None:
            self._capture.close()
            self._capture = None
        for attr in ("_master", "_slave"):
            fd = getattr(self, attr)
            if fd >= 0:
                try:
                    os.close(fd)
                except OSError:
                    pass
                setattr(self, attr, -1)


# --- privilege --------------------------------------------------------------


def is_root() -> bool:
    """True if the process has the privilege root scenarios need."""
    return os.geteuid() == 0


def root_cmd(cmd: Sequence[str], use_sudo: bool) -> list[str]:
    """Wrap cmd so it runs with root privilege, or raise if it cannot."""
    if is_root():
        return list(cmd)
    if not use_sudo:
        raise RuntimeError("root command requested without --sudo")
    # sudo scrubs the environment, so forward GOARCH explicitly: root
    # helpers (ntpshm.py) need it to pick the cross-built daemon's layout.
    goarch = os.environ.get("GOARCH")
    env = ["env", f"GOARCH={goarch}"] if goarch else []
    return ["sudo", "-n", *env, *cmd]


# --- transport --------------------------------------------------------------


def make_transport(path: str, readwrite: bool, disconnectable: bool) -> Transport:
    """Create the serial-input transport with the requested capabilities.

    On Unix the pty provides both read-write and disconnect while the FIFO
    provides neither, so a request for either capability yields a pty and a
    request for neither yields a FIFO backed by path. A platform that cannot
    honour a request raises platform_api.TransportUnsupported; Unix always can.
    """
    if readwrite or disconnectable:
        return PtyTransport()
    return FifoTransport(path)


# --- shutdown ---------------------------------------------------------------


def stop_daemon(
    daemon: subprocess.Popen[bytes],
    grace: float = 5.0,
    process_group: bool = False,
    signaler: Callable[[signal.Signals], None] | None = None,
    final_grace: float | None = None,
) -> str | None:
    """Stop the daemon, escalating the way the systemd unit does.

    satpulsed catches both SIGINT and SIGTERM and treats them as the same
    graceful-shutdown request, so SIGTERM cannot rescue a hung shutdown.
    The packaged unit uses TimeoutStopSec=5 with FinalKillSignal=SIGQUIT,
    so on a hang we escalate to SIGQUIT and normally stop there: the daemon
    does not catch it, and Go's default handler dumps all goroutine stacks
    (to the daemon log) and aborts the process -- both the diagnostic we want
    for a shutdown hang and a reliable terminator. SIGQUIT is the final
    backstop unless the caller sets final_grace for a platform wrapper that
    needs its own bounded wait.

    Returns None on a clean SIGINT exit, otherwise an error string
    describing the escalation that was needed.
    """
    _send_daemon_signal(daemon, signal.SIGINT, process_group, signaler)
    try:
        daemon.wait(timeout=grace)
        return None
    except subprocess.TimeoutExpired:
        pass
    _send_daemon_signal(daemon, signal.SIGQUIT, process_group, signaler)
    if final_grace is None:
        daemon.wait()
    else:
        try:
            daemon.wait(timeout=final_grace)
        except subprocess.TimeoutExpired:
            _send_daemon_signal(daemon, signal.SIGKILL, process_group, signaler)
            try:
                daemon.wait(timeout=final_grace)
            except subprocess.TimeoutExpired:
                return (
                    f"daemon did not exit within {grace:g}s of SIGINT, "
                    f"{final_grace:g}s of SIGQUIT, or {final_grace:g}s of SIGKILL"
                )
            return (
                f"daemon did not exit within {grace:g}s of SIGINT or "
                f"{final_grace:g}s of SIGQUIT; SIGKILL stopped it"
            )
    return (
        f"daemon did not exit within {grace:g}s of SIGINT; "
        "SIGQUIT dumped goroutines (see satpulsed.log) and aborted it"
    )


def _send_daemon_signal(
    daemon: subprocess.Popen[bytes],
    sig: signal.Signals,
    process_group: bool,
    signaler: Callable[[signal.Signals], None] | None = None,
) -> None:
    """Send sig to the daemon, or to its process group when launched through sudo."""
    if signaler is not None:
        signaler(sig)
        return
    if process_group:
        os.killpg(daemon.pid, sig)
        return
    daemon.send_signal(sig)


def wait_exit(daemon: subprocess.Popen[bytes], timeout: float = 10) -> int:
    """Wait for the daemon to exit on its own, sending it no signal.

    For SELF_SHUTDOWN scenarios after disconnect(): the daemon must shut
    down because its serial input went away. A hang is exactly the bug we
    guard against, so on timeout escalate to SIGQUIT -- Go dumps every
    goroutine stack to the daemon log -- and fail, the same diagnostic
    stop_daemon gives for a stuck shutdown.
    """
    try:
        return daemon.wait(timeout=timeout)
    except subprocess.TimeoutExpired:
        daemon.send_signal(signal.SIGQUIT)
        daemon.wait()
        raise RuntimeError(
            f"daemon did not exit on its own within {timeout:g}s after the "
            "serial input disconnected; SIGQUIT dumped goroutines to "
            "satpulsed.log"
        )
