# Playwright browser tests for the webui frontends (#376)

A `@playwright/test` suite covering the two embedded frontends --
the dashboard served by satpulsed and the workbench served by
satpulsewb -- with tests that run in a real browser and check what
the user sees, driven by the same hardware-free packet sources the
smoketest suite uses: a FIFO packet-log replay and the u-blox
receiver simulator (gps/app/ubxsim). This plan supersedes phase 8
of satpulseweb.md (#357) and the Playwright half of its phase 9,
both removed from that plan; the smoketest halves have landed.

The plan is organised by UI surface, not by delivery order; the
tests for different surfaces are largely independent, and a short
Sequencing section at the end covers the PR order and the few real
dependencies.

## Why Playwright, and the division of labour

The frontends have no test coverage today beyond a few vitest unit
tests: the smoketest suite asserts everything at the wire level
(HTTP, SSE, serial writes), but nothing checks that the token is
consumed and stripped from the URL bar, that SSE events actually
render on the page, that the config panel's validation and dirty
tracking work, or that an apply round-trips as the user sees it.
Frontend logic (form validation, populate-from-readback, pending
tracking) is unreachable from the wire; a browser test is its only
possible home.

Playwright drives real browsers (Chromium, Firefox, WebKit)
headless; `@playwright/test` is a Node test runner with parallel
workers and a fixture system for launching server processes. Its
assertions wait and retry until a timeout, which is the main
defence against flaky tests; one test can open multiple pages and
browser contexts -- real multi-tab, which the workbench seat model
needs -- and failures record traces inspectable in a viewer without
reproducing locally. Chromium only: no per-PR cross-browser runs,
and nothing cross-browser is planned.

The division of labour with smoketest: anything assertable at the
wire level stays in `smoketest/`, which needs no npm and no
browser; where a browser test overlaps a smoketest scenario (the
config apply tests overlap `config/wb-apply`), the smoketest
scenario keeps the exhaustive wire assertions and the browser test
checks what the user sees.

Depth varies by surface, guided by the web redesign (#284): every
component that plan shares or rewrites is a monitoring widget (the
status bar, map, clock, sky view, signal graph), so the monitoring
surfaces get basic does-it-render checks until the redesign lands,
while the configuration tab -- the product priority, untouched by
#284, and where the untested frontend logic lives -- is tested in
depth.

The tests run against the real binaries with their embedded assets
-- what ships -- not against a vite dev server. A side effect is
that a stale checked-in `dist/` fails any test whose behaviour the
un-regenerated source change affects.

## The e2e package

`webui/packages/e2e`, npm name `@satpulse/e2e`, registered in the
root `workspaces` array. Flat layout:

```
webui/packages/e2e/
  package.json
  playwright.config.ts
  harness.ts
  dashboard/    *.spec.ts for the dashboard project
  workbench/    *.spec.ts for the workbench project
```

- The package has no `test` script: the root `npm test` runs
  `--workspaces --if-present`, and the browser suite must not run
  under the vitest step of the CI workflow. Its runner script is
  named `e2e` and invoked explicitly.
- `playwright.config.ts` declares two projects, `dashboard`
  (`testDir: './dashboard'`) and `workbench`
  (`testDir: './workbench'`), runnable independently with
  `--project`. Chromium only, `workers: 1` (one launched daemon at
  a time, no port juggling), `trace: 'retain-on-failure'`.
- `harness.ts` exports `test`/`expect` extended with worker-scoped
  fixtures (below) that launch the binaries under test and hand
  each test a base URL. Binaries come from `out/<arch>` (the
  smoketest's contract: `make` has run first); the harness resolves
  the repo root and arch itself.
- Selector policy: ARIA roles and visible text, `data-testid` where
  neither is stable. No screenshot or visual-regression testing:
  #284 will rework markup and styling, and role/text selectors are
  what survive that.

## Fixtures

The harness provides these launch fixtures; each section below
names the one it uses.

- Dashboard replay: create a temp run dir and a FIFO, pick a free
  port, render a minimal satpulsed TOML (`serial.device` pointing
  at the FIFO, one `[[http]]` endpoint, nothing else -- satpulsed
  runs unprivileged), spawn satpulsed, poll the HTTP endpoint for
  readiness, then start `satpulsetool pack --realtime FACTOR`
  writing
  `gps/testdata/packets/u-blox/ZED-F9P/daemon-sats-pos-38400.jsonl`
  into the FIFO -- writer after reader, the open ordering the
  smoketest transport already uses. Teardown SIGTERMs both and
  keeps the run dir on failure.
- Workbench replay: the same FIFO replay behind `satpulsewb -d
  <fifo>` with no `-L`; parse the printed URL and generated token
  from stdout, as the smoketest's `program_satpulsewb.py` does --
  printing the URL is readiness -- rewriting the host part to
  127.0.0.1 (the printed URLs carry LAN interface addresses). The
  read-only FIFO means connect lands in passive detection.
- Workbench ubxsim: spawn `satpulsetool ubxsim --link
  <dir>/gps.pty -r gps/testdata/config/u-blox/ZED-F9P/sim.jsonl
  gps/app/ubxsim/testdata/f9p/f9p-personality.ubx`; the symlink
  appearing is readiness. Ordering as in the smoketest's ubxsim
  provider: simulator up before satpulsewb opens the device,
  SIGTERM only after satpulsewb has exited. Launch `satpulsewb -d
  <link> -s 38400` (the personality's default baud, keeping the
  config phase out of the baud-change path); for the connection
  panel tests, launch without `-d` instead. The probe answers, so
  detection is active and the config path works. The simulator's
  nav engine runs in real time (one epoch per second, no speed
  factor), so these are the slowest tests; the 300-epoch bank
  bounds them comfortably.
- Fixed-port variant: `satpulsewb -L 127.0.0.1:<port>` with no
  token, for the restart test (a restarted satpulsewb generates a
  fresh token, so a token-authenticated page cannot survive a
  restart by design).
- Fake caster: the smoketest's `fakesource.py` NTRIP caster,
  spawned by the harness (CI runners have python3), for the
  corrections tests.

## CI

Extends the node-tests workflow (see Prerequisites); no second
workflow, because both stages share `npm ci` and the node setup.
Appended steps: `actions/setup-go`; `make` (the suite needs real
satpulsed, satpulsewb and satpulsetool binaries as test subjects);
`npx playwright install --with-deps chromium` behind an
`actions/cache` on `~/.cache/ms-playwright` keyed on the Playwright
version; the e2e run; upload of traces and report as an artifact on
failure.

The workflow stays always-on (push and pull_request to master, like
the other workflows), not path-filtered: the tests exercise
cmd/satpulsewb and gps/app/session as much as the frontend, so
filtering on `webui/**` would skip them exactly when the server
side changes. Estimated cost: roughly 4 minutes cold-cache, 3-4
warm, comparable to the slowest existing workflow; if the suite
outgrows that, the levers are building only the needed binaries
instead of the full `make`, and raising `workers` (each worker gets
its own launched server, so tests parallelise across fixtures).

## Prerequisites

- The node-tests workflow: a standalone PR, outside this plan,
  justified on its own -- no CI workflow runs npm today, so the
  existing vitest suites and typecheck never run in CI.
  `.github/workflows/node-tests.yml` ("Node tests"): checkout,
  `actions/setup-node` with the npm cache keyed on
  `webui/package-lock.json`, `npm ci --prefix webui`, `npm run
  typecheck --prefix webui`, `npm test --prefix webui`. Runs 1-2
  minutes, in parallel with the existing workflows. Future
  packages join automatically via `--workspaces --if-present`.
- ubxsim reset handling: a prerequisite of Testing the
  Configuration tab, on its own branch as its own PR. The
  simulator models the layered config database (Default, RAM, BBR,
  Flash) and CFG-CFG clear/save/load fully, but ignores CFG-RST --
  a real receiver never ACKs it, so the sim silently drops it.
  Cold start and factory reset send CFG-RST (a hardware reset), so
  after a simulated factory reset the RAM config wrongly keeps its
  current values instead of being rebuilt from the cleared layers.
  The enhancement: treat a CFG-RST hardware reset as a reboot --
  rebuild RAM from Default, Flash, BBR in layer priority order and
  restart the nav engine -- with the simulator process and its pty
  staying alive so the program's connection survives. Validate the
  behaviour against the real F9P (what a cold start does to
  unsaved config, and what the workbench sees during the reset).
  Anything else the configuration tests turn out to need from the
  simulator rides the same PR.

## Testing the dashboard

Workbench-independent; dashboard replay fixture. Basic render
checks only -- this is the surface #284 reworks first.

- Startup: the page renders; the satellite display goes from empty
  to populated; the clock advances (the time text changes between
  two reads); a position appears. Assertions retry with timeouts
  sized to the replay factor.
- The remaining cards each render their data from the replay.
  Deeper assertions wait for the redesign.

## Testing workbench startup and access control

Workbench replay fixture, except the restart test (fixed-port
variant).

- Startup: navigating to the URL with `?t=` renders the app and
  the URL bar no longer contains the token -- the one behaviour
  only a real browser can verify. The connection state reaches
  connected (passive detection over the read-only FIFO, as in the
  `http/wb-default` scenario). Satellites and position render and
  advance.
- The bare origin without a token gets the rejection, not the app.
- Stale-token notice, both paths to it: at page load, a wrong
  token value fails the pre-mount validation and shows the notice
  instead of the app; and after a satpulsewb restart invalidates
  the page's token, the transport shows the same notice via
  wb:authlost (main.tsx documents this as deliberate).
- Seat takeover: two pages in one browser context; the second
  opens the bare origin and inherits the token from the shared
  localStorage (checking the token store on the way), claims the
  write seat, and shows a consistent state immediately, filled
  from the server's cache of the latest event per name rather than
  by waiting for fresh events; the first tab drops to read-only
  viewer, checked by its config controls becoming disabled (the
  panel folds the viewer role into its disabled state).
- Reconnect after a server restart (fixed-port variant, so there
  is no token to go stale): the fixture restarts satpulsewb on the
  same port; the page reconnects its SSE and the display
  repopulates from the new server's event cache.

## Testing the Monitor tab

Workbench replay fixture. Basic render checks only -- these are
the widgets #284 rewrites (sky view, signal graph, clock) or
shares. Each panel populates from the replay: satellites, signal
graph, position, time display, scatter. The map is excluded: it
fetches OpenStreetMap tiles over the network, which CI must not
depend on.

## Testing the Connection panel

Workbench ubxsim fixture, launched without `-d`. The panel has a
free-text device input beside the enumerated port dropdown, which
is how the test reaches the simulator: the pty symlink is not a
real /dev node, so enumeration will not list it.

- Type the symlink path, select 38400, Connect: the state walks to
  connected and the receiver identity shows the personality's F9P
  model and firmware (active detection -- the probe answered).
- Disconnect returns to disconnected and the dependent controls
  disable.
- Connecting to a nonexistent path surfaces the failure to the
  user.
- The port dropdown renders (an empty "No ports found" list is
  acceptable on a CI runner).

## Testing the Configuration tab

Prerequisite: the ubxsim reset PR. All tests on the workbench
ubxsim fixture. Two facts shape them (config-panel.tsx): readback
is automatic on first opening the Config tab while connected --
there is no read button, and re-reading within a connection means
reloading the page, which re-checks the stored token on the way --
and the bottom action bar's status label walks through "Changes
pending to ...", "Applying configuration..." and "Configuration
applied", which is what the tests assert against. The simulator's
config database returns VALSET values verbatim, so properties
round-trip.

Panel-wide behaviour:

- The automatic readback populates the panel on first opening the
  tab; before it, and whenever disconnected, every control is
  disabled.
- The pending label names exactly the sections touched ("Changes
  pending to time pulse, messages"); Apply is disabled with
  nothing pending.
- Discard restores the readback values and clears the pending
  state.
- Editing the cable delay marks the whole time-pulse section
  touched, so an apply re-sends the time-pulse values as read back
  -- harmless against the simulator, whose own VALGET supplied
  them. (Recorded here so the tests do not mistake it for a bug.)

Time pulse:

- Readback populates the group: period, pulse width, time GNSS,
  cable delay, and the three checkboxes (align to GNSS, only when
  locked, rising edge) hold the personality's values.
- Validation (client-side logic in `validateFields`, untestable
  anywhere but a browser): a non-numeric cable delay marks the
  input invalid and disables Apply; a negative period likewise; a
  pulse width outside 0..1, or at least as long as the period,
  likewise; correcting the field re-enables Apply.
- Dirty tracking: editing any field adds "time pulse" to the
  pending label; Discard restores the readback values and clears
  it.
- Apply round-trip: set a new period, width, and cable delay;
  Apply reaches "Configuration applied"; after a page reload the
  fresh readback shows the new values.

Time mode:

- Readback selects the mode radio matching the receiver (mobile /
  survey-in / fixed position) and fills the fixed-position fields
  when set.
- Selecting survey-in enables the survey subgroup and leaves the
  fixed-position subgroup disabled, and vice versa.
- Validation: latitude outside -90..90, longitude outside
  -180..180, empty or non-numeric ECEF coordinates, a survey time
  of zero or less, and a survey accuracy below 1 mm each mark the
  field invalid and disable Apply.
- The ECEF on-Earth indicator: coordinates far from the Earth's
  surface show the not-on-Earth marker (this exercises the
  server's check-on-earth endpoint from the browser).
- Apply round-trip: a fixed ECEF position; the reload-readback
  shows fixed mode with the position. A second apply back to
  mobile restores the fixture state for later tests in the file.

Satellites and signals:

- Readback checks the constellation boxes per signalsEnabled.
- The signal picker dialog: Edit signals... opens it, toggling a
  signal and confirming updates the selection and marks the
  section pending; Cancel does not.
- Validation: minimum elevation outside 0..90 marks the field
  invalid.
- Apply round-trip: change the enabled signals and the minimum
  elevation; the reload-readback shows both. The displayed
  satellites are not asserted to change: the simulator replays its
  recorded satellites regardless of the signal config.

Messages:

- The Minimum and Daemon preset buttons set the group controls
  (pure frontend logic; no apply needed to check them).
- Apply round-trip with a live effect: enabling the satellites
  messages makes satellite data appear (NAV-SAT is off in the
  personality defaults and present in the bank, so only the apply
  can make it flow).
- The per-group wire encodings (NMEA both directions, RTCM, PVT,
  Raw) stay in `config/wb-apply`, which cycles all of them.

Serial speed:

- The dropdown shows the readback baud rate (38400).
- No apply: speed changes are hardware-test territory (a pty has
  no real line speed), and the non-UART "not applicable" state is
  untestable while the personality models a UART port.

Persistent operations:

Save and Reset have no readback to populate from; testing them
means testing what survives. Reset=Reload (a CFG-CFG load -- the
receiver rebuilds its RAM config from the saved layers, without a
hardware reset) is the observation tool:

- Unsaved changes are volatile: apply a property change with
  Save=Nothing, then apply Reset=Reload alone; the reload-readback
  shows the old value.
- Save=Changes persists: apply the same change with Save=Changes
  (the VALSET is written to the Flash, BBR and RAM layers), then
  Reset=Reload; the readback still shows the new value, and still
  does after another page reload.
- Save=All: apply a change with Save=Nothing, then Save=All alone
  (CFG-CFG save of the whole current configuration), then
  Reset=Reload; the change survives.
- Cold start, once the ubxsim reset PR has landed: an unsaved
  change plus Reset=Cold start; the connection survives (the
  simulator keeps its pty open across the simulated reboot) and a
  fresh readback shows the change gone. Factory reset is checked
  the same way if the workbench flow allows it cleanly; otherwise
  it stays untested here.

## Testing the Packets tab

Workbench replay fixture.

- Opening the Packets tab makes packets appear on the page, with
  message names from the replay log. Whether the server actually
  stops streaming when the tab closes is a wire question and stays
  with `check_wb_packet_gating`.

## Testing the Messages tab

- Catalog (workbench replay fixture): with `SATPULSE_GPSMSG_PATH`
  pointed at the repo's `configs/gpsmsg`, the vendor and file
  dropdowns populate, and loading a file shows its tag table.
- Send (workbench ubxsim fixture): load a u-blox library file,
  send a query tag, and the response table shows the reply status
  -- the simulator answers, so the send path (sendWorker,
  correlator, response events) gets its first browser-level check.

## Testing the Corrections tab

Workbench replay fixture plus the fake caster.

- Fill in the source form (host, port, mountpoint), start: the
  corrections state reaches connected and the correction messages
  panel populates with the caster's RTCM.
- Stop returns to idle.
- The deeper wire checks (RTCM written to the port, VRS refusal
  without a fix, GGA upload) stay in `stream/wb-corrections`.

## Relationship to web-redesign (#284)

The redesign reworks the monitoring widgets this suite drives with
its basic render checks; those checks are written to survive
restyling (role and text selectors, no visual assertions) and are
deliberately not deepened until the redesign lands. The
configuration tab tests are unaffected: #284 does not touch that
panel. When the shared components land, the same render checks
cover both frontends' use of them; visual or more detailed
monitoring assertions are a decision for after the redesign.

## Sequencing

The sections above are not phases; most are independent. PR order:

1. The node-tests workflow (standalone, first).
2. The e2e package, the CI extension, and the dashboard tests --
   the smallest PR that proves the infrastructure end to end. The
   suite must be green in CI from its first commit.
3. Workbench startup and access control.
4. The ubxsim reset PR, on its own branch, any time after (1) --
   it gates only the Configuration tab section.
5. The Configuration tab (after 4), then Connection panel, Monitor,
   Packets, Messages, Corrections in any order, sized into PRs as
   convenient.

Verification for every PR: the suite run repeatedly locally (~10x)
to check for flaky tests, plus green CI. Known sources of
flakiness, each with a known mitigation: replay pacing vs assertion
timeouts, FIFO open ordering, and the simulator's real-time pacing.
