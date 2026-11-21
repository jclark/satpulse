import {test, expect} from '../harness';

test.describe('optional transport capabilities', () => {
    test('omits corrections controls and subscriptions without the capability', async ({page, mockWorkbench}) => {
        await page.goto(mockWorkbench.url());
        await expect(page.getByRole('banner')).toBeVisible();
        await expect(page.getByRole('button', {name: 'Corrections', exact: true})).toHaveCount(0);
        await expect(page.getByPlaceholder('e.g. 10.0.0.1')).toHaveCount(0);
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
});
