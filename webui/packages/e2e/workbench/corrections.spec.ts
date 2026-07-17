import {test, expect} from '../harness';
import type {Page} from '@playwright/test';

// The workbench Corrections tab, driven by the workbenchCaster fixture:
// satpulsewb over the ubxsim pty plus the smoketest's fakesource.py Ntrip
// caster streaming base-arp.jsonl -- one MSM7 per constellation (1077 GPS,
// 1087 GLONASS, 1097 Galileo, 1127 BeiDou), 1230 biases and 1005 station
// coordinates each second, all from station 0. The pty matters: a correction
// session writes every pulled packet back to the receiver port, and the
// simulator consumes those writes, so the session stays connected; the
// caster then holds the connection open after the log drains, so "connected"
// is stable for as long as a test cares to look. Selectors are grounded in
// workbench/src/corrections-panel.tsx and cor-msg-panel.tsx.
//
// The fixture is worker-scoped and its simulator paces nav in real time, so
// nothing here depends on GPS nav data -- only the receiver connection state
// and the corrections stream, both of which the caster and simulator sustain
// indefinitely. The deeper wire checks (RTCM bytes on the port, VRS refusal,
// GGA upload) stay in the smoketest's stream/wb-corrections.

// corrPanel returns the CorrectionsPanel root (corrections-panel.tsx renders
// it as a flex h-full flex-col div). "Mountpoint:" is unique to this panel,
// and scoping matters: the connection bar in the header has its own
// Connect/Disconnect button.
function corrPanel(page: Page) {
    return page.locator('div.flex.h-full.flex-col').filter({has: page.locator('span:text-is("Mountpoint:")')});
}

// input returns the field next to a label span; the panel lays out each field
// as a span followed by its input (corrections-panel.tsx).
function input(page: Page, label: string) {
    return corrPanel(page).locator(`span:text-is("${label}") + input`);
}

// connectedDot matches the state dot in its connected colour (bg-success,
// corrections-panel.tsx dotClass). The dot is the only rendering of the
// connected state -- there is no text for it -- and nothing else in the panel
// uses bg-success (the message badges are rtcm/spartn toned).
function connectedDot(page: Page) {
    return corrPanel(page).locator('.bg-success');
}

// msgRow returns the message-panel row for an RTCM type (cor-msg-panel.tsx,
// one row per message type).
function msgRow(page: Page, msgType: string) {
    return corrPanel(page).getByRole('row').filter({has: page.getByRole('cell', {name: msgType, exact: true})});
}

// openCorrections waits for the receiver connection (the panel's fields stay
// disabled until connState leaves disconnected) and opens the tab.
async function openCorrections(page: Page, baseURL: string) {
    await page.goto(baseURL);
    await expect(page.getByText('Connected', {exact: true})).toBeVisible();
    await page.getByRole('button', {name: 'Corrections'}).click();
}

test('Ntrip: connect streams the caster\'s RTCM into the message panel; stop returns to idle', async ({page, workbenchCaster}) => {
    const {caster} = workbenchCaster;
    await openCorrections(page, workbenchCaster.baseURL);
    const panel = corrPanel(page);

    // NTRIP is the default mode; fill the source form with the fixture
    // caster's address and the credentials it requires.
    await input(page, 'Host:').fill(caster.host);
    await input(page, 'Port:').fill(String(caster.port));
    await input(page, 'Mountpoint:').fill(caster.mountpoint);
    await input(page, 'User:').fill(caster.username);
    await input(page, 'Password:').fill(caster.password);
    await panel.getByRole('button', {name: 'Connect'}).click();

    // The corrections state reaches connected, and the running session
    // relabels the toggle and locks the source form.
    await expect(connectedDot(page)).toHaveCount(1);
    await expect(panel.getByRole('button', {name: 'Disconnect'})).toBeVisible();
    await expect(input(page, 'Host:')).toBeDisabled();

    // The message panel populates with the caster's RTCM: each type in
    // base-arp.jsonl gets a row keyed by its type number, with the MSM label
    // and description from rtcm.ts and the log's station ID 0.
    const gps = msgRow(page, '1077');
    await expect(gps).toBeVisible();
    await expect(gps.getByRole('cell', {name: 'MSM7', exact: true})).toBeVisible();
    await expect(gps.getByRole('cell', {name: 'Full GPS Pseudoranges, PhaseRanges, PhaseRangeRate and CNR (high resolution)'})).toBeVisible();
    await expect(gps.getByRole('cell', {name: '0', exact: true})).toBeVisible();
    for (const t of ['1087', '1097', '1127', '1230']) {
        await expect(msgRow(page, t)).toBeVisible();
    }
    const arp = msgRow(page, '1005');
    await expect(arp.getByRole('cell', {name: 'Stationary Antenna Reference Point, No Height Information'})).toBeVisible();

    // Still connected after all those packets round-tripped to the receiver:
    // the state held while the panel populated.
    await expect(connectedDot(page)).toHaveCount(1);

    // Stop returns to idle: Connect on offer, fields editable again, the
    // connected dot gone. The received rows persist (the panel clears only
    // on a new session or receiver disconnect).
    await panel.getByRole('button', {name: 'Disconnect'}).click();
    await expect(panel.getByRole('button', {name: 'Connect'})).toBeEnabled();
    await expect(input(page, 'Host:')).toBeEnabled();
    await expect(connectedDot(page)).toHaveCount(0);
    await expect(gps).toBeVisible();
});

test('TCP mode disables the Ntrip-only fields', async ({page, workbenchCaster}) => {
    // Pure frontend form logic (corrections-panel.tsx ntripDisabled and
    // readPort): no session is started.
    await openCorrections(page, workbenchCaster.baseURL);
    const panel = corrPanel(page);

    await panel.getByRole('combobox').selectOption('tcp');
    await expect(input(page, 'Mountpoint:')).toBeDisabled();
    await expect(input(page, 'User:')).toBeDisabled();
    await expect(input(page, 'Password:')).toBeDisabled();

    // Back to NTRIP: the fields re-enable and the port field returns to the
    // NTRIP default.
    await panel.getByRole('combobox').selectOption('ntrip');
    await expect(input(page, 'Mountpoint:')).toBeEnabled();
    await expect(input(page, 'Port:')).toHaveValue('2101');
});
