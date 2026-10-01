import {test, expect} from '@playwright/test';
import type {Page} from '@playwright/test';
import * as path from 'node:path';

// Vite serves this test-only entry point using the real Workbench components.
const fixture = 'http://127.0.0.1:4175/@fs/' + path.resolve(__dirname, '../../workbench/test/index.html');

async function chooseDevice(page: Page) {
    await page.getByRole('button', {name: 'Select port', exact: true}).click();
    await expect(page.locator('header ul li').last()).toHaveText('Add a device...');
    await page.getByRole('button', {name: 'Add a device...', exact: true}).click();
}

test.describe('optional transport capabilities', () => {
    test('omits corrections controls and subscriptions without the capability', async ({page}) => {
        await page.goto(fixture);
        await expect(page.getByRole('banner')).toBeVisible();
        await expect(page.getByRole('button', {name: 'Corrections', exact: true})).toHaveCount(0);
        await expect(page.getByPlaceholder('e.g. 10.0.0.1')).toHaveCount(0);
        expect(await page.evaluate(() => window.workbenchTest.subscriptions())).not.toContain('gps:corrpacket');
    });

    test('mounts corrections controls and synchronizes through the capability', async ({page}) => {
        await page.goto(fixture + '?corrections');
        await page.getByRole('button', {name: 'Corrections', exact: true}).click();
        await expect(page.getByPlaceholder('e.g. 10.0.0.1')).toBeVisible();
        await expect.poll(() => page.evaluate(() => window.workbenchTest.calls.filter(c => c.method === 'getCorrectionsState').length)).toBe(1);
        expect(await page.evaluate(() => window.workbenchTest.subscriptions())).toContain('gps:corrpacket');
    });

    test('starts and stops through the supplied corrections capability', async ({page}) => {
        await page.goto(fixture + '?corrections');
        await page.getByRole('button', {name: 'Corrections', exact: true}).click();
        const panel = page.locator('div.flex.h-full.flex-col').filter({has: page.locator('span:text-is("Mountpoint:")')});
        await page.getByPlaceholder('e.g. 10.0.0.1').fill('caster.test');
        await panel.locator('span:text-is("Mountpoint:") + input').fill('TEST');
        await panel.getByRole('button', {name: 'Connect', exact: true}).click();
        await expect(panel.getByRole('button', {name: 'Disconnect', exact: true})).toBeVisible();
        expect(await page.evaluate(() => window.workbenchTest.calls.find(c => c.method === 'startCorrections')?.args)).toEqual([{
            mode: 'ntrip', host: 'caster.test', port: 2101, mountpoint: 'TEST',
            username: '', password: '', nmeaSend: false,
        }]);
        await panel.getByRole('button', {name: 'Disconnect', exact: true}).click();
        await expect(panel.getByRole('button', {name: 'Connect', exact: true})).toBeEnabled();
        expect(await page.evaluate(() => window.workbenchTest.calls.filter(c => c.method === 'stopCorrections').length)).toBe(1);
    });

    test('adds the chosen device, refreshes choices and uses it for Connect', async ({page}) => {
        await page.goto(fixture + '?picker=select');
        await chooseDevice(page);
        const device = page.locator('header input').first();
        await expect(device).toHaveValue('serial:1');
        await expect(device).toHaveJSProperty('readOnly', true);
        await expect(page.getByRole('button', {name: 'Add a device...', exact: true})).toHaveCount(0);
        expect(await page.evaluate(() => window.workbenchTest.calls.filter(c => c.method === 'choosePort').length)).toBe(1);
        await page.getByRole('banner').getByRole('button', {name: 'Connect', exact: true}).click();
        expect(await page.evaluate(() => window.workbenchTest.calls.find(c => c.method === 'connect')?.args)).toEqual(['serial:1', 38400]);
        await page.getByRole('button', {name: 'Select port', exact: true}).click();
        await expect(page.locator('header ul')).toContainText('Receiver');
    });

    test('picker cancellation preserves the selected device', async ({page}) => {
        await page.goto(fixture + '?picker=cancel&device=serial:1');
        await expect(page.locator('header input').first()).toHaveValue('serial:1');
        await chooseDevice(page);
        expect(await page.evaluate(() => window.workbenchTest.calls.filter(c => c.method === 'choosePort').length)).toBe(1);
        await expect(page.locator('header input').first()).toHaveValue('serial:1');
        await expect(page.getByRole('banner').getByRole('button', {name: 'Connect', exact: true})).toBeEnabled();
    });

    test('picker errors are visible and preserve the selected device', async ({page}) => {
        await page.goto(fixture + '?picker=error&device=serial:1');
        await expect(page.locator('header input').first()).toHaveValue('serial:1');
        await chooseDevice(page);
        await expect(page.getByText('Permission denied', {exact: true})).toBeVisible();
        await expect(page.locator('header input').first()).toHaveValue('serial:1');
    });

    test('without a picker the device path stays editable and there is no add action', async ({page}) => {
        await page.goto(fixture + '?picker=none&device=/dev/ttyTEST');
        const device = page.locator('header input').first();
        await expect(device).toHaveValue('/dev/ttyTEST');
        await expect(device).toHaveJSProperty('readOnly', false);
        await expect(device).toHaveAttribute('placeholder', 'device path');
        await page.getByRole('button', {name: 'Select port', exact: true}).click();
        await expect(page.getByRole('button', {name: 'Add a device...', exact: true})).toHaveCount(0);
    });
});
