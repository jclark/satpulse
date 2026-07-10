"""Program seam for the smoke-test runner.

run.py drives one program under test whose identity is a scenario dimension:
`satpulsed` by default, `satpulsewb` when a scenario sets PROGRAM = "satpulsewb".
The two programs differ in a handful of places, and the Program interface owns
exactly those:

  - input preparation: satpulsed renders a `.toml.in` config and derives its
    helper peers from the rendered config; satpulsewb renders a `.args.in` flag
    list and declares its peers explicitly (both with ${SATPULSE_TEST_*}
    substitution);
  - the start command;
  - readiness: config-derived listeners vs the one HTTP port plus parsing the
    printed URL and token;
  - the base set of daemon-log lines the error scan tolerates;
  - which listen ports must be free once the program has stopped.

Everything else -- port-block allocation, run dirs, the serial transport
(FIFO/pty, write capture, disconnect), the replay lifecycle, the fake peers,
and the parallel executor -- stays shared in run.py.

This is the program counterpart to platform_api. Unlike the platform module
(one per process, chosen by os.name), a Program is chosen per scenario, so the
programs coexist in one process and share this Protocol -- the same way the
serial Transport does. run.py holds one Program per scenario and calls it
polymorphically; the concrete programs live in program_satpulsed.py and
program_satpulsewb.py.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Protocol

if TYPE_CHECKING:
    from run import Context, ScenarioModule


class Program(Protocol):
    """The program under test and the per-program steps around running it."""

    # Binary name under out/<arch>/ and the label used in messages.
    name: str

    def prepare(self, ctx: Context, scen: ScenarioModule) -> None:
        """Render the program's input into the run dir and record ctx state.

        satpulsed renders name.toml.in and sets ctx.has_http/ntrip/... from the
        rendered config so the runner starts the matching peers and waits on the
        matching listeners; satpulsewb renders name.args.in into ctx.wb_args and
        records the scenario's declared peers. Runs after the transport is
        attached and before the program starts.
        """
        ...

    def command(self, ctx: Context) -> list[str]:
        """The program's argv, before any root wrapping."""
        ...

    def start_peers(self, ctx: Context, scen: ScenarioModule) -> None:
        """Start the fake peers the program needs, before the program starts."""
        ...

    def wait_ready(self, ctx: Context) -> None:
        """Block until the program is observable by the scenario's checks."""
        ...

    def allowed_errors(self) -> tuple[str, ...]:
        """Base error/warn substrings the daemon-log scan tolerates for this program."""
        ...

    def ports_to_free(self, ctx: Context) -> list[int]:
        """Listen ports that must be released once the program has stopped."""
        ...


def select(name: str) -> Program:
    """Return the Program for a scenario's PROGRAM name (default satpulsed)."""
    if name == "satpulsed":
        import program_satpulsed

        return program_satpulsed.Satpulsed()
    if name == "satpulsewb":
        import program_satpulsewb

        return program_satpulsewb.Satpulsewb()
    raise RuntimeError(f"unknown PROGRAM {name!r}")


def substitute(text: str, env: dict[str, str]) -> str:
    """Expand ${SATPULSE_TEST_*} references in a template against env."""
    for key, val in env.items():
        text = text.replace("${" + key + "}", val)
    return text
