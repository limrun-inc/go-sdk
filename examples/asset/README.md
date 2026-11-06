# Upload and install an asset

Downloads Expo Go 54.0.6, calls `Assets.GetOrUpload`, and creates an iOS instance with an `AssetName` initial asset. The uploaded asset and simulator remain available. The temporary archive is removed.

Run from the repository root with Go 1.22+ and `LIM_API_KEY` set:

```sh
go run ./examples/asset
```

The download URL and Expo version match the TypeScript example. Read [main.go](main.go) to see the upload and instance creation together.

[TypeScript counterpart](https://github.com/limrun-inc/typescript-sdk/tree/319078ef75a7f8a4e1abf36b73570ff11064453d/examples/asset) · [All examples](../README.md)
