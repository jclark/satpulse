import { test, expect } from '../harness';
import type { Locator, Page } from '@playwright/test';

// The satpulsed dashboard, driven by the FIFO replay. Basic render checks only:
// this is the surface the web redesign (#284) reworks first, so the assertions
// use ARIA-role and visible-text selectors that survive a restyle, not layout
// or visual comparisons. The dashboard renders one card per SSE event name, so
// each card's heading and one of its field labels stand in for "this card
// rendered its data".

// fieldValue returns the value span beside a "Label:" field. FieldElement lays
// the two out as sibling spans, so the value is the label span's next sibling.
function fieldValue(page: Page, label: string): Locator {
  return page.getByText(`${label}:`, { exact: true }).locator('xpath=following-sibling::span[1]');
}

// One test, one page: satpulsed sends a new SSE client no init events (only
// satpulsewb caches the latest event per name), so data only reaches a page
// that is connected while the single replay flows. Every data assertion
// therefore lives on the one page opened before the replay starts.
test('startup: page renders empty, the replay populates the cards, the clock advances', async ({ page, dashboardReplay }) => {
  // A fresh daemon primes no init events, so the page mounts empty: the app
  // shell is present but no card has rendered yet.
  await page.goto(dashboardReplay.baseURL);
  await expect(page).toHaveTitle('satpulse');
  await expect(page.locator('#root > div')).toBeAttached();
  await expect(page.getByRole('heading')).toHaveCount(0);

  // The replay then feeds satellites, time, position and quality; the satellite
  // display goes from empty to populated, a position appears, and the clock
  // ticks as successive time events arrive.
  dashboardReplay.ensureReplay();

  await expect(page.getByRole('heading', { name: 'Signal Levels' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Position', exact: true })).toBeVisible();
  await expect(fieldValue(page, 'Coordinates')).toBeVisible();

  const clock = fieldValue(page, 'Local time');
  await expect(clock).toBeVisible();
  const first = await clock.innerText();
  await expect.poll(async () => clock.innerText(), { timeout: 10_000 }).not.toBe(first);

  // The remaining cards render their data from the same replay, on the same
  // still-connected page.
  await expect(page.getByRole('heading', { name: 'Current GPS Time' })).toBeVisible();
  await expect(fieldValue(page, 'UTC')).toBeVisible();

  await expect(page.getByRole('heading', { name: 'Status', exact: true })).toBeVisible();
  await expect(fieldValue(page, 'Fix type')).toBeVisible();

  await expect(page.getByRole('heading', { name: 'Position Quality' })).toBeVisible();
  await expect(fieldValue(page, 'Horizontal accuracy')).toBeVisible();
});
