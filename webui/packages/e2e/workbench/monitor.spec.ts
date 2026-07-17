import { test, expect } from '../harness';
import type { Page } from '@playwright/test';

// The workbench Monitor and Packets tabs and the Message file catalog, driven
// by the FIFO replay. Basic render checks only: the redesign (#284) reworks
// these widgets, so everything is asserted through ARIA roles and visible
// text, never layout or visuals. The map is excluded: it fetches
// OpenStreetMap tiles over the network, which CI must not depend on.
//
// Timing: the replay runs from fixture setup and lasts ~6s of wall time.
// satpulsewb caches only the sticky events (gps:time, gps:epochPVT, ...) for
// late joiners; the high-rate gps:msg kinds (satellites, navEpoch, pos/vel/
// time rows) and the gps:packet stream are never cached (see
// cmd/satpulsewb/sink.go), so every assertion that depends on them must sit
// on a page that is connected while the replay flows. That forces the Monitor
// and Packets checks into one test on one page, with the packet-stream
// subscription made first (it only streams while the Packets tab is open).

// The clock renders receiver time in the browser's local zone; pin it so the
// date asserted below is the replay log's UTC date on any machine.
test.use({ timezoneId: 'UTC' });

// visibleCell returns the first visible table cell with the given text. Both
// tabs stay mounted (the inactive one is display:none) and several message
// names appear in both the packet table and the PVT tables, so cell lookups
// filter to the visible tab.
function visibleCell(page: Page, name: string) {
  return page.getByRole('cell', { name, exact: true }).filter({ visible: true }).first();
}

test('packets and monitor panels populate from the replay', async ({ page, workbenchReplay }) => {
  await page.goto(workbenchReplay.baseURL);

  // Monitor is the default tab: its collapsible sections are already there.
  await expect(page.getByRole('button', { name: 'PVT Messages' })).toBeVisible();

  // Packets tab first: the gps:packet stream flows only while this tab is
  // subscribed and is never cached, so it must be opened while the replay is
  // still running. Message names come from the replayed u-blox log.
  await page.getByRole('button', { name: 'Packets' }).click();
  await expect(page.getByRole('columnheader', { name: 'Last message' })).toBeVisible();
  await expect(visibleCell(page, 'NAV-PVT')).toBeVisible();
  await expect(visibleCell(page, 'NAV-SAT')).toBeVisible();
  await expect(visibleCell(page, 'UBX')).toBeVisible();
  await expect(visibleCell(page, 'Rx')).toBeVisible();

  // Back to Monitor. The app-level listeners accumulated the monitor state
  // while the Packets tab was open, so these panels are already populated
  // (or populate as the tail of the replay arrives).
  await page.getByRole('button', { name: 'Monitor' }).click();

  // Clock: the replay log's UTC date, rendered once gps:time arrives.
  await expect(page.getByText('2026-03-26', { exact: true })).toBeVisible();

  // Summary: the fix and satellite-count lines from the nav epoch.
  await expect(page.getByText(/Fix: /).first()).toBeVisible();
  await expect(page.getByText(/Sats: /).first()).toBeVisible();

  // Sky view: the legend lists the constellations present in the satellite
  // data (the log carries GPS and Galileo among others).
  const legendItem = (label: string) =>
    page.getByRole('listitem').filter({ hasText: new RegExp(`^${label}$`) }).filter({ visible: true });
  await expect(legendItem('GPS')).toBeVisible();
  await expect(legendItem('GAL')).toBeVisible();

  // PVT Messages auto-expands on first PVT data (app.tsx), so its tables are
  // visible without a click: position rows with coordinates and a time row,
  // keyed by the log's native message IDs.
  await expect(page.getByRole('heading', { name: 'Position', exact: true })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Latitude,Longitude' }).filter({ visible: true })).toBeVisible();
  await expect(visibleCell(page, 'NAV-HPPOSLLH')).toBeVisible();
  await expect(visibleCell(page, 'NAV-TIMEGPS')).toBeVisible();

  // Position Scatter is collapsed by default; opened, it has at least the
  // point primed from the cached epoch PVT, so the statistics render.
  await page.getByRole('button', { name: 'Position Scatter' }).click();
  await expect(page.getByRole('heading', { name: 'Precision' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Mean ECEF' })).toBeVisible();

  // Satellite Signals is collapsed by default; opened, it shows the
  // constellation filter buttons and the per-signal table.
  await page.getByRole('button', { name: 'Satellite Signals' }).click();
  await expect(page.getByText('Constellations:')).toBeVisible();
  await expect(page.getByRole('button', { name: 'GPS', exact: true })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'CN0' })).toBeVisible();

  // Survey: the log carries no survey messages, so the section stays
  // collapsed (no auto-expand); opened, it renders its field labels.
  await page.getByRole('button', { name: 'Survey', exact: true }).click();
  await expect(page.getByText('Observation time')).toBeVisible();
  await expect(page.getByText('Status', { exact: true })).toBeVisible();
});

// The catalog needs no replay data: the harness points SATPULSE_GPSMSG_PATH
// at configs/gpsmsg for every workbench fixture, and a fresh page claims the
// write seat on load, so Load works whenever this test runs.
test('message file catalog: dropdowns populate and a loaded file shows its tags', async ({ page, workbenchReplay }) => {
  await page.goto(workbenchReplay.baseURL);
  await page.getByRole('button', { name: 'Message file' }).click();

  // The vendor dropdown lists the configs/gpsmsg vendor directories and the
  // file dropdown that vendor's files (selectOption waits for the options).
  const vendor = page.getByLabel('Vendor').filter({ visible: true });
  await vendor.selectOption('u-blox');
  await expect(vendor.locator('option[value="unicore"]')).toHaveCount(1);
  await page.getByLabel('File').filter({ visible: true }).selectOption('ubx');
  await page.getByRole('button', { name: 'Load', exact: true }).click();

  // Loading shows the resolved path and the file's tag table.
  await expect(page.getByText(/u-blox\/ubx\.toml/)).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Tag', exact: true }).filter({ visible: true })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: '# Msgs' })).toBeVisible();
  await expect(visibleCell(page, 'get-pubx-04')).toBeVisible();
  await expect(visibleCell(page, 'get-gnss')).toBeVisible();
});
