# Test in-app purchases

Creates an Xcode instance and iOS simulator, attaches them, syncs and builds your project, then registers a StoreKit configuration. Both instances remain available for interactive purchase testing.

Run from the repository root with Go 1.25+ and `LIM_API_KEY` set:

```sh
go run ./examples/ios-iap-testing -bundle-id com.example.app -storekit /absolute/path/to/Products.storekit /absolute/path/to/project
```

Omit `-storekit` to call `DiscoverStoreKitConfig`. Open the printed simulator link and the app’s paywall so the server can capture product information. `-timeout 120` sets the discovery timeout in seconds. Put flags before the project path.

[TypeScript counterpart](https://github.com/limrun-inc/typescript-sdk/tree/319078ef75a7f8a4e1abf36b73570ff11064453d/examples/ios-iap-testing) · [All examples](../README.md)
