# Sync an iOS app and stream logs

Syncs a local simulator `.app` bundle, installs it, and relaunches it when changes arrive. It streams app logs for 30 seconds, streams syslog filtered by bundle ID for another 30 seconds, then fetches the last ten app-log lines. The simulator remains available.

Run from the repository root with Go 1.25+ and `LIM_API_KEY` set:

```sh
go run ./examples/ios-hot-reload -watch /absolute/path/to/MyApp.app
```

Use a simulator build, not a device IPA. Omit `-watch` for a single sync. Put flags before the path. `SyncAppOptions.Watch` controls watching; the returned `StopWatching` stops it. Log batches arrive through `LogStream.Lines()`; `Err()` reports terminal errors and `Stop()` closes the stream. Press Ctrl+C to stop watching.

[TypeScript counterpart](https://github.com/limrun-inc/typescript-sdk/tree/319078ef75a7f8a4e1abf36b73570ff11064453d/examples/ios-hot-reload) · [All examples](../README.md)
