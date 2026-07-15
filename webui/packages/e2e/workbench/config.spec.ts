import { test, expect } from '../harness';
import type { Page, Locator } from '@playwright/test';

// The workbench Configuration tab, driven by the u-blox F9P simulator
// (workbenchUbxsim fixture: satpulsewb -d <pty> -s 38400). The simulator
// answers probes and config, so the readback/apply path works; its config
// database returns VALSET values verbatim, so properties round-trip. Each test
// gets a FRESH simulator with a pristine config database and a fresh satpulsewb
// already connected, so no state leaks between tests.
//
// Selectors are ARIA roles and the visible text quoted from config-panel.tsx
// (and its group components), never layout or styling. The bottom action bar's
// status label walks "Changes pending to ...", "Applying configuration...",
// "Configuration applied", which the tests assert against.
//
// Grounded readback values for this personality (ZED-F9P, FW HPG 1.51):
//   time pulse: period 1, width 0.1, cable delay 50, Time GNSS "--",
//     align/locked/rising all checked
//   time mode:  Mobile
//   signals:    all six constellations (BDS GAL GLO GPS QZSS SBAS) enabled,
//     min elevation 10
//   serial:     38400
//   NAV-SAT is off in the defaults, so no satellite data flows until the
//   satellites messages are enabled and applied.

// The automatic readback drives connState to "configuring" and back, which
// disables and then re-enables the panel. Waiting for the Period input to be
// enabled with a value is the reliable signal that the first readback landed.
const PERIOD = 'e.g. 1.0';
const WIDTH = 'e.g. 0.1';
const CABLE = 'e.g. 50';
const MINELEV = 'e.g. 10';

// ph resolves a config field by its exact placeholder. Exact matters: the
// corrections panel's host input placeholder "e.g. 10.0.0.1" contains the
// "e.g. 10" of the minimum-elevation field.
function ph(page: Page, placeholder: string): Locator {
  return page.getByPlaceholder(placeholder, { exact: true });
}

async function gotoWorkbench(page: Page, baseURL: string) {
  await page.goto(baseURL);
  await expect(page.getByRole('heading', { name: 'SatPulse', exact: true })).toBeVisible();
  await expect(page.getByText('Connected', { exact: true })).toBeVisible();
}

// openConfig opens the Configuration tab (enabled once the receiver is
// identified) and waits for the automatic readback to populate and re-enable
// the panel.
async function openConfig(page: Page) {
  const tab = page.getByRole('button', { name: 'Configuration' });
  await expect(tab).toBeEnabled({ timeout: 30_000 });
  await tab.click();
  const period = ph(page, PERIOD);
  await expect(period).toBeEnabled({ timeout: 30_000 });
  await expect(period).not.toHaveValue('', { timeout: 30_000 });
}

// reloadAndOpenConfig reloads the page (re-checking the stored token) and
// reopens the Config tab, forcing a fresh automatic readback -- the only way
// to re-read within a connection.
async function reloadAndOpenConfig(page: Page) {
  await page.reload();
  await expect(page.getByText('Connected', { exact: true })).toBeVisible({ timeout: 30_000 });
  await openConfig(page);
}

async function applyAndWait(page: Page) {
  await applyButton(page).click();
  await expect(page.getByText('Configuration applied')).toBeVisible({ timeout: 30_000 });
}

function applyButton(page: Page): Locator {
  return page.getByRole('button', { name: 'Apply', exact: true });
}

// fieldAfter returns the form control immediately following a label whose exact
// text is given -- for the ECEF/LLH/survey fields, which have no placeholder
// and no associated label element.
function inputAfter(page: Page, labelText: string): Locator {
  return page.getByText(labelText, { exact: true }).locator('xpath=following-sibling::input[1]');
}
function selectAfter(page: Page, labelText: string): Locator {
  return page.getByText(labelText, { exact: true }).locator('xpath=following-sibling::select[1]');
}

// --- panel-wide behaviour ---------------------------------------------------

test('panel-wide: the automatic readback populates the panel; disconnect disables it', async ({ page, workbenchUbxsim }) => {
  await gotoWorkbench(page, workbenchUbxsim.baseURL);
  await openConfig(page);

  // The readback populated the panel and re-enabled its controls.
  await expect(ph(page, PERIOD)).toHaveValue('1');
  await expect(ph(page, PERIOD)).toBeEnabled();

  // Disconnecting the receiver disables every control (config-panel folds the
  // disconnected state into `connected`), and clears the read-back value.
  await page.getByRole('button', { name: 'Disconnect' }).click();
  await expect(ph(page, PERIOD)).toBeDisabled();
  await expect(ph(page, PERIOD)).toHaveValue('');
});

test('panel-wide: pending label names exactly the touched sections; Apply gates on pending; Discard restores', async ({ page, workbenchUbxsim }) => {
  await gotoWorkbench(page, workbenchUbxsim.baseURL);
  await openConfig(page);

  // Nothing pending after readback: Apply disabled, no status label.
  await expect(applyButton(page)).toBeDisabled();
  await expect(page.getByText(/Changes pending/)).toHaveCount(0);

  // Touch time pulse (period) and messages (the first group's Change box): the
  // pending label names exactly those two sections in order.
  await ph(page, PERIOD).fill('2');
  await page.getByRole('checkbox', { name: 'Change' }).first().check();
  await expect(page.getByText('Changes pending to time pulse, messages', { exact: true })).toBeVisible();
  await expect(applyButton(page)).toBeEnabled();

  // Discard restores the readback values and clears the pending state.
  await page.getByRole('button', { name: 'Discard' }).click();
  await expect(ph(page, PERIOD)).toHaveValue('1');
  await expect(page.getByRole('checkbox', { name: 'Change' }).first()).not.toBeChecked();
  await expect(page.getByText(/Changes pending/)).toHaveCount(0);
  await expect(applyButton(page)).toBeDisabled();
});

// --- time pulse -------------------------------------------------------------

test('time pulse: readback populates the group and editing tracks it dirty', async ({ page, workbenchUbxsim }) => {
  await gotoWorkbench(page, workbenchUbxsim.baseURL);
  await openConfig(page);

  await expect(ph(page, PERIOD)).toHaveValue('1');
  await expect(ph(page, WIDTH)).toHaveValue('0.1');
  await expect(ph(page, CABLE)).toHaveValue('50');
  await expect(selectAfter(page, 'Time GNSS')).toHaveValue('');
  await expect(page.getByRole('checkbox', { name: 'Align to GNSS' })).toBeChecked();
  await expect(page.getByRole('checkbox', { name: 'Only when locked' })).toBeChecked();
  await expect(page.getByRole('checkbox', { name: 'Rising edge' })).toBeChecked();

  // Editing any time-pulse field adds "time pulse" to the pending label. The
  // cable-delay edit is recorded in the plan as marking the whole section
  // touched -- expected, not a bug.
  await ph(page, CABLE).fill('75');
  await expect(page.getByText('Changes pending to time pulse', { exact: true })).toBeVisible();
});

test('time pulse: client-side validation disables Apply until corrected', async ({ page, workbenchUbxsim }) => {
  await gotoWorkbench(page, workbenchUbxsim.baseURL);
  await openConfig(page);

  // A non-numeric cable delay is invalid and disables Apply.
  await ph(page, CABLE).fill('abc');
  await expect(applyButton(page)).toBeDisabled();
  await ph(page, CABLE).fill('50');
  await expect(applyButton(page)).toBeEnabled();

  // A negative period is invalid.
  await ph(page, PERIOD).fill('-1');
  await expect(applyButton(page)).toBeDisabled();
  await ph(page, PERIOD).fill('1');
  await expect(applyButton(page)).toBeEnabled();

  // A pulse width above 1 s is invalid, as is a width at least as long as the
  // period; correcting re-enables Apply.
  await ph(page, WIDTH).fill('2');
  await expect(applyButton(page)).toBeDisabled();
  await ph(page, WIDTH).fill('1');
  await expect(applyButton(page)).toBeDisabled();
  await ph(page, WIDTH).fill('0.5');
  await expect(applyButton(page)).toBeEnabled();
});

test('time pulse: apply round-trips through a reload readback', async ({ page, workbenchUbxsim }) => {
  test.slow();
  await gotoWorkbench(page, workbenchUbxsim.baseURL);
  await openConfig(page);

  await ph(page, PERIOD).fill('2');
  await ph(page, WIDTH).fill('0.2');
  await ph(page, CABLE).fill('100');
  await applyAndWait(page);

  await reloadAndOpenConfig(page);
  await expect(ph(page, PERIOD)).toHaveValue('2');
  await expect(ph(page, WIDTH)).toHaveValue('0.2');
  await expect(ph(page, CABLE)).toHaveValue('100');
});

// --- time mode --------------------------------------------------------------

test('time mode: readback selects the mode and the subgroups enable with the radio', async ({ page, workbenchUbxsim }) => {
  await gotoWorkbench(page, workbenchUbxsim.baseURL);
  await openConfig(page);

  // The personality is mobile.
  await expect(page.getByRole('radio', { name: 'Mobile' })).toBeChecked();

  // Selecting survey-in enables the survey subgroup, leaves fixed disabled.
  await page.getByRole('radio', { name: 'Survey-in' }).check();
  await expect(page.getByPlaceholder('2000')).toBeEnabled();
  await expect(inputAfter(page, 'X (m)')).toBeDisabled();

  // Selecting fixed position does the reverse.
  await page.getByRole('radio', { name: 'Fixed position' }).check();
  await expect(inputAfter(page, 'X (m)')).toBeEnabled();
  await expect(page.getByPlaceholder('2000')).toBeDisabled();
});

test('time mode: validation flags bad coordinates and survey parameters', async ({ page, workbenchUbxsim }) => {
  await gotoWorkbench(page, workbenchUbxsim.baseURL);
  await openConfig(page);

  // Fixed ECEF with empty coordinates is invalid.
  await page.getByRole('radio', { name: 'Fixed position' }).check();
  await expect(applyButton(page)).toBeDisabled();

  // Lat/Lon out of range is invalid.
  await page.getByRole('radio', { name: 'Lat/Lon/Height' }).check();
  await inputAfter(page, 'Latitude (deg)').fill('95');
  await inputAfter(page, 'Longitude (deg)').fill('0');
  await inputAfter(page, 'Height (m)').fill('0');
  await expect(applyButton(page)).toBeDisabled();
  await inputAfter(page, 'Latitude (deg)').fill('45');
  await expect(applyButton(page)).toBeEnabled();
  await inputAfter(page, 'Longitude (deg)').fill('200');
  await expect(applyButton(page)).toBeDisabled();
  await inputAfter(page, 'Longitude (deg)').fill('100');
  await expect(applyButton(page)).toBeEnabled();

  // Survey time of zero and a sub-millimetre accuracy are invalid.
  await page.getByRole('radio', { name: 'Survey-in' }).check();
  await page.getByPlaceholder('2000').fill('0');
  await expect(applyButton(page)).toBeDisabled();
  await page.getByPlaceholder('2000').fill('2000');
  await expect(applyButton(page)).toBeEnabled();
  await inputAfter(page, 'Survey accuracy (m)').fill('0.0001');
  await expect(applyButton(page)).toBeDisabled();
  await inputAfter(page, 'Survey accuracy (m)').fill('20');
  await expect(applyButton(page)).toBeEnabled();
});

test('time mode: the ECEF on-Earth indicator marks coordinates off the surface', async ({ page, workbenchUbxsim }) => {
  await gotoWorkbench(page, workbenchUbxsim.baseURL);
  await openConfig(page);

  await page.getByRole('radio', { name: 'Fixed position' }).check();
  const offEarth = page.locator('[title="ECEF coordinates not on Earth"]');

  // A point on the equatorial surface is on Earth: no marker, Apply enabled.
  await inputAfter(page, 'X (m)').fill('6378137');
  await inputAfter(page, 'Y (m)').fill('0');
  await inputAfter(page, 'Z (m)').fill('0');
  await expect(offEarth).toHaveCount(0);
  await expect(applyButton(page)).toBeEnabled();

  // The geocentre is far from the surface: the server's check-on-earth
  // endpoint reports not-on-Earth, so the marker shows and Apply disables.
  await inputAfter(page, 'X (m)').fill('0');
  await expect(offEarth.first()).toBeVisible();
  await expect(applyButton(page)).toBeDisabled();
});

test('time mode: apply a fixed ECEF position, round-trip it, and return to mobile', async ({ page, workbenchUbxsim }) => {
  test.slow();
  await gotoWorkbench(page, workbenchUbxsim.baseURL);
  await openConfig(page);

  await page.getByRole('radio', { name: 'Fixed position' }).check();
  await inputAfter(page, 'X (m)').fill('6378137');
  await inputAfter(page, 'Y (m)').fill('0');
  await inputAfter(page, 'Z (m)').fill('0');
  await applyAndWait(page);

  await reloadAndOpenConfig(page);
  await expect(page.getByRole('radio', { name: 'Fixed position' })).toBeChecked();
  await expect(inputAfter(page, 'X (m)')).toHaveValue('6378137');
  await expect(inputAfter(page, 'Y (m)')).toHaveValue('0');
  await expect(inputAfter(page, 'Z (m)')).toHaveValue('0');

  // A second apply restores the receiver to mobile.
  await page.getByRole('radio', { name: 'Mobile' }).check();
  await applyAndWait(page);
  await reloadAndOpenConfig(page);
  await expect(page.getByRole('radio', { name: 'Mobile' })).toBeChecked();
});

// --- satellites and signals -------------------------------------------------

test('satellites: readback checks the enabled constellations', async ({ page, workbenchUbxsim }) => {
  await gotoWorkbench(page, workbenchUbxsim.baseURL);
  await openConfig(page);

  for (const gnss of ['BDS', 'GAL', 'GLO', 'GPS', 'QZSS', 'SBAS']) {
    await expect(page.getByRole('checkbox', { name: gnss })).toBeChecked();
  }
  await expect(ph(page, MINELEV)).toHaveValue('10');
});

test('satellites: the signal picker updates the selection on OK but not on Cancel', async ({ page, workbenchUbxsim }) => {
  await gotoWorkbench(page, workbenchUbxsim.baseURL);
  await openConfig(page);

  // The personality enables a subset of each constellation's signals, so every
  // picker group box reads indeterminate. check() then uncheck() first selects
  // all of SBAS, then clears it, leaving the SBAS constellation empty.
  const clearSbasInPicker = async () => {
    await page.getByRole('button', { name: 'Edit signals...' }).click();
    const picker = page.getByRole('heading', { name: 'Select GNSS signals' }).locator('xpath=ancestor::div[2]');
    const sbas = picker.getByRole('checkbox', { name: 'SBAS' });
    await sbas.check();
    await sbas.uncheck();
  };

  // Cancel: clear SBAS in the picker, then Cancel -- no change, nothing pending.
  await clearSbasInPicker();
  await page.getByRole('button', { name: 'Cancel' }).click();
  await expect(page.getByRole('checkbox', { name: 'SBAS' })).toBeChecked();
  await expect(page.getByText(/Changes pending/)).toHaveCount(0);

  // OK: the same edit, confirmed, clears the SBAS constellation box and marks
  // the section pending.
  await clearSbasInPicker();
  await page.getByRole('button', { name: 'OK' }).click();
  await expect(page.getByRole('checkbox', { name: 'SBAS' })).not.toBeChecked();
  await expect(page.getByText('Changes pending to satellites and signals', { exact: true })).toBeVisible();
});

test('satellites: minimum elevation outside 0..90 is invalid', async ({ page, workbenchUbxsim }) => {
  await gotoWorkbench(page, workbenchUbxsim.baseURL);
  await openConfig(page);

  await ph(page, MINELEV).fill('95');
  await expect(applyButton(page)).toBeDisabled();
  await ph(page, MINELEV).fill('15');
  await expect(applyButton(page)).toBeEnabled();
});

test('satellites: apply round-trips the enabled signals and minimum elevation', async ({ page, workbenchUbxsim }) => {
  test.slow();
  await gotoWorkbench(page, workbenchUbxsim.baseURL);
  await openConfig(page);

  // Disable one constellation and raise the minimum elevation.
  await page.getByRole('checkbox', { name: 'SBAS' }).uncheck();
  await ph(page, MINELEV).fill('15');
  await applyAndWait(page);

  await reloadAndOpenConfig(page);
  await expect(page.getByRole('checkbox', { name: 'SBAS' })).not.toBeChecked();
  await expect(page.getByRole('checkbox', { name: 'GPS' })).toBeChecked();
  await expect(ph(page, MINELEV)).toHaveValue('15');
});

// --- messages ---------------------------------------------------------------

test('messages: the preset buttons set the group controls', async ({ page, workbenchUbxsim }) => {
  await gotoWorkbench(page, workbenchUbxsim.baseURL);
  await openConfig(page);

  // Minimum: NMEA on with just RMC, RTCM disabled.
  await page.getByRole('button', { name: 'Minimum' }).click();
  await expect(page.getByRole('checkbox', { name: 'RMC' })).toBeChecked();
  await expect(page.getByRole('checkbox', { name: 'GGA' })).not.toBeChecked();

  // Daemon: satellites messages on (speed 38400 >= 19200) and the PVT preset
  // set (time-pulse time and position among them).
  await page.getByRole('button', { name: 'Daemon' }).click();
  await expect(page.getByRole('checkbox', { name: 'Satellite positions' })).toBeChecked();
  await expect(page.getByRole('checkbox', { name: 'Time-pulse time' })).toBeChecked();
  await expect(page.getByRole('checkbox', { name: 'Position', exact: true })).toBeChecked();
});

test('messages: enabling the satellites messages makes satellite data appear', async ({ page, workbenchUbxsim }) => {
  test.slow();
  await gotoWorkbench(page, workbenchUbxsim.baseURL);

  // NAV-SAT is off in the personality defaults, so no satellite count shows.
  await expect(page.getByText(/Sats:/)).toHaveCount(0);

  await openConfig(page);
  // Enable the Satellites messages subgroup: its Change box, then satellite
  // positions.
  const satsSub = page.getByText('Satellites', { exact: true }).locator('xpath=following-sibling::div[1]');
  await satsSub.getByRole('checkbox', { name: 'Change' }).check();
  await satsSub.getByRole('checkbox', { name: 'Satellite positions' }).check();
  await applyAndWait(page);

  // Back on the Monitor the satellite count now appears as NAV-SAT flows.
  await page.getByRole('button', { name: 'Monitor' }).click();
  await expect(page.getByText(/Sats:/).first()).toBeVisible({ timeout: 30_000 });
});

// --- serial speed -----------------------------------------------------------

test('serial speed: the dropdown shows the readback baud rate', async ({ page, workbenchUbxsim }) => {
  await gotoWorkbench(page, workbenchUbxsim.baseURL);
  await openConfig(page);

  const baud = selectAfter(page, 'Baud rate');
  await expect(baud).toHaveValue('38400');
  await expect(baud).toBeEnabled();
});
