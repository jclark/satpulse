import { test, expect } from '../harness';
import type { Page } from '@playwright/test';

// The Configuration tab's persistent operations (Save and Reset), on the
// workbenchUbxsim fixture. Save and Reset have no readback to populate from,
// so they are tested by what survives: Reset=Reload (a CFG-CFG load rebuilding
// RAM from the saved layers, no hardware reset) is the observation tool, and a
// page reload forces a fresh readback. Each test gets a fresh simulator with a
// pristine config database whose Period reads back as 1.
//
// The simulator answers config immediately but paces nav in real time; every
// wait is an expect poll on visible state, never a bare sleep. The apply and
// reload-readback round-trips can approach the default timeout, so these tests
// run test.slow().

const PERIOD = 'e.g. 1.0';

async function gotoWorkbench(page: Page, baseURL: string) {
  await page.goto(baseURL);
  await expect(page.getByRole('heading', { name: 'SatPulse', exact: true })).toBeVisible();
  await expect(page.getByText('Connected', { exact: true })).toBeVisible();
}

async function openConfig(page: Page) {
  const tab = page.getByRole('button', { name: 'Configuration' });
  await expect(tab).toBeEnabled({ timeout: 30_000 });
  await tab.click();
  await page.getByRole('button', { name: 'Time pulse', exact: true }).click();
  const period = page.getByPlaceholder(PERIOD);
  await expect(period).toBeEnabled({ timeout: 30_000 });
  await expect(period).not.toHaveValue('', { timeout: 30_000 });
}

async function reloadAndOpenConfig(page: Page) {
  await page.reload();
  await expect(page.getByText('Connected', { exact: true })).toBeVisible({ timeout: 30_000 });
  await openConfig(page);
}

async function applyAndWait(page: Page) {
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  await expect(page.getByText('Configuration applied')).toBeVisible({ timeout: 30_000 });
}

test('unsaved changes are volatile under Reset=Reload', async ({ page, workbenchUbxsim }) => {
  test.slow();
  await gotoWorkbench(page, workbenchUbxsim.baseURL);
  await openConfig(page);

  // Apply a period change with Save=Nothing (the default): RAM only.
  await page.getByPlaceholder(PERIOD).fill('2');
  await applyAndWait(page);

  // Reset=Reload alone rebuilds RAM from the saved layers, discarding the
  // unsaved change.
  await page.getByRole('radio', { name: 'Reload' }).check();
  await expect(page.getByText(/Changes pending to reset/)).toBeVisible();
  await applyAndWait(page);

  await reloadAndOpenConfig(page);
  await expect(page.getByPlaceholder(PERIOD)).toHaveValue('1');
});

test('Save=Changes persists across Reset=Reload and a further reload', async ({ page, workbenchUbxsim }) => {
  test.slow();
  await gotoWorkbench(page, workbenchUbxsim.baseURL);
  await openConfig(page);

  // Apply the change with Save=Changes: the VALSET is written to the Flash,
  // BBR and RAM layers.
  await page.getByPlaceholder(PERIOD).fill('2');
  await page.getByRole('radio', { name: 'Changes', exact: true }).check();
  await applyAndWait(page);

  // Reset=Reload rebuilds RAM from the saved layers: the change survives.
  await page.getByRole('radio', { name: 'Reload' }).check();
  await applyAndWait(page);

  await reloadAndOpenConfig(page);
  await expect(page.getByPlaceholder(PERIOD)).toHaveValue('2');

  // And it still reads back after another page reload.
  await reloadAndOpenConfig(page);
  await expect(page.getByPlaceholder(PERIOD)).toHaveValue('2');
});

test('Save=All persists a previously-unsaved change', async ({ page, workbenchUbxsim }) => {
  test.slow();
  await gotoWorkbench(page, workbenchUbxsim.baseURL);
  await openConfig(page);

  // Apply the change with Save=Nothing: RAM only.
  await page.getByPlaceholder(PERIOD).fill('2');
  await applyAndWait(page);

  // Save=All alone saves the whole current configuration (CFG-CFG save).
  await page.getByRole('radio', { name: 'All', exact: true }).check();
  await expect(page.getByText(/Changes pending to save/)).toBeVisible();
  await applyAndWait(page);

  // Reset=Reload then confirms the change was persisted, not just in RAM.
  await page.getByRole('radio', { name: 'Reload' }).check();
  await applyAndWait(page);

  await reloadAndOpenConfig(page);
  await expect(page.getByPlaceholder(PERIOD)).toHaveValue('2');
});

test('a cold start drops an unsaved change while the connection survives', async ({ page, workbenchUbxsim }) => {
  test.slow();
  await gotoWorkbench(page, workbenchUbxsim.baseURL);
  await openConfig(page);

  // Apply an unsaved change, then Reset=Cold start (a hardware reset). The
  // simulator keeps its pty open across the simulated reboot, so the
  // connection survives.
  await page.getByPlaceholder(PERIOD).fill('3');
  await applyAndWait(page);
  await page.getByRole('radio', { name: 'Cold start' }).check();
  await expect(page.getByText(/Changes pending to reset/)).toBeVisible();
  await applyAndWait(page);
  await expect(page.getByText('Connected', { exact: true })).toBeVisible();

  // A fresh readback shows the unsaved change gone.
  await reloadAndOpenConfig(page);
  await expect(page.getByPlaceholder(PERIOD)).toHaveValue('1');
});

test('a factory reset drops an unsaved change while the connection survives', async ({ page, workbenchUbxsim }) => {
  test.slow();
  await gotoWorkbench(page, workbenchUbxsim.baseURL);
  await openConfig(page);

  await page.getByPlaceholder(PERIOD).fill('3');
  await applyAndWait(page);
  await page.getByRole('radio', { name: 'Factory reset' }).check();
  await expect(page.getByText(/Changes pending to reset/)).toBeVisible();
  await applyAndWait(page);
  await expect(page.getByText('Connected', { exact: true })).toBeVisible();

  await reloadAndOpenConfig(page);
  await expect(page.getByPlaceholder(PERIOD)).toHaveValue('1');
});
