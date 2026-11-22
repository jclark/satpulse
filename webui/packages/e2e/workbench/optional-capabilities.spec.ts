import {test, expect} from '../harness';
import type {Page} from '@playwright/test';

async function chooseDevice(page: Page) {
    await page.getByRole('button', {name: 'Select port', exact: true}).click();
    await expect(page.locator('header ul li').last()).toHaveText('Add a device...');
    await page.getByRole('button', {name: 'Add a device...', exact: true}).click();
}

test.describe('optional transport capabilities', () => {
    test('omits corrections controls and subscriptions without the capability', async ({page, mockWorkbench}) => {
        await page.goto(mockWorkbench.url());
        await expect(page.getByRole('banner')).toBeVisible();
        await expect(page.getByRole('button', {name: 'Corrections', exact: true})).toHaveCount(0);
        await expect(page.getByPlaceholder('e.g. 10.0.0.1')).toHaveCount(0);
        // gps:state is registered by the same effect, so once it is present the
        // correction subscriptions would be too.
        await expect.poll(() => page.evaluate(() => window.workbenchTest.subscriptions())).toContain('gps:state');
        const subs = await page.evaluate(() => window.workbenchTest.subscriptions());
        for (const name of ['gps:corrpacket', 'gps:corrections', 'gps:basearp']) expect(subs).not.toContain(name);
    });

    test('mounts corrections controls and synchronizes through the capability', async ({page, mockWorkbench}) => {
        // The snapshot reports a running stream that no event announces, as
        // for a window opened after corrections started.
        await page.goto(mockWorkbench.url('?corrections=connected'));
        await page.getByRole('button', {name: 'Corrections', exact: true}).click();
        const panel = page.locator('div.flex.h-full.flex-col').filter({has: page.locator('span:text-is("Mountpoint:")')});
        await expect(panel.getByRole('button', {name: 'Disconnect', exact: true})).toBeVisible();
        expect(await page.evaluate(() => window.workbenchTest.calls.filter(c => c.method === 'getCorrectionsState').length)).toBe(1);
        const subs = await page.evaluate(() => window.workbenchTest.subscriptions());
        for (const name of ['gps:corrpacket', 'gps:corrections', 'gps:basearp']) expect(subs).toContain(name);
    });

    test('starts and stops through the supplied corrections capability', async ({page, mockWorkbench}) => {
        await page.goto(mockWorkbench.url('?corrections'));
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

    test('adds the chosen device, refreshes choices and uses it for Connect', async ({page, mockWorkbench}) => {
        await page.goto(mockWorkbench.url('?picker=select'));
        await chooseDevice(page);
        const device = page.locator('header input').first();
        await expect(device).toHaveValue('serial:1');
        await expect(device).toHaveJSProperty('readOnly', true);
        await expect(page.getByRole('button', {name: 'Add a device...', exact: true})).toHaveCount(0);
        expect(await page.evaluate(() => window.workbenchTest.calls.filter(c => c.method === 'choosePort').length)).toBe(1);
        // The refresh after choosing, not the one that reopening the list does.
        await expect.poll(() => page.evaluate(() => {
            const methods = window.workbenchTest.calls.map(c => c.method);
            return methods.indexOf('listPorts', methods.indexOf('choosePort')) >= 0;
        })).toBe(true);
        await expect(page.getByRole('button', {name: 'Select port', exact: true})).toBeFocused();
        await page.getByRole('banner').getByRole('button', {name: 'Connect', exact: true}).click();
        expect(await page.evaluate(() => window.workbenchTest.calls.find(c => c.method === 'connect')?.args)).toEqual(['serial:1', 38400]);
        await page.getByRole('button', {name: 'Select port', exact: true}).click();
        await expect(page.locator('header ul')).toContainText('Receiver');
    });

    test('picker cancellation preserves the selected device', async ({page, mockWorkbench}) => {
        await page.goto(mockWorkbench.url('?picker=cancel&device=serial:1'));
        await expect(page.locator('header input').first()).toHaveValue('serial:1');
        await chooseDevice(page);
        expect(await page.evaluate(() => window.workbenchTest.calls.filter(c => c.method === 'choosePort').length)).toBe(1);
        await expect(page.locator('header input').first()).toHaveValue('serial:1');
        await expect(page.getByRole('banner').getByRole('button', {name: 'Connect', exact: true})).toBeEnabled();
    });

    test('picker errors are visible and preserve the selected device', async ({page, mockWorkbench}) => {
        await page.goto(mockWorkbench.url('?picker=error&device=serial:1'));
        await expect(page.locator('header input').first()).toHaveValue('serial:1');
        await chooseDevice(page);
        await expect(page.getByText('Permission denied', {exact: true})).toBeVisible();
        await expect(page.locator('header input').first()).toHaveValue('serial:1');
    });

    test('without a picker the device path stays editable and there is no add action', async ({page, mockWorkbench}) => {
        await page.goto(mockWorkbench.url('?picker=none&device=/dev/ttyTEST'));
        const device = page.locator('header input').first();
        await expect(device).toHaveValue('/dev/ttyTEST');
        await expect(device).toHaveJSProperty('readOnly', false);
        await expect(device).toHaveAttribute('placeholder', 'device path');
        await page.getByRole('button', {name: 'Select port', exact: true}).click();
        await expect(page.getByRole('button', {name: 'Add a device...', exact: true})).toHaveCount(0);
    });
});
