// WebdriverIO handles the test; main.go handles all Limrun SDK calls.
import { remote } from 'webdriverio';
const chunks = [];
for await (const chunk of process.stdin) chunks.push(chunk);
const input = JSON.parse(Buffer.concat(chunks).toString('utf8'));
const driver = await remote({
  capabilities: {
    platformName: 'iOS',
    browserName: 'safari',
    'appium:udid': input.udid,
    'appium:automationName': '@limrun/xcuitest',
    'appium:noReset': true, // Otherwise, deletes the app we installed.
    'appium:fullReset': false,
    'appium:webDriverAgentUrl': input.wdaUrl,
    'appium:wdaLocalPort': 443,
    'appium:useNewWDA': false,
    'appium:usePreinstalledWDA': true,
    'appium:skipLogCapture': true,
    // Safari's first launch on a fresh instance can take a while to expose
    // its webviews to the remote debugger.
    'appium:webviewConnectTimeout': 60_000,
    'appium:limInstanceApiUrl': input.apiUrl,
    'appium:limInstanceToken': input.token,
    'appium:wdaRequestHeaders': {
      Authorization: `Bearer ${input.token}`,
    },
  },
  hostname: '127.0.0.1',
  port: 4723,
  path: '/',
  protocol: 'http',
});
try {
console.log('Appium successfully connected to the Limrun iOS instance');
await driver.executeScript('window.location.assign(arguments[0])', ['https://news.ycombinator.com/']);
console.log('Navigated to Hacker News');

// Switch to webview context (requires remote debugger)
const contexts = await driver.getContexts();
console.log('Available contexts:', contexts);
const webviewContext = contexts.find((ctx) => String(ctx).includes('WEBVIEW'));
if (!webviewContext) {
  throw new Error('WEBVIEW context not found');
}
await driver.switchContext(String(webviewContext));
console.log('Switched to WEBVIEW context');
await driver.pause(1_000);

await driver.execute('window.scrollTo(0, document.body.scrollHeight)');
console.log('Scrolled to the bottom');

// Find and click the "More" link at the bottom using CSS selector
console.time('click.morelink');
await driver.$('a.morelink').click();
console.timeEnd('click.morelink');

console.time('getPageSource');
await driver.getPageSource();
console.timeEnd('getPageSource');
console.log('Done');

} finally {
  await driver.deleteSession();
}
