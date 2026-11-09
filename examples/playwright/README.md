# Run Playwright on Android

Go creates an Android 15 instance with the Chrome command-line flag and Playwright Android sandbox `1.60.0-lim.1`. [runner.mjs](runner.mjs) uses Playwright 1.60.0 to browse the Playwright GitHub repository and save `examples/playwright/screenshot.png`. The instance is deleted after the test.

Run from the repository root with Go 1.25+ and `LIM_API_KEY` set:

```sh
npm --prefix examples/playwright ci
go run ./examples/playwright
```

Node.js runs Playwright’s Android API. Provisioning and deletion use the Go SDK. The scenario matches the TypeScript example, including its GitHub page selectors; site changes can require updating both examples.

[TypeScript counterpart](https://github.com/limrun-inc/typescript-sdk/tree/319078ef75a7f8a4e1abf36b73570ff11064453d/examples/playwright) · [All examples](../README.md)
