# Go SDK API guide

The library implements the protocols and helpers in TypeScript SDK 0.69.1,
commit `319078ef75a7f8a4e1abf36b73570ff11064453d`. Go methods take a context for
cancellation and return errors. Streams use channels and explicit stop methods.
The TypeScript repository's CLI and React packages are separate products.

## REST services

Create a client with `limrun.NewClient()`. It reads `LIM_API_KEY` and
`LIMRUN_BASE_URL`; request options can override either value.

| Service | Methods |
| --- | --- |
| `AndroidInstances` | `New`, `Get`, `List`, `ListAutoPaging`, `Update`, `Delete`, `CreateClient`, `ListSessionArtifacts` |
| `IosInstances` | `New`, `Get`, `List`, `ListAutoPaging`, `Update`, `Delete`, `CreateClient`, `ListSessionArtifacts` |
| `XcodeInstances` | `New`, `Get`, `List`, `ListAutoPaging`, `Update`, `Delete`, `CreateClient`, `CreateClientWithCredentials`, `ListBuildLogs`, `ListBazelBuildLogs`, `GetSnapshot`, `BindSnapshotKey`, `FollowSnapshot` |
| `GradleInstances` | `New`, `Get`, `List`, `ListAutoPaging`, `Update`, `Delete`, `CreateClient`, `ListBuildLogs` |
| `Assets` | `List`, `Get`, `GetOrNew`, `GetOrUpload`, `Update`, `CreateUploadURL`, `Delete` |
| `AccessTokens` | `New`, `Revoke` |
| `ScopedTokens` | `New`, the deprecated access-token alias |
| `XcodeSnapshots`, `IosSnapshots` | `List`, `Get`, `Delete`, scoped by organization |
| `Analytics` | `Get`, `GetInstances` |

Instance updates accept an `IfMatch` ETag and support labels and delegated roles.
Empty maps and permission lists remain explicit updates. Instance list cursors
use `metadata.id`.

Xcode snapshot operations retain the legacy `GetCache`, `BindCacheKey`, and
`FollowCache` names. Standalone `FollowXcodeSnapshot` and `FollowXcodeCache`
helpers report progress, deduplicate events, and return typed timeout or
instance-gone errors.

## Instance clients

Pass a ready instance to its service's `CreateClient` method. For an API URL and
token, construct a client directly with `ios.NewClient`, `android.NewClient`,
`build.NewXcodeClient`, or `build.NewGradleClient`.

| Package | Operations |
| --- | --- |
| `websocket/ios` | Screenshots, accessibility actions and batches, keyboard and touch input, Duo displays and folding, app installation and launch, `WatchApp`, persistent `MonitorApp`, crash and hang callbacks, app logs and syslog, simctl processes, files and keychain, camera and microphone input, recordings, StoreKit, session captures, tunnels, xcrun shim, and Maestro preparation |
| `websocket/android` | Screenshots, UIAutomator selectors and actions, app installation and launch, `WatchApp`, crash and ANR callbacks, ADB shell and tunnels, logcat, files, app sync, camera and microphone input, Wi-Fi bandwidth, recordings, session captures, and destination tunnels |
| `build` | Xcode and Gradle execution, shell commands, process output and cancellation, resumable build logs, XCTest and store-upload events, file sync and download, simulator attachment, Xcode selection, and Bazel remote execution |

Close iOS and Android clients with `Close`. Control connections reconnect with
bounded backoff. Requests in flight fail when their connection drops; callers
decide whether to retry the operation. Connection-state callbacks expose this
transition.

`MonitorApp` remains registered across app launches on the same connection.
It does not replay its registration after a connection loss. A new server session
needs a new registration, as in TypeScript.

The [iOS monitoring example](examples/ios-monitor/main.go) demonstrates structured
exit details and syslog streaming. Existing examples cover
[basic iOS control](examples/websocket/main.go),
[asset uploads](examples/assets/main.go), and [ADB tunnels](examples/tunnel/main.go).

## Shared helpers

| Package | Operations |
| --- | --- |
| `session` | Artifact, log, and event models; persistence TTLs; resumable SSE streams with `Entries`, `Err`, `Stop`, and `Done` |
| `syncfolder` | Folder manifests, nested gitignore rules, additional files, symlinks, gzip and zstd, VCDIFF deltas, basis caches, missing-basis recovery, file watching, app archives, and Android basis downloads |
| `tunnel` | TCP forwarding, reverse forwarding, destination selectors and protocol codecs, private connectors, HTTP proxies, TLS inspection events, and HAR assembly |
| `build` | Build-setting validation, RBE retries and readiness, Bazel workspace discovery, target inference, and workspace configuration files |

HTTP and WebSocket transports honor proxy environment variables. A custom HTTP
client can be supplied through REST request options or instance-client options.
Use `errors.As` with `*limrun.DirectInstanceHTTPError` to inspect direct-instance
HTTP failures. A 404 also supports the existing `*limrun.Error` type.

For complete Go signatures and types, run:

```sh
go doc github.com/limrun-inc/go-sdk
go doc github.com/limrun-inc/go-sdk/websocket/ios.Client
go doc github.com/limrun-inc/go-sdk/websocket/android.Client
go doc github.com/limrun-inc/go-sdk/build.XcodeClient
go doc github.com/limrun-inc/go-sdk/build.GradleClient
go doc github.com/limrun-inc/go-sdk/tunnel
```

## Local validation

`./scripts/test` starts Prism with the checked-in SDK OpenAPI fixture and runs
the Go tests. `./scripts/test -race` also enables Go's race detector. The suite
uses local HTTP and WebSocket servers; it does not create Limrun instances.
Tests for the xcrun executable and native delta interoperability run when Node.js
and `xdelta3`, respectively, are installed.
