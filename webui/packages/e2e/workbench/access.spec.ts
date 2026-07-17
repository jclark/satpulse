import {test, expect} from '../harness';
import type {Page} from '@playwright/test';

// The satpulsewb workbench: startup, the token-in-URL handshake, and the
// access-control paths (rejection, stale-token notice, seat takeover,
// reconnect). Selectors are ARIA roles and visible text quoted from the
// frontend sources (workbench-http/src/main.tsx, workbench/src/app.tsx,
// connection-panel.tsx), so a restyle does not break them.
//
// The workbenchReplay fixture is test-scoped: each test gets a fresh daemon
// with its own live ~6s replay (factor 5), so no test depends on file or test
// order. Within a test, a page opened after its replay drains still receives
// satpulsewb's per-name event cache (sink.go), which primes a late-joining
// client with the latest state event, time, and position (but not satellites
// or the nav epoch, which are high-rate and uncached).

// The stale-token notice rendered by main.tsx's AuthNotice, identified by its
// first paragraph.
const AUTH_NOTICE = 'The access token for this page is missing or no longer valid.';

// The read-only banner connection-panel.tsx shows when another window holds
// the write seat.
const READ_ONLY = 'Read-only: this workbench has been taken over by another window';

// origin strips any token query from a base URL, giving the bare origin a
// second tab or a token-less visitor would use.
function origin(port: number): string {
    return `http://127.0.0.1:${port}/`;
}

// appHeading is the "SatPulse" title in the connection bar -- present once the
// app itself (not the connecting/auth notice) has mounted. exact:true keeps it
// from matching the notice's "SatPulse Workbench" heading.
function appHeading(page: Page) {
    return page.getByRole('heading', {name: 'SatPulse', exact: true});
}

// statusBar returns the connection-state label at the bottom of the app
// (app.tsx connStateLabel). exact:true distinguishes "Connected" from
// "Disconnected" and "Reconnecting...".
function statusBar(page: Page, label: string) {
    return page.getByText(label, {exact: true});
}

test('startup: the tokened URL mounts the app, strips the token, connects, and data advances', async ({page, workbenchReplay}) => {
    await page.goto(workbenchReplay.baseURL);
    await expect(page).toHaveTitle('SatPulse Workbench');

    // The app mounts (not the connecting or auth notice), and main.tsx has
    // stripped the token from the URL bar -- the behaviour only a real browser
    // can verify. The token lands in the store instead.
    await expect(appHeading(page)).toBeVisible();
    await expect.poll(() => page.url()).not.toContain('t=');
    expect(await page.evaluate(() => localStorage.getItem('satpulsewb-token'))).toBe(workbenchReplay.token);

    // Passive detection over the read-only FIFO walks the connection state to
    // Connected (the status bar at the bottom).
    await expect(statusBar(page, 'Connected')).toBeVisible();

    // The replay feeds satellites, a nav fix and position: the summary panel
    // reports the satellite count, and the PVT panel auto-expands its Position
    // table on the first position message.
    await expect(page.getByText(/Sats:/)).toBeVisible();
    await expect(page.getByRole('heading', {name: 'Position', exact: true})).toBeVisible();

    // The stream advances: the live page keeps re-rendering as successive time,
    // position and satellite events arrive, so the rendered text changes.
    const app = page.locator('#app');
    const first = await app.innerText();
    await expect.poll(() => app.innerText(), {timeout: 10_000}).not.toBe(first);
});

test('seat takeover: a second tab claims the write seat and the first drops to read-only', async ({browser, workbenchReplay}) => {
    // One context so the two tabs share localStorage -- the mechanism by which
    // the token reaches the second tab.
    const ctx = await browser.newContext();
    try {
        const a = await ctx.newPage();
        await a.goto(workbenchReplay.baseURL);
        await expect(appHeading(a)).toBeVisible();
        await expect(statusBar(a, 'Connected')).toBeVisible();
        // The token is now in the shared store; the second tab reads it from
        // there rather than the URL.
        expect(await a.evaluate(() => localStorage.getItem('satpulsewb-token'))).toBe(workbenchReplay.token);
        await expect(a.getByText(READ_ONLY)).toHaveCount(0);

        // The second tab opens the bare origin, inherits the stored token, and
        // its transport claims the write seat on boot.
        const b = await ctx.newPage();
        await b.goto(origin(workbenchReplay.port));
        await expect(appHeading(b)).toBeVisible();

        // The replay has drained by now, so the second tab's consistent state
        // comes from the server's per-name event cache, not fresh events: the
        // state and clock are primed at once. The clock shows a real time
        // rather than its "Waiting for time" placeholder.
        await expect(statusBar(b, 'Connected')).toBeVisible();
        await expect(b.getByText('Waiting for time')).toHaveCount(0);

        // The claim makes the second tab the writer, so the first tab receives
        // the writer broadcast for a seat it does not hold and folds into the
        // read-only role, offering "Use here" to reclaim.
        await expect(a.getByText(READ_ONLY)).toBeVisible();
        await expect(a.getByRole('button', {name: 'Use here'})).toBeVisible();
        // The taker-over stays the writer: no read-only banner on it.
        await expect(b.getByText(READ_ONLY)).toHaveCount(0);
    } finally {
        await ctx.close();
    }
});

test('the bare origin without a token is rejected, not served the app', async ({browser, workbenchReplay}) => {
    // A fresh context so no stored token from another test supplies one.
    const ctx = await browser.newContext();
    try {
        const page = await ctx.newPage();
        await page.goto(origin(workbenchReplay.port));
        // The static HTML always serves, but the pre-mount seat claim gets a
        // 401, so boot() shows the notice instead of mounting the app.
        await expect(page.getByText(AUTH_NOTICE)).toBeVisible();
        await expect(appHeading(page)).toHaveCount(0);
    } finally {
        await ctx.close();
    }
});

test('a wrong token at page load fails pre-mount validation and shows the stale-token notice', async ({browser, workbenchReplay}) => {
    // A fresh context again, and a token that is not the server's.
    const ctx = await browser.newContext();
    try {
        const page = await ctx.newPage();
        await page.goto(origin(workbenchReplay.port) + '?t=not-the-real-token');
        await expect(page.getByText(AUTH_NOTICE)).toBeVisible();
        await expect(appHeading(page)).toHaveCount(0);
    } finally {
        await ctx.close();
    }
});

// Path (b) of the stale-token notice: an already-open page losing its token.
// wb:authlost fires only when the SSE EventSource closes terminally
// (http-transport.ts openStream), which needs the server to 401 the stream,
// which needs the token to change -- and the token changes only on a
// satpulsewb restart. The workbenchFixedToken fixture (-L <port> -T) restarts
// on the same port with a fresh token, so the page can still reach the
// server; its reconnecting EventSource is then 401ed and the notice replaces
// the app (main.tsx documents this as deliberate).
test('a live token invalidated by a server restart shows the notice via wb:authlost', async ({page, workbenchFixedToken}) => {
    await page.goto(workbenchFixedToken.baseURL);
    await expect(appHeading(page)).toBeVisible();

    await workbenchFixedToken.restart();

    await expect(page.getByText(AUTH_NOTICE)).toBeVisible();
    await expect(appHeading(page)).toHaveCount(0);
});

test('reconnect: after a server restart the page reconnects its SSE and re-primes', async ({page, workbenchFixed}) => {
    // The fixed-port fixture runs satpulsewb -L with no token and no device, so
    // there is no token to go stale and no GPS data. The app mounts and sits
    // disconnected.
    await page.goto(workbenchFixed.baseURL);
    await expect(appHeading(page)).toBeVisible();
    await expect(statusBar(page, 'Disconnected')).toBeVisible();
    // This tab holds the current write seat, so no read-only banner yet.
    await expect(page.getByText(READ_ONLY)).toHaveCount(0);

    await workbenchFixed.restart();

    // The browser's EventSource sees the drop as transient (readyState
    // CONNECTING, not CLOSED), so it reconnects to the restarted server rather
    // than firing wb:authlost -- the notice must not appear.
    //
    // The reconnect re-primes from the new server's cache, which seeds a fresh
    // writer broadcast at startup (server.go newServer). That seat is not the
    // one this tab claimed before the restart, so the reconnected page learns
    // its seat is stale and folds into read-only -- the observable proof that
    // the SSE reconnected and re-primed. The app shell survives throughout.
    await expect(page.getByText(READ_ONLY)).toBeVisible();
    await expect(page.getByText(AUTH_NOTICE)).toHaveCount(0);
    await expect(appHeading(page)).toBeVisible();
    await expect(statusBar(page, 'Disconnected')).toBeVisible();
});
