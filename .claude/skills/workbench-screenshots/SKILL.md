---
name: workbench-screenshots
description: Take screenshots of satpulsewb tabs for the website's workbench pages, with Playwright against live receivers. Partly manual - the user chooses which receivers go on which pages.
allowed-tools: Read, Bash, Glob, Grep, Write, Edit
---

# Workbench screenshots

Each page under `docs/workbench/` shows screenshots of one satpulsewb
tab, taken with headless Chromium against a satpulsewb instance
connected to a real receiver. `docs/workbench/CLAUDE.md` describes the
gallery front matter and the image naming; this skill is the procedure
for making the images. The script `shoot.mjs` in this directory drives
the browser.

The process is partly manual. Which receivers are connected and which
receiver a page should show are the user's decisions, made at step 3.
Do not run the shooter before the user has answered.

## Fixed decisions

- Viewport 1024x820, deviceScaleFactor 1, headless Chromium. No
  border, no window chrome, no post-processing.
- Images go in `docs/assets/images/` as `wb-<run>-<shot>.png`. `<run>`
  names the receiver (f9p, g5, allystar, casic, ...) and picks the run
  branch in the script.
- Do NOT set `SATPULSE_GPSMSG_PATH` for any satpulsewb instance. The
  Message file tab displays the resolved path, and a reader should see
  `built-in:...`, not a worktree path. Which satpulsewb build to shoot
  against (installed or from the tree) is a question for the user each
  round.
- Any write to a receiver's configuration (Apply in Configuration,
  Send in Message file, `satpulsetool gps` with config options) needs
  the user's confirmation first. Reading, connecting to a caster, and
  taking screenshots do not.
- Do not touch a receiver that satpulsed normally owns beyond what the
  user has agreed to; in particular a receiver in fixed-position time
  mode stays that way, and RTK shots use a different receiver.

## Procedure

### 1. Read what the pages currently show

For each `docs/workbench/*.md`, read the `<page>_gallery` list in the
front matter and list the images with their `<run>` and `<shot>`. This
is the starting map of page to receiver and the set of shots to
retake. It is the only list of shots there is: the pages are the
source, not this skill.

### 2. Discover what is connected

Check no satpulsed unit holds a device: `ps ax | grep "[s]atpulsed"`.
If one does, tell the user which; stopping units is theirs to do (sudo),
not yours.

Then, with the `satpulsetool` skill's conventions:

    satpulsetool serial                       # list ports, vid/pid, serial numbers
    satpulsetool serial -d DEV                # detect the speed
    satpulsetool gps -d DEV -s SPEED --show-receiver [--vendor NAME]

`--show-receiver` identifies the model. Record device, speed, vendor
and model in `CLAUDE.local.md` as the satpulsetool skill asks.

### 3. Present the plan and wait

Show the user two lists: what each page currently shows, and what is
on the bench, flagging any page whose receiver is not connected. Ask in
prose which receiver goes on which page and which shots to retake. The
constraints under "Fixed decisions" and the notes under "Findings"
belong in that message where they bear on the choice, so the user can
override them knowingly.

### 4. Start satpulsewb instances

One instance per receiver, loopback, token disabled, without
`SATPULSE_GPSMSG_PATH`:

    setsid nohup satpulsewb -L 127.0.0.1:PORT -n [--vendor NAME] -d DEV -s SPEED \
      > <scratch>/wb-<run>.log 2>&1 < /dev/null & disown

Check with `ps ax | grep "[s]atpulsewb -L"`. To stop one, use
`pgrep -f "satpulsewb -L 127.0.0.1:PORT" | xargs -r kill`; never
`pkill -f satpulsewb` from the tool shell, the pattern matches the
shell itself.

Then rewrite the run branches at the end of `shoot.mjs` for the agreed
plan. The helpers above the branches are the stable part; leave them
unless a selector has broken.

### 5. Take one shot, check it, then run the legs

Playwright is hoisted to `webui/node_modules`; the script resolves it
from there and runs from anywhere. Needs `npm --prefix webui ci` and
`npx playwright install chromium` to have been run once.

    LD_LIBRARY_PATH=~/.cache/playwright-sys-libs node .claude/skills/workbench-screenshots/shoot.mjs \
      http://127.0.0.1:PORT/ docs/assets/images <run> [step]

Run the cheapest leg first and look at the image before running the
rest: a broken selector shows up there, and the fix goes into
`shoot.mjs` in place. Then run each leg, look at every image, and
update the gallery front matter so every image on a page was just
taken and no `wb-*.png` is left unreferenced.

To check what a message set will produce without a browser, capture
and replay:

    satpulsetool gps -d DEV -s SPEED --vendor NAME -m MSGFILE -t TAGS --capture 15 --packet-log LOG
    satpulsetool replay --vendor NAME LOG

The replay prints the pipeline's events; the `posGeo` entries and
their `nativeMsgID` show which message supplies position.

### 6. Leave the receivers as found

Say at the end what was changed on each receiver and whether it was
saved to non-volatile memory (it should not have been).

## Script notes

Things that cost time to find out:

- satpulsewb holds the Ntrip connection, not the browser. A re-run
  finds the Host field disabled and has to disconnect first. The
  caster's Disconnect is the second visible one, because the header's
  Disconnect for the serial device comes first in the DOM.
- The scatter panel's Clear is the first visible Clear on the Monitor
  tab; the log panel's is the second.
- The RTK solution wanders for some minutes after it first fixes, so
  the scatter run settles, then clears the scatter, then collects
  points.
- Killing satpulsewb drops the Ntrip connection, so every receiver
  reconfiguration costs the fix and it has to re-acquire. A `step`
  argument that retakes only one shot (the `scatter` example) avoids
  redoing the others.
- Configuration section shots scroll so the section heading is at the
  top of the panel. Packets shots click Clear, wait 5s, click the named
  row, and scroll the table back to top.

## Findings

### The Configuration tab greys out controls, not sections

Sections a receiver cannot do are not greyed out as wholes: Time pulse
and Time mode stay editable on a receiver without them. What greys is
finer: in Satellites and signals, constellations the receiver does not
report are disabled; in Messages, the RTCM subsection is disabled when
the receiver has none. Pick section shots that show this.

### Position source priority

The Position Scatter panel plots the epoch's PosECEF, derived from the
merged PosGeo when no message supplies an ECEF directly, and
`PosGeoMsg.Merge` lets the higher `MsgPriority` win. NMEA GGA is
`PriGenericHigh` and vendor binary NAV messages are `PriVendorLow`, so
any binary position message beats GGA. Where the binary messages carry
position in 1e-7 degrees or whole centimetres (Allystar NAV-AUTO,
NAV-POSLLH, NAV-POSECEF), the scatter collapses onto a 1 cm lattice.
For a scatter shot on such a receiver, leave GGA on and turn those
messages off, keeping ones that fill the PVT panel's time and velocity
rows without touching position (NAV-TIMEUTC, NAV-VELNED); GGA's
quality indicator still supplies the fix state. Turning off the
satellite and DOP messages also loses the sky view and the Sats, DOP
and Acc lines, so take the top-of-tab shot before doing so.

### Output rate

The Allystar TAU951M-P200 is 5Hz-native and the CFG-MSG rate is a
divisor of the native measurement cycle, so rate 1 means 5Hz and
`allystar.toml` has only rate-1 entries. The only place this shows in
an image is the scatter's point count and cloud density.

### Septentrio in the Packets tab

The mosaic-G5's ChannelStatus block is too big for the packets table;
it is not a good packets shot.
