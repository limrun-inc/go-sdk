# Control an Android emulator

Demonstrates screenshots, XML and typed element trees, element lookup, selectors, taps, text entry, keys, URLs, and scrolling. It saves `android-screenshot.png` and `android-screenshot-final.png` in the repository root and keeps the emulator.

Run from the repository root with Go 1.22+ and `LIM_API_KEY` set:

```sh
go run ./examples/websocket-android
```

Selectors use pointers for optional values, such as `Text` and `Clickable`. `ElementTarget` accepts either a selector or coordinates. The sample reports missing optional UI elements and continues, matching the TypeScript scenario.

[TypeScript counterpart](https://github.com/limrun-inc/typescript-sdk/tree/319078ef75a7f8a4e1abf36b73570ff11064453d/examples/websocket-android) · [All examples](../README.md)
