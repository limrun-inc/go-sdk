# Go SDK examples

These examples mirror all 18 example directories in the
[TypeScript SDK](https://github.com/limrun-inc/typescript-sdk/tree/319078ef75a7f8a4e1abf36b73570ff11064453d/examples)
used for the v0.69.1 parity port. The existing Go ADB and iOS monitoring examples
are included too. Read each `main.go` (or `backend/main.go`) for the public Go
interfaces. Every example is self-contained: the few setup helpers it needs live
in its own directory, so any example can be copied out as a starting point.

Start with [ios-monitor](ios-monitor) for the original monitoring/crash/syslog
question, [websocket-ios](websocket-ios) for simulator control, and
[fullstack/backend/main.go](fullstack/backend/main.go) for backend integration.

## Run

Use Go 1.25 or newer and run commands from the repository root:

```sh
export LIM_API_KEY="your API key"
go run ./examples/websocket-ios
```

Examples create real Limrun resources when run. The table states which ones are
kept for inspection. Use Ctrl+C to stop long-running examples; examples that delete
instances use a separate cleanup context so cancellation does not skip deletion.
The default `LIMRUN_BASE_URL` is the production API; use the environment variable
to point the Go REST client at another endpoint.

| Example | Interfaces and scenario | Keeps instances? |
| --- | --- | --- |
| [asset](asset) | Asset upload and installation | Yes |
| [websocket-ios](websocket-ios) | iOS control, accessibility, simctl, HID actions, soft reset | Yes |
| [websocket-android](websocket-android) | Android control, selectors, keys, scrolling | Yes |
| [ios-monitor](ios-monitor) | MonitorApp, crash/hang callbacks, syslog | Uses an existing instance |
| [ios-hot-reload](ios-hot-reload) | SyncApp, watching, app logs and syslog | Yes |
| [ios-video-recording](ios-video-recording) | iOS recording and download | No |
| [android-video-recording](android-video-recording) | Android recording and download | No |
| [ios-iap-testing](ios-iap-testing) | AttachSimulator, Xcodebuild, StoreKit | Yes, both |
| [ios-with-xcode](ios-with-xcode) | Project sync, simulator builds, HTTP and MCP triggers | Yes, both |
| [xcode-sandbox](xcode-sandbox) | Standalone Xcode builds and artifact upload | Yes |
| [tunnel](tunnel) | Destination selectors, HTTP inspection and HAR | No |
| [adb-tunnel](adb-tunnel) | Local ADB forwarding | No |
| [web-inspector-ios](web-inspector-ios) | Lsof and multiplexed TCP forwarding | No |
| [maestro-ios](maestro-ios) | PrepareMaestroRun, runner checks, recording | No |
| [appium-ios](appium-ios) | WDA setup and WebdriverIO automation | Yes |
| [playwright](playwright) | Android sandbox configuration and Playwright | No |
| [detox-ios](detox-ios) | Go provisioning with the JavaScript Detox integration | Only with LIMRUN_KEEP_INSTANCE=true |
| [fullstack](fullstack) | Go instance API and React remote control | Until Stop Instance |
| [device-install](device-install) | Access tokens, exact asset authorization, OTA installation | CLI build lifecycle |
| [publish-to-stores](publish-to-stores) | Relay access token, signed iOS/Android publishing | CLI build lifecycle |

## Language boundaries

REST provisioning and Go-supported instance operations are in Go. The three React
frontends remain TypeScript, using `@limrun/ui`, Apple/Google auth, and device
installation browser packages. Appium and Playwright keep their JavaScript test
runners. Detox additionally needs a TypeScript instance client because
`@limrun/detox` accepts that client; its driver is a separate package. The optional
MCP transport in `ios-with-xcode` calls the Go build endpoint.

These dependencies are named in each README. Core SDK examples need only Go;
Maestro needs its CLI and Node.js for the xcrun shim. Store publishing and device
installation use the `lim` CLI just as their TypeScript counterparts do.

The old `assets`, `websocket`, and `server` directories now point to `asset`,
`websocket-ios`, and `fullstack`, matching the TypeScript directory names. The old
`tunnel` ADB scenario moved to `adb-tunnel`; `tunnel` now matches the destination
routing and HAR example.

## Local validation

Compile every Go example without credentials or cloud workloads:

```sh
go build ./examples/...
go vet ./examples/...
go test -race ./examples/...
```

Each frontend has a `yarn build` command. JavaScript adapters have npm lockfiles;
use `npm ci` in their directories. Compilation checks local interfaces and syntax;
end-to-end runs also require the services and credentials listed in each README.
