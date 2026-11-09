# Run Appium on iOS

Go creates a simulator with WebDriverAgent, reads `DeviceInfo`, checks the WDA endpoint, and restarts WDA with `Simctl` when needed. [runner.mjs](runner.mjs) retains the TypeScript example’s WebdriverIO test: open Hacker News in Safari, switch to its webview, scroll, and click More. The simulator remains available.

Run from the repository root with Go 1.25+ and `LIM_API_KEY` set:

```sh
npm --prefix examples/appium-ios ci
go run ./examples/appium-ios
```

Install Node.js, Appium, and the Limrun XCUITest driver, then start Appium in another terminal:

```sh
npm install --global appium
appium driver install --source npm @limrun/appium-xcuitest-driver@10.14.6-lim.10
appium
```

The runner connects to Appium at `127.0.0.1:4723`. All Limrun SDK calls are in [main.go](main.go); credentials reach the JavaScript runner through stdin. Appium requires the WDA HTTPS port to be explicit.

[TypeScript counterpart](https://github.com/limrun-inc/typescript-sdk/tree/319078ef75a7f8a4e1abf36b73570ff11064453d/examples/appium-ios) · [All examples](../README.md)
