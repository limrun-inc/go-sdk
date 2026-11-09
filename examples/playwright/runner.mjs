// Playwright's Android API runs in JavaScript; provisioning is in main.go.
import { _android as android } from 'playwright';
const chunks = [];
for await (const chunk of process.stdin) chunks.push(chunk);
const input = JSON.parse(Buffer.concat(chunks).toString('utf8'));
const endpoint = new URL(input.url);
endpoint.searchParams.set('token', input.token);
const device = await android.connect(endpoint.toString());
try {
// This is needed for Chrome's first-run initializations to complete.
await device.shell('am start com.android.chrome/com.google.android.apps.chrome.Main');
await new Promise((resolve) => setTimeout(resolve, 1_000));
await device.shell('am force-stop com.android.chrome');
console.log('Chrome is ready');

const browser = await device.launchBrowser();
console.log('Browser launched');

console.time('cdp.commands');
const page = await browser.newPage();
await page.goto('https://github.com/microsoft/playwright');
await page.waitForURL('https://github.com/microsoft/playwright');
console.log(await page.title());
console.log('Page title logged');
// Wait for main content to be visible
await page.waitForSelector('[data-hpc]', { state: 'visible' });
const linksCount = await page.locator('a').count();
console.log(`Links on page: ${linksCount}`);

console.time('click.github');
await page.locator('a[title=".github"]').first().click();
console.timeEnd('click.github');
await page.locator('a[title="workflows"]').first().click();
await page.locator('a[title="infra.yml"]').first().click();
// Scroll
await page.evaluate(() => {
  window.scrollTo(0, document.body.scrollHeight);
});
console.time('cdp.screenshot');
await page.screenshot({ path: 'screenshot.png' });
console.timeEnd('cdp.screenshot');
console.timeEnd('cdp.commands');

} finally {
  await device.close();
}
