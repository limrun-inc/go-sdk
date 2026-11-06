# Run Maestro on iOS

Creates a simulator with Expo Go and the Maestro runner, checks or launches the runner with `Simctl`, and uses `PrepareMaestroRun` to configure the local CLI. The copied YAML flow exercises the Expo sample app’s counter, greeting, and detail screen. The instance is deleted after the run.

Run from the repository root with Go 1.22+ and `LIM_API_KEY` set:

```sh
export EXPO_URL="exp://your-reachable-metro-host:8081"
go run ./examples/maestro-ios
```

Install Node.js and the Maestro CLI on this computer. Serve the [sample-expo-test-app](https://github.com/limrun-inc/sample-expo-test-app) at `EXPO_URL`; the flow expects its `demo-*` selectors. Maestro output goes to `examples/maestro-ios/artifacts/` and the recording to `examples/maestro-ios/video.mp4`, including when tests fail. `PrepareMaestroRun` returns `Env`, `Args`, and `MaestroVersion` and manages the runner proxy and xcrun shim.

[TypeScript counterpart](https://github.com/limrun-inc/typescript-sdk/tree/319078ef75a7f8a4e1abf36b73570ff11064453d/examples/maestro-ios) · [All examples](../README.md)
