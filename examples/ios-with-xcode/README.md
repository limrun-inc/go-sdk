# Build and install on an attached simulator

Creates or reuses Xcode and iOS instances, calls `AttachSimulator`, watches project files with `Sync`, and builds with `Xcodebuild`. Both instances remain available. Ctrl+C stops the local watcher and HTTP server.

Run from the repository root with Go 1.22+ and `LIM_API_KEY` set:

```sh
go run ./examples/ios-with-xcode /absolute/path/to/project
```

Trigger another build with `curl http://localhost:3000/xcodebuild`.

The TypeScript example also exposes an MCP `build` tool. This version keeps that transport in a small JavaScript adapter using the same MCP SDK; the adapter calls the Go build endpoint. In a second terminal:

```sh
npm --prefix examples/ios-with-xcode ci
npm --prefix examples/ios-with-xcode run mcp
```

Configure the MCP client with `http://localhost:3001`. The Go HTTP build endpoint stays on port 3000. Only the optional MCP adapter needs Node.js.

[TypeScript counterpart](https://github.com/limrun-inc/typescript-sdk/tree/319078ef75a7f8a4e1abf36b73570ff11064453d/examples/ios-with-xcode) · [All examples](../README.md)
