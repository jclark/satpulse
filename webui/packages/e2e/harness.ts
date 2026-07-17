// Playwright fixtures that launch the real satpulse binaries the same way the
// smoketest suite does -- a FIFO packet-log replay or the u-blox simulator, no
// GPS hardware -- and hand each test a base URL to a live frontend.
//
// The binaries come from out/<arch> at the repo root (make has already run, the
// smoketest's contract). Fixtures whose data source never runs dry (the ubxsim
// simulator, the fixed-port server) are worker-scoped and shared by the
// project's tests; the FIFO-replay fixtures are test-scoped, because exactly
// one finite replay runs per daemon lifetime, so a shared daemon would leave
// every spec file after the first with a dead stream (satpulsewb caches only
// the sticky events, not satellites or packets). A failing test keeps its
// run dir for inspection; a clean teardown removes it.

import { test as base, expect, TestInfo } from '@playwright/test';
import { ChildProcess, spawn, spawnSync } from 'node:child_process';
import * as fs from 'node:fs';
import * as http from 'node:http';
import * as net from 'node:net';
import * as os from 'node:os';
import * as path from 'node:path';

const REPO = path.resolve(__dirname, '..', '..', '..');

// Parent directory for the per-worker run dirs. Defaults to the system temp
// dir; CI sets SATPULSE_E2E_RUNDIR_PARENT to a workspace path so a failing
// run's kept dirs (daemon logs, rendered config) land somewhere the workflow
// can collect as an artifact.
const RUNDIR_PARENT = process.env.SATPULSE_E2E_RUNDIR_PARENT || os.tmpdir();

// The realtime speedup for the FIFO replay. The log spans 30s of receiver time,
// so factor 5 replays it in ~6s: fast enough that population and a clock tick
// land well inside a test, slow enough that successive time events stay
// distinct. The simulator ignores it (it paces its own nav in real time).
const REPLAY_FACTOR = 5;

// The read-only FIFO replay log: the daemon-facing capture the smoketest's
// dashboard-shaped scenarios use, carrying satellites, time and position.
const REPLAY_LOG = 'gps/testdata/packets/u-blox/ZED-F9P/daemon-sats-pos-38400.jsonl';

// The u-blox F9P simulator inputs: the personality it answers probes with and
// the nav bank it replays, both under gps/.
const UBXSIM_PERSONALITY = 'gps/app/ubxsim/testdata/f9p/f9p-personality.ubx';
const UBXSIM_BANK = 'gps/testdata/config/u-blox/ZED-F9P/sim.jsonl';
const UBXSIM_SPEED = '38400';

// The workbench message-file library the catalog/send tests read from. Exposed
// to every workbench fixture via SATPULSE_GPSMSG_PATH so those tests need no
// per-test wiring.
const GPSMSG_PATH = path.join(REPO, 'configs', 'gpsmsg');

// The fake NTRIP caster feeding the corrections tests: fakesource.py streaming a
// base-station RTCM capture (MSM7 plus 1005 station coordinates), the same log
// and credentials the smoketest's stream/wb-corrections scenario uses.
const CASTER_LOG = 'gps/testdata/packets/u-blox/ZED-F9P/base-arp.jsonl';
const CASTER_MOUNTPOINT = 'RTCM';
const CASTER_USERNAME = 'smoke';
const CASTER_PASSWORD = 'smoketest';

// The workbench URL satpulsewb prints at startup, e.g.
//   http://127.0.0.1:15754/?t=RM3YmI3_Hl-G5EbdrlkmiA
// The bracketed alternative matches an IPv6 host; the token group is absent when
// the token is disabled (-L without -T). Matches program_satpulsewb.py's regex.
const WB_URL_RE = /http:\/\/(?:\[[^\]]+\]|[^\s/]+?):(\d+)\/(?:\?t=(\S+))?/;

// --- process and readiness helpers ------------------------------------------

function buildDir(): string {
  // Honour GOOS/GOARCH like the smoketest so a cross-built tree runs against its
  // own binaries; default to the host. Linux uses out/<arch>, other Unix
  // out/<goos>_<arch>, matching unix-build.sh.
  const goos = process.env.GOOS || (process.platform === 'darwin' ? 'darwin' : 'linux');
  let goarch = process.env.GOARCH;
  if (!goarch) {
    goarch = process.arch === 'x64' ? 'amd64' : process.arch === 'arm64' ? 'arm64' : process.arch;
  }
  return goos === 'linux' ? goarch : `${goos}_${goarch}`;
}

function binPath(name: string): string {
  return path.join(REPO, 'out', buildDir(), name);
}

const satpulsed = binPath('satpulsed');
const satpulsewb = binPath('satpulsewb');
const satpulsetool = binPath('satpulsetool');

function repoPath(rel: string): string {
  return path.isAbsolute(rel) ? rel : path.join(REPO, rel);
}

// Proc pairs a child with its log file so a failure can quote the tail.
interface Proc {
  name: string;
  child: ChildProcess;
  logPath: string;
  exited: boolean;
  exitCode: number | null;
}

// A RunSession owns one launched server's run dir, processes and teardown. The
// worker fixture keeps the dir when a test in the worker failed (keep), else
// removes it.
class RunSession {
  runDir: string;
  keep = false;
  private procs: Proc[] = [];

  constructor(prefix: string) {
    fs.mkdirSync(RUNDIR_PARENT, { recursive: true });
    this.runDir = fs.mkdtempSync(path.join(RUNDIR_PARENT, prefix + '-'));
  }

  // spawnLogged starts a process with stdout+stderr redirected to <name>.log in
  // the run dir, tracks its exit, and records it for teardown and error quoting.
  spawnLogged(name: string, cmd: string, args: string[], env?: NodeJS.ProcessEnv): Proc {
    const logPath = path.join(this.runDir, name + '.log');
    const fd = fs.openSync(logPath, 'a');
    const child = spawn(cmd, args, {
      stdio: ['ignore', fd, fd],
      env: env ? { ...process.env, ...env } : process.env,
    });
    fs.closeSync(fd);
    const proc: Proc = { name, child, logPath, exited: false, exitCode: null };
    child.on('exit', (code) => {
      proc.exited = true;
      proc.exitCode = code;
    });
    this.procs.push(proc);
    return proc;
  }

  logTail(proc: Proc, lines = 40): string {
    try {
      return fs.readFileSync(proc.logPath, 'utf8').split('\n').slice(-lines).join('\n').trim();
    } catch {
      return '(no log)';
    }
  }

  // deadIfExited fails fast with the log tail when a process the harness is
  // waiting on has already died, so a startup miswiring surfaces its cause
  // rather than an opaque timeout.
  deadIfExited(proc: Proc): void {
    if (proc.exited) {
      throw new Error(
        `${proc.name} exited early (code ${proc.exitCode})\n--- ${proc.name}.log tail ---\n${this.logTail(proc)}`,
      );
    }
  }

  async teardown(): Promise<void> {
    // SIGTERM every process (satpulsed/satpulsewb treat it as graceful
    // shutdown), newest first so a replay/simulator is stopped before the
    // server it feeds, then wait briefly for each. See the per-fixture ordering
    // notes for the ubxsim exception.
    for (const proc of [...this.procs].reverse()) {
      await this.stop(proc);
    }
    if (this.keep) {
      console.log(`e2e: kept run dir for inspection: ${this.runDir}`);
    } else {
      fs.rmSync(this.runDir, { recursive: true, force: true });
    }
  }

  async stop(proc: Proc, grace = 5000): Promise<void> {
    if (proc.exited || proc.child.pid === undefined) return;
    proc.child.kill('SIGTERM');
    await this.waitExit(proc, grace);
    if (!proc.exited) {
      proc.child.kill('SIGKILL');
      await this.waitExit(proc, 2000);
    }
  }

  private waitExit(proc: Proc, timeout: number): Promise<void> {
    return new Promise((resolve) => {
      if (proc.exited) return resolve();
      const timer = setTimeout(resolve, timeout);
      proc.child.once('exit', () => {
        clearTimeout(timer);
        resolve();
      });
    });
  }
}

// freePort asks the kernel for an unused loopback TCP port and releases it. With
// workers: 1 the window between release and the daemon's bind is small; the
// daemon logs and the readiness poll would surface a lost race.
function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.once('error', reject);
    srv.listen(0, '127.0.0.1', () => {
      const addr = srv.address();
      if (addr === null || typeof addr === 'string') {
        srv.close();
        return reject(new Error('could not allocate a port'));
      }
      const port = addr.port;
      srv.close(() => resolve(port));
    });
  });
}

async function poll(what: string, deadlineMs: number, fn: () => Promise<boolean> | boolean): Promise<void> {
  const deadline = Date.now() + deadlineMs;
  for (;;) {
    if (await fn()) return;
    if (Date.now() >= deadline) throw new Error(`timed out waiting for ${what} after ${deadlineMs}ms`);
    await sleep(50);
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}

function portAccepts(port: number): Promise<boolean> {
  return new Promise((resolve) => {
    const sock = net.connect({ host: '127.0.0.1', port }, () => {
      sock.destroy();
      resolve(true);
    });
    sock.once('error', () => {
      sock.destroy();
      resolve(false);
    });
    sock.setTimeout(300, () => {
      sock.destroy();
      resolve(false);
    });
  });
}

function httpResponds(port: number, urlPath: string): Promise<boolean> {
  return new Promise((resolve) => {
    const req = http.get({ host: '127.0.0.1', port, path: urlPath, timeout: 500 }, (res) => {
      res.resume();
      resolve(true);
    });
    req.once('error', () => resolve(false));
    req.once('timeout', () => {
      req.destroy();
      resolve(false);
    });
  });
}

// mkfifo creates a named pipe. The daemon opens it O_RDWR and holds its own
// write end, so an idle FIFO never reaches EOF -- it reads as a silent-but-
// connected receiver, and the daemon and replayer can start in any order.
function mkfifo(fifoPath: string): void {
  const res = spawnSync('mkfifo', [fifoPath]);
  if (res.error) throw res.error;
  if (res.status !== 0) throw new Error(`mkfifo ${fifoPath} failed (code ${res.status})`);
}

// startReplay launches the single packet-log replay into the FIFO, once the
// daemon (the reader) is up. The shell opens the FIFO for writing -- blocking
// until the reader is present, so the writer opens after the reader -- and execs
// pack into it, giving pack a plain blocking write fd. Exactly one replay runs
// per server lifetime; concatenating replays would corrupt the RTCM frame
// boundary at the junction.
function startReplay(session: RunSession, fifoPath: string): Proc {
  const log = repoPath(REPLAY_LOG);
  return session.spawnLogged('replay', 'sh', [
    '-c',
    'exec "$0" pack --realtime "$1" "$2" > "$3"',
    satpulsetool,
    String(REPLAY_FACTOR),
    log,
    fifoPath,
  ]);
}

// waitWbUrl parses the workbench URL and token from satpulsewb's log, then waits
// for its port to accept. Printing the URL is readiness; the printed host is a
// LAN interface address, so the returned base URL always uses loopback.
async function waitWbUrl(session: RunSession, proc: Proc): Promise<{ baseURL: string; port: number; token: string }> {
  let port = 0;
  let token = '';
  await poll('satpulsewb URL', 15000, () => {
    session.deadIfExited(proc);
    let text: string;
    try {
      text = fs.readFileSync(proc.logPath, 'utf8');
    } catch {
      return false;
    }
    const m = WB_URL_RE.exec(text);
    if (m === null) return false;
    port = Number(m[1]);
    token = m[2] ?? '';
    return true;
  });
  await poll(`satpulsewb port ${port}`, 15000, () => {
    session.deadIfExited(proc);
    return portAccepts(port);
  });
  const baseURL = `http://127.0.0.1:${port}/` + (token ? `?t=${token}` : '');
  return { baseURL, port, token };
}

// --- fixture value types ----------------------------------------------------

// DashboardReplay hands the base URL to a fresh satpulsed dashboard and starts
// the FIFO replay on demand. The replay is not started at setup so a test can
// observe the empty page first, then call ensureReplay to watch it populate.
// satpulsed sends a new SSE client no data events (it has no per-name event
// cache, unlike satpulsewb), and exactly one replay runs per daemon lifetime,
// so data assertions only work on a page connected while the replay flows.
export interface DashboardReplay {
  baseURL: string;
  ensureReplay(): void;
}

// WorkbenchReplay is satpulsewb -d <fifo>: the read-only FIFO replay behind the
// workbench, so connect lands in passive detection. The replay runs from setup.
// Test-scoped: each test gets a fresh daemon and its own live replay, so data
// assertions never depend on which spec file ran first.
export interface WorkbenchReplay {
  baseURL: string;
  token: string;
  port: number;
}

// WorkbenchUbxsim is satpulsewb backed by the u-blox simulator, which answers
// probes and config so the config path works. withDevice selects the -d <link>
// launch (active detection from startup); without it satpulsewb starts with no
// device for the connection-panel tests, and devicePath is the simulator's pty
// symlink to type into that panel. Test-scoped: config tests mutate the
// simulator's config database (VALSETs, saves to Flash and BBR, resets), so a
// shared simulator would couple tests through persistent state.
export interface WorkbenchUbxsim {
  baseURL: string;
  token: string;
  port: number;
  devicePath: string;
}

// WorkbenchFixed is satpulsewb -L 127.0.0.1:<port> with no token, on a stable
// port. restart() stops and relaunches satpulsewb on the same port, for the
// reconnect test; a fresh server generates a fresh (here absent) token.
export interface WorkbenchFixed {
  baseURL: string;
  port: number;
  restart(): Promise<void>;
}

// WorkbenchFixedToken is satpulsewb -L 127.0.0.1:<port> -T: a stable port WITH
// a generated token, so restart() invalidates the page's token while the page
// can still reach the port -- the stale-token-after-restart path. Test-scoped:
// a restart burns the fixture's token, so it cannot be shared.
export interface WorkbenchFixedToken {
  baseURL: string;
  token: string;
  port: number;
  restart(): Promise<{ baseURL: string; token: string }>;
}

// WorkbenchCaster is satpulsewb over the ubxsim pty plus a running fake NTRIP
// caster the corrections tests point their source form at. The pty (not the
// read-only FIFO) matters: a correction session writes the RTCM back to the
// port, a write the FIFO fails fatally, ending the session at the first
// packet; the simulator consumes the writes, so a session stays connected --
// the same reason the smoketest's wb-corrections scenario runs on a pty.
export interface WorkbenchCaster {
  baseURL: string;
  token: string;
  port: number;
  caster: { host: string; port: number; mountpoint: string; username: string; password: string };
}

// --- launchers --------------------------------------------------------------

async function launchSatpulsed(session: RunSession): Promise<{ fifoPath: string; httpPort: number }> {
  const fifoPath = path.join(session.runDir, 'gps.fifo');
  mkfifo(fifoPath);
  const httpPort = await freePort();
  // Minimal unprivileged config: the FIFO as the serial device and one HTTP
  // endpoint, nothing else, mirroring smoketest scenarios/http/full.toml.in.
  const toml = `[serial]\ndevice = "${fifoPath}"\n\n[[http]]\nlisten = "127.0.0.1:${httpPort}"\n`;
  const configPath = path.join(session.runDir, 'satpulse.toml');
  fs.writeFileSync(configPath, toml);
  const proc = session.spawnLogged('satpulsed', satpulsed, ['-v', '-f', configPath]);
  // The daemon brings up its HTTP listener only after GPS detection times out
  // (~2s with no data), so waiting for the endpoint to respond guarantees the
  // SSE observer exists before any replay packet flows.
  await poll(`satpulsed http ${httpPort}`, 20000, () => {
    session.deadIfExited(proc);
    return httpResponds(httpPort, '/');
  });
  return { fifoPath, httpPort };
}

async function launchWbFifo(session: RunSession, extraArgs: string[]): Promise<{ fifoPath: string } & Awaited<ReturnType<typeof waitWbUrl>>> {
  const fifoPath = path.join(session.runDir, 'gps.fifo');
  mkfifo(fifoPath);
  const proc = session.spawnLogged('satpulsewb', satpulsewb, ['-d', fifoPath, ...extraArgs], {
    SATPULSE_GPSMSG_PATH: GPSMSG_PATH,
  });
  const info = await waitWbUrl(session, proc);
  return { fifoPath, ...info };
}

async function launchUbxsim(session: RunSession): Promise<string> {
  // The simulator must exist before satpulsewb opens the device, and it holds
  // its own slave fd open so a program restart never EOFs it. The pty symlink
  // appearing is readiness.
  const link = path.join(session.runDir, 'gps.pty');
  const proc = session.spawnLogged('ubxsim', satpulsetool, [
    'ubxsim',
    '--link',
    link,
    '-r',
    repoPath(UBXSIM_BANK),
    repoPath(UBXSIM_PERSONALITY),
  ]);
  await poll('ubxsim symlink', 10000, () => {
    session.deadIfExited(proc);
    return fs.existsSync(link);
  });
  return link;
}

function startCaster(session: RunSession, port: number): Proc {
  return session.spawnLogged('caster', 'python3', [
    repoPath('smoketest/scenarios/stream/fakesource.py'),
    `127.0.0.1:${port}`,
    repoPath(CASTER_LOG),
    '--pack',
    satpulsetool,
    '--factor',
    String(REPLAY_FACTOR),
    '--mountpoint',
    CASTER_MOUNTPOINT,
    '--username',
    CASTER_USERNAME,
    '--password',
    CASTER_PASSWORD,
  ]);
}

// --- fixtures ---------------------------------------------------------------

// Worker fixtures track their session in openSessions so the auto per-test
// fixture can mark them for keeping when a test fails.
const openSessions = new Set<RunSession>();

// keepIfFailed marks a TEST-scoped session for keeping when its test failed.
// Called right after use() returns, during the session fixture's own teardown:
// the auto keepOnFailure fixture tears down only after test-scoped fixtures,
// too late to save their run dirs. testInfo.status is set by then.
function keepIfFailed(session: RunSession, testInfo: TestInfo): void {
  if (testInfo.status !== testInfo.expectedStatus) session.keep = true;
}

interface WorkerFixtures {
  dashboardReplay: DashboardReplay;
  workbenchFixed: WorkbenchFixed;
  workbenchCaster: WorkbenchCaster;
}

interface TestFixtures {
  keepOnFailure: void;
  workbenchReplay: WorkbenchReplay;
  workbenchUbxsim: WorkbenchUbxsim;
  workbenchUbxsimNoDevice: WorkbenchUbxsim;
  workbenchFixedToken: WorkbenchFixedToken;
}

export const test = base.extend<TestFixtures, WorkerFixtures>({
  // A failing test keeps every open WORKER-scoped session's run dir; a passing
  // one leaves the teardown to remove it. This only covers the worker-scoped
  // sessions: an auto fixture is set up first and so torn down last, after the
  // test-scoped session fixtures have already torn down, so those mark their
  // own session from testInfo before their teardown instead.
  keepOnFailure: [
    async ({}, use, testInfo) => {
      await use();
      if (testInfo.status !== testInfo.expectedStatus) {
        for (const s of openSessions) s.keep = true;
      }
    },
    { auto: true },
  ],

  dashboardReplay: [
    async ({}, use) => {
      const session = new RunSession('satpulse-e2e-dashboard');
      openSessions.add(session);
      let started = false;
      try {
        const { fifoPath, httpPort } = await launchSatpulsed(session);
        const value: DashboardReplay = {
          baseURL: `http://127.0.0.1:${httpPort}/`,
          ensureReplay() {
            if (started) return;
            started = true;
            startReplay(session, fifoPath);
          },
        };
        await use(value);
      } catch (e) {
        // A fixture setup failure never reaches keepOnFailure, so keep the run
        // dir (config and logs) here.
        session.keep = true;
        throw e;
      } finally {
        openSessions.delete(session);
        await session.teardown();
      }
    },
    { scope: 'worker' },
  ],

  workbenchReplay: async ({}, use, testInfo) => {
    const session = new RunSession('satpulse-e2e-wb');
    openSessions.add(session);
    try {
      const info = await launchWbFifo(session, []);
      startReplay(session, info.fifoPath);
      await use({ baseURL: info.baseURL, token: info.token, port: info.port });
      keepIfFailed(session, testInfo);
    } catch (e) {
      session.keep = true;
      throw e;
    } finally {
      openSessions.delete(session);
      await session.teardown();
    }
  },

  workbenchFixedToken: async ({}, use, testInfo) => {
    const session = new RunSession('satpulse-e2e-wb-fixed-token');
    openSessions.add(session);
    try {
      const port = await freePort();
      let proc: Proc | null = null;
      let launches = 0;
      // Each launch logs to its own file: waitWbUrl takes the first URL in the
      // log, and a restart must yield the fresh token, not the burnt one.
      const launch = async (): Promise<{ baseURL: string; token: string }> => {
        launches++;
        proc = session.spawnLogged(`satpulsewb-${launches}`, satpulsewb, ['-L', `127.0.0.1:${port}`, '-T'], {
          SATPULSE_GPSMSG_PATH: GPSMSG_PATH,
        });
        const info = await waitWbUrl(session, proc);
        return { baseURL: info.baseURL, token: info.token };
      };
      const first = await launch();
      const value: WorkbenchFixedToken = {
        baseURL: first.baseURL,
        token: first.token,
        port,
        async restart() {
          if (proc) await session.stop(proc);
          return launch();
        },
      };
      await use(value);
      keepIfFailed(session, testInfo);
    } catch (e) {
      session.keep = true;
      throw e;
    } finally {
      openSessions.delete(session);
      await session.teardown();
    }
  },

  workbenchUbxsim: async ({}, use, testInfo) => {
    await useUbxsim(use, true, testInfo);
  },

  workbenchUbxsimNoDevice: async ({}, use, testInfo) => {
    await useUbxsim(use, false, testInfo);
  },

  workbenchFixed: [
    async ({}, use) => {
      const session = new RunSession('satpulse-e2e-wb-fixed');
      openSessions.add(session);
      let proc: Proc | null = null;
      try {
        const port = await freePort();
        const launch = async (): Promise<void> => {
          proc = session.spawnLogged('satpulsewb', satpulsewb, ['-L', `127.0.0.1:${port}`], {
            SATPULSE_GPSMSG_PATH: GPSMSG_PATH,
          });
          await poll(`satpulsewb port ${port}`, 15000, () => {
            session.deadIfExited(proc as Proc);
            return portAccepts(port);
          });
        };
        await launch();
        const value: WorkbenchFixed = {
          baseURL: `http://127.0.0.1:${port}/`,
          port,
          async restart() {
            if (proc) await session.stop(proc);
            await launch();
          },
        };
        await use(value);
      } catch (e) {
        session.keep = true;
        throw e;
      } finally {
        openSessions.delete(session);
        await session.teardown();
      }
    },
    { scope: 'worker' },
  ],

  workbenchCaster: [
    async ({}, use) => {
      const session = new RunSession('satpulse-e2e-wb-corr');
      openSessions.add(session);
      try {
        const casterPort = await freePort();
        // The caster listens before the workbench connects to it; the workbench
        // only dials once the corrections form is submitted, so ordering is not
        // critical, but starting it first keeps parity with the smoketest. Wait
        // for its listener so a caster that dies at startup fails here with its
        // log tail, not later as an opaque connection refusal.
        const caster = startCaster(session, casterPort);
        await poll(`caster port ${casterPort}`, 10000, () => {
          session.deadIfExited(caster);
          return portAccepts(casterPort);
        });
        const link = await launchUbxsim(session);
        const proc = session.spawnLogged('satpulsewb', satpulsewb, ['-d', link, '-s', UBXSIM_SPEED], {
          SATPULSE_GPSMSG_PATH: GPSMSG_PATH,
        });
        const info = await waitWbUrl(session, proc);
        await use({
          baseURL: info.baseURL,
          token: info.token,
          port: info.port,
          caster: {
            host: '127.0.0.1',
            port: casterPort,
            mountpoint: CASTER_MOUNTPOINT,
            username: CASTER_USERNAME,
            password: CASTER_PASSWORD,
          },
        });
      } catch (e) {
        session.keep = true;
        throw e;
      } finally {
        openSessions.delete(session);
        await session.teardown();
      }
    },
    { scope: 'worker' },
  ],
});

// useUbxsim launches the simulator, then satpulsewb over it (with or without the
// device flag), and tears them down in the required order: satpulsewb first,
// then the simulator, so an early simulator kill cannot inject read errors into
// satpulsewb's shutdown. RunSession.teardown stops newest first, and the
// simulator is spawned first, so this ordering holds.
async function useUbxsim(use: (v: WorkbenchUbxsim) => Promise<void>, withDevice: boolean, testInfo: TestInfo): Promise<void> {
  const session = new RunSession('satpulse-e2e-wb-ubxsim');
  openSessions.add(session);
  try {
    const link = await launchUbxsim(session);
    const args = withDevice ? ['-d', link, '-s', UBXSIM_SPEED] : [];
    const proc = session.spawnLogged('satpulsewb', satpulsewb, args, {
      SATPULSE_GPSMSG_PATH: GPSMSG_PATH,
    });
    const info = await waitWbUrl(session, proc);
    await use({ baseURL: info.baseURL, token: info.token, port: info.port, devicePath: link });
    keepIfFailed(session, testInfo);
  } catch (e) {
    session.keep = true;
    throw e;
  } finally {
    openSessions.delete(session);
    await session.teardown();
  }
}

export { expect };
