import { test, expect } from '../harness';
import type { Page } from '@playwright/test';

// The workbench Connection panel, driven by the u-blox simulator with
// satpulsewb launched WITHOUT -d (workbenchUbxsimNoDevice): the app starts
// disconnected and every connection is made through the panel itself. The
// simulator's pty symlink is not a real /dev node, so port enumeration never
// lists it; the free-text device input (placeholder "device path" in
// connection-panel.tsx) is how the tests reach it. Selectors are ARIA roles
// and visible text quoted from connection-panel.tsx and app.tsx.
//
// The fixture is test-scoped: every test gets a fresh simulator and server,
// so no state leaks between tests.

// The receiver identity app.tsx composes for an identified receiver:
// vendor + " " + hardware + " (FW " + firmware + ")". The simulator
// replays the u-blox F9P personality's MON-VER, whose extension strings
// carry MOD=ZED-F9P, FWVER=HPG 1.51 and PROTVER=27.50 (ubx/ubxcfg.go
// ReceiverInfo joins the firmware parts with " PROTVER ").
const IDENT = 'u-blox ZED-F9P (FW HPG 1.51 PROTVER 27.50)';

// The connection bar is the app's <header>, which maps to the banner role;
// scoping to it keeps the panel's controls distinct from the tab panels
// below (which stay mounted, merely display:none, when inactive).
function banner(page: Page) {
    return page.getByRole('banner');
}

// statusBar returns the connection-state label at the bottom of the app
// (app.tsx connStateLabel). exact:true distinguishes "Connected" from
// "Disconnected" and "Connecting...".
function statusBar(page: Page, label: string) {
    return page.getByText(label, { exact: true });
}

function deviceInput(page: Page) {
    return page.getByPlaceholder('device path');
}

// The speed dropdown is the only combobox in the connection bar.
function speedSelect(page: Page) {
    return banner(page).getByRole('combobox');
}

// fillDevice types a device path and confirms it stuck: on mount app.tsx
// enumerates ports and auto-fills the device input when the machine has
// exactly one, so a fill racing that late setDevice could be overwritten.
async function fillDevice(page: Page, device: string) {
    await expect(async () => {
        await deviceInput(page).fill(device);
        await expect(deviceInput(page)).toHaveValue(device);
    }).toPass();
}

// connect drives the panel from disconnected to Connected against the
// simulator: type the pty symlink path, pick the personality's baud, click
// Connect. The state walk (connecting, then active detection with the
// probe answered by the simulator) takes a few seconds; the expect waits
// cover it.
async function connect(page: Page, device: string) {
    await expect(statusBar(page, 'Disconnected')).toBeVisible();
    await fillDevice(page, device);
    await speedSelect(page).selectOption('38400');
    await banner(page).getByRole('button', { name: 'Connect', exact: true }).click();
    await expect(statusBar(page, 'Connected')).toBeVisible();
}

test('startup device and speed populate the same connection controls used by the UI', async ({ page, workbenchUbxsim }) => {
    await page.goto(workbenchUbxsim.baseURL);

    await expect(statusBar(page, 'Connected')).toBeVisible();
    await expect(deviceInput(page)).toHaveValue(workbenchUbxsim.devicePath);
    await expect(speedSelect(page)).toHaveValue('38400');
});

test('a startup device without a speed populates the control without auto-connecting', async ({ page, workbenchUbxsimDeviceOnly }) => {
    await page.goto(workbenchUbxsimDeviceOnly.baseURL);

    await expect(statusBar(page, 'Disconnected')).toBeVisible();
    await expect(deviceInput(page)).toHaveValue(workbenchUbxsimDeviceOnly.devicePath);
    await expect(speedSelect(page)).toHaveValue('9600');
});

test('connect: a typed device path and speed walk the state to Connected and identify the receiver', async ({ page, workbenchUbxsimNoDevice }) => {
    await page.goto(workbenchUbxsimNoDevice.baseURL);
    await connect(page, workbenchUbxsimNoDevice.devicePath);

    // Active detection: the simulator answered the MON-VER probe, so the
    // identity span appears in the connection bar with the personality's
    // model and firmware.
    await expect(banner(page).getByText(IDENT)).toBeVisible();

    // While connected the connection inputs lock and the button flips to
    // Disconnect (connection-panel.tsx disables them on connected).
    await expect(deviceInput(page)).toBeDisabled();
    await expect(speedSelect(page)).toBeDisabled();
    await expect(banner(page).getByRole('button', { name: 'Disconnect' })).toBeVisible();
});

test('disconnect: returns to Disconnected and the dependent controls disable', async ({ page, workbenchUbxsimNoDevice }) => {
    await page.goto(workbenchUbxsimNoDevice.baseURL);
    await connect(page, workbenchUbxsimNoDevice.devicePath);

    // Positive control while connected: the Configuration tab is enabled
    // (receiver identified), so clicking it shows the config panel. Opening
    // it kicks off the automatic readback; wait for the state to settle back
    // to Connected before disconnecting under it.
    await expect(banner(page).getByText(IDENT)).toBeVisible();
    await page.getByRole('button', { name: 'Configuration' }).click();
    await expect(page.getByRole('button', { name: 'Time pulse' })).toBeVisible();
    await expect(statusBar(page, 'Connected')).toBeVisible();
    await page.getByRole('button', { name: 'Monitor' }).click();

    await banner(page).getByRole('button', { name: 'Disconnect' }).click();
    await expect(statusBar(page, 'Disconnected')).toBeVisible();

    // Disconnection clears the receiver state (app.tsx gps:state handler):
    // the identity leaves the connection bar, the inputs unlock, and the
    // Configuration tab disables (configDisabled), so clicking it no longer
    // switches tabs -- the config panel stays hidden and Monitor stays up.
    await expect(banner(page).getByText(IDENT)).toHaveCount(0);
    await expect(deviceInput(page)).toBeEnabled();
    await expect(speedSelect(page)).toBeEnabled();
    await page.getByRole('button', { name: 'Configuration' }).click();
    await expect(page.getByRole('button', { name: 'Time pulse' })).not.toBeVisible();
    await expect(page.getByRole('button', { name: 'PVT Messages' })).toBeVisible();
});

test('connecting to a nonexistent device path surfaces the failure', async ({ page, workbenchUbxsimNoDevice }) => {
    await page.goto(workbenchUbxsimNoDevice.baseURL);
    await expect(statusBar(page, 'Disconnected')).toBeVisible();
    await fillDevice(page, '/nonexistent/gps-device');
    await banner(page).getByRole('button', { name: 'Connect', exact: true }).click();

    // SerialOpener retries a missing node for ~2s (session.go
    // connectOpenTimeout) before failing; the server's error message lands
    // in the toast app.tsx handleConnect raises, and the state falls back
    // to Disconnected.
    await expect(page.getByText(/no such file or directory/)).toBeVisible();
    await expect(statusBar(page, 'Disconnected')).toBeVisible();
});

test('the port dropdown renders its enumerated ports or the empty state', async ({ page, workbenchUbxsimNoDevice }) => {
    await page.goto(workbenchUbxsimNoDevice.baseURL);
    await expect(statusBar(page, 'Disconnected')).toBeVisible();

    // The dropdown toggle is the button labelled "Select port"; opening it
    // refreshes the enumeration and renders the list. This machine may or
    // may not have real serial ports, and the pty symlink is never
    // enumerated, so accept either outcome: connection-panel.tsx renders
    // real ports as list items and an empty enumeration as a single
    // "No ports found" item, so the list always has at least one item.
    const toggle = banner(page).getByRole('button', { name: 'Select port' });
    await expect(toggle).toBeVisible();
    await toggle.click();
    const list = banner(page).getByRole('list');
    await expect(list).toBeVisible();
    await expect(list.getByRole('listitem').first()).toBeVisible();
});
