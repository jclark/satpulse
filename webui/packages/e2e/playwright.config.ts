import { defineConfig } from '@playwright/test';

// The suite launches real satpulsed/satpulsewb daemons and replays packet logs,
// so a single worker runs the tests serially against its worker-scoped servers
// (several fixtures' servers can be alive at once, each on its own free port).
// Two projects mirror the two frontends and the
// two testDirs; either runs alone with --project. Traces are kept only for a
// failing test, where they are worth the disk. Assertions wait out the replay
// pacing, so the expect timeout is well above Playwright's 5s default; the test
// timeout also has to cover the ubxsim fixture, whose simulator emits nav in
// real time (one epoch per second).
export default defineConfig({
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  // List locally; on CI add an HTML report so a failing run uploads a browsable
  // artifact alongside the traces.
  reporter: process.env.CI
    ? [['list'], ['html', { open: 'never' }]]
    : [['list']],
  use: {
    browserName: 'chromium',
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'dashboard', testDir: './dashboard' },
    { name: 'workbench', testDir: './workbench' },
  ],
});
