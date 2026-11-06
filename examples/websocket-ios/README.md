# Control an iOS simulator

Demonstrates screenshots, typed accessibility trees, element taps, text input, keyboard actions, streaming `Simctl`, raw HID action batches, and `SoftReset`. It drives Safari and saves `screenshot.jpg` and `screenshot-final.jpg` in the repository root. The simulator remains available.

Run from the repository root with Go 1.22+ and `LIM_API_KEY` set:

```sh
go run ./examples/websocket-ios
```

`AccessibilitySelector{AccessibilityID: "Safari"}` corresponds to TypeScript’s `{ AXUniqueId: "Safari" }`; `ElementType` and `Label` correspond to `type` and `AXLabel`. Screens load asynchronously, so the example retries element selection with a deadline.

[TypeScript counterpart](https://github.com/limrun-inc/typescript-sdk/tree/319078ef75a7f8a4e1abf36b73570ff11064453d/examples/websocket-ios) · [All examples](../README.md)
