import { test, expect } from '../harness';

// The Message file tab's send path against the u-blox simulator
// (workbenchUbxsim: satpulsewb -d/-s over the sim's pty, already connected).
// This is the first browser-level check of the send machinery -- sendWorker,
// the response correlator, and the gps:msgsend/gps:response events -- which
// the wire-level smoketests do not reach.
//
// The tag under test is get-gnss from configs/gpsmsg/u-blox/ubx.toml: a UBX
// poll of MON-GNSS (class 0x0A id 0x28), which the simulator answers by
// replaying the personality's captured MON-GNSS packet. A binary response
// with a known message ID renders in the response pane as
// "<tag>-<msgID>" -- "UBX-MON-GNSS" (msgfile-panel.tsx formatResponseLine;
// the format tag is ubx.go's Tag "UBX") -- followed by the packet hex.

test('send: a loaded query tag round-trips to the simulator and its response renders', async ({ page, workbenchUbxsim }) => {
    await page.goto(workbenchUbxsim.baseURL);
    // Active detection over the simulator walks the state to Connected
    // (app.tsx connStateLabel in the status bar); Send is gated on it.
    await expect(page.getByText('Connected', { exact: true })).toBeVisible();

    // Load the u-blox library file through the catalog dropdowns
    // (msgfile-panel.tsx picker path); the loaded path confirms it.
    await page.getByRole('button', { name: 'Message file' }).click();
    await page.getByLabel('Vendor').filter({ visible: true }).selectOption('u-blox');
    await page.getByLabel('File').filter({ visible: true }).selectOption('ubx');
    await page.getByRole('button', { name: 'Load', exact: true }).click();
    await expect(page.getByText(/u-blox\/ubx\.toml/)).toBeVisible();

    // Clicking the tag row arms it, which enables Send.
    const send = page.getByRole('button', { name: 'Send', exact: true });
    await expect(send).toBeDisabled();
    await page.getByRole('cell', { name: 'get-gnss', exact: true }).click();
    await expect(send).toBeEnabled();
    await send.click();

    // The simulator's MON-GNSS reply comes back through the correlator as a
    // gps:response event and lands in the response pane, labelled with the
    // sent message's identity.
    await expect(page.getByText('UBX-MON-GNSS', { exact: true })).toBeVisible();
});
