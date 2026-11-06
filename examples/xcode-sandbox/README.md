# Build in an Xcode sandbox

Creates an Xcode instance, watches and syncs a local project, runs an initial build, and serves an HTTP endpoint for subsequent builds. The workload can upload only under `xcode-example/`. The instance remains available after Ctrl+C.

Run from the repository root with Go 1.22+ and `LIM_API_KEY` set:

```sh
go run ./examples/xcode-sandbox /absolute/path/to/project
```

In another terminal:

```sh
curl http://localhost:3000/xcodebuild
curl "http://localhost:3000/xcodebuild?sdk=iphoneos&target=name/xcode-example/device-build.ipa"
```

The response streams build output. Successful uploads are found with `Assets.List` and `IncludeDownloadURL`; reading the download URL uses the caller’s permission. Device builds require an appropriately configured project. See `BuildUpload`, `XcodeProjectConfig`, and `ExecOptions` in [main.go](main.go).

[TypeScript counterpart](https://github.com/limrun-inc/typescript-sdk/tree/319078ef75a7f8a4e1abf36b73570ff11064453d/examples/xcode-sandbox) · [All examples](../README.md)
