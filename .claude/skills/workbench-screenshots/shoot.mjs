// Screenshots of the workbench tabs for the website, taken against a running
// satpulsewb. Usage: node shoot.mjs <url> <outdir> <run> [step], where run is
// one of f9p, g5, allystar, casic. For allystar, step 'scatter' retakes only
// the position scatter shot, leaving the other two alone.
//
// The viewport and the helpers (shot, tab, visibleButton, section,
// packetsShot, configShot, connectCaster, scatterShot) are the stable part.
// The run branches at the end are worked examples from one round of
// screenshots: rewrite them for whatever receivers are connected.
//
// Playwright is resolved from webui/node_modules, so the script can run from
// anywhere; it is not part of the e2e suite.
import { createRequire } from 'node:module';
const { chromium } = createRequire(new URL('../../../webui/package.json', import.meta.url))('playwright');
const [url, outdir, run, step] = process.argv.slice(2);
const W = 1024, H = 820;
const browser = await chromium.launch();
const ctx = await browser.newContext({ viewport: { width: W, height: H }, deviceScaleFactor: 1 });
const page = await ctx.newPage();
const shot = (name) => page.screenshot({ path: `${outdir}/wb-${run}-${name}.png` });
const tab = (name) => page.getByRole('button', { name, exact: true }).click();
const visibleButton = (name) => page.getByRole('button', { name, exact: true }).filter({ visible: true }).first();
const section = (name) => visibleButton(name).click();
const settle = (ms) => page.waitForTimeout(ms);

async function packetsShot(rowName) {
  await tab('Packets');
  await visibleButton('Clear').click();
  await settle(5000);
  const row = page.getByRole('cell', { name: rowName, exact: true }).filter({ visible: true }).first();
  await row.click();
  await row.evaluate(el => { for (let p = el.parentElement; p; p = p.parentElement) if (p.scrollHeight > p.clientHeight) { p.scrollTop = 0; break; } });
  await settle(1000);
  await shot('packets');
}

let openSection = null;
async function configShot(name, file) {
  if (openSection) await section(openSection);
  await section(name);
  openSection = name;
  await visibleButton(name).evaluate(el => el.scrollIntoView({ block: 'start' }));
  await settle(500);
  await shot(file);
}

async function connectCaster(host, port, mountpoint, user, password) {
  await tab('Corrections');
  const input = (label) => page.locator(`span:text-is("${label}") + input`);
  // satpulsewb keeps the caster connection across browser sessions.
  if (await input('Host:').isDisabled()) {
    // The header's Disconnect for the serial device comes first; this is the caster's.
    await page.getByRole('button', { name: 'Disconnect', exact: true }).filter({ visible: true }).nth(1).click();
    await visibleButton('Connect').waitFor({ timeout: 30_000 });
  }
  await input('Host:').fill(host);
  await input('Port:').fill(port);
  await input('Mountpoint:').fill(mountpoint);
  if (user) await input('User:').fill(user);
  if (password) await input('Password:').fill(password);
  await page.getByText('Send position as NMEA').click();
  await visibleButton('Connect').click();
  await visibleButton('Disconnect').waitFor({ timeout: 30_000 });
  await settle(15000);
}

async function scatterShot(withTop) {
  await tab('Monitor');
  await page.getByText(/Fix: fixed/).first().waitFor({ timeout: 300_000 });
  await settle(30000);
  if (withTop) await shot('monitor-rtk');
  await section('Position Scatter');
  await section('PVT Messages');
  // The RTK solution wanders for some minutes after it first fixes, so let it
  // settle before clearing: the points kept are the ones worth plotting.
  await settle(240000);
  // The scatter panel's Clear precedes the log panel's in document order.
  await visibleButton('Clear').click();
  await settle(120000);
  await visibleButton('Position Scatter').evaluate(el => el.scrollIntoView({ block: 'start' }));
  await settle(1000);
  await shot('monitor-scrolled');
}

await page.goto(url);
await page.getByText(/Fix: /).first().waitFor({ timeout: 90_000 });
await settle(8000);

if (run === 'f9p') {
  await shot('monitor');
  await packetsShot('NAV-PVT');
  await tab('Configuration');
  await settle(15000);
  await shot('config-signals');
  openSection = 'Satellites and signals';
  await configShot('Time pulse', 'config-timepulse');
  await configShot('Time mode', 'config-timemode');
  await configShot('Messages', 'config-messages');
} else if (run === 'g5') {
  await tab('Message file');
  await page.getByLabel('Vendor').filter({ visible: true }).selectOption('septentrio');
  await page.getByLabel('File').filter({ visible: true }).selectOption('mosaic-g5');
  await visibleButton('Load').click();
  await page.getByText(/mosaic-g5\.toml/).waitFor();
  await settle(1000);
  await shot('message-file');
} else if (run === 'allystar') {
  await connectCaster('serpa.lan', '2101', 'BKK', '', '');
  if (step !== 'scatter') await shot('corrections');
  await scatterShot(step !== 'scatter');
} else if (run === 'casic') {
  await packetsShot('NAV-PV');
  await tab('Configuration');
  await settle(15000);
  await shot('config-signals');
  openSection = 'Satellites and signals';
  await configShot('Messages', 'config-messages');
}
await browser.close();
