# Run Detox on iOS

Go provisions the Expo Go simulator and deletes it after the run. The JavaScript adapter runs the same Detox configuration and four-screenshot test as the TypeScript example.

Run from the repository root with Go 1.25+ and `LIM_API_KEY` set:

```sh
npm --prefix examples/detox-ios ci
export EXPO_URL="exp://your-reachable-metro-host:8081"
go run ./examples/detox-ios
```

Install Node.js and serve [sample-detox-with-expo](https://github.com/limrun-inc/sample-detox-with-expo), as in the TypeScript example; the test expects its `demo-*` selectors. `@limrun/detox` is a separate JavaScript package whose preparation API requires a TypeScript instance client. Consequently [runner.mjs](runner.mjs) also depends on `@limrun/api`; this example does **not** claim to port the Detox driver to Go.

`DETOX_SESSION_ID`, `DETOX_VERSION`, and `DETOX_ARTIFACTS_DIR` have the same meanings as in TypeScript. Relative artifact paths are resolved from `examples/detox-ios`; the default is `artifacts/limrun-detox`. Set `LIMRUN_KEEP_INSTANCE=true` to keep the simulator.

[TypeScript counterpart](https://github.com/limrun-inc/typescript-sdk/tree/319078ef75a7f8a4e1abf36b73570ff11064453d/examples/detox-ios) · [All examples](../README.md)
