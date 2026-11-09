# Tunnel app traffic through this computer

Creates an Android instance and calls `StartTunnel` with `*.example.test` selectors.
HTTP inspection captures bodies and persists the capture for 72 hours.
`HARAssembler.Add` combines inspection events and prints each completed request’s
method, URL, and status. The example runs for 30 seconds, then closes the tunnel
and deletes the instance; Ctrl+C also cleans up.

Run from the repository root with `LIM_API_KEY` set:

```sh
go run ./examples/tunnel
```

Send traffic from the emulator to a matching host reachable from this computer.
Edit `Selectors` in [main.go](main.go) to match your development service. Without
matching traffic the tunnel remains idle and prints no requests.

The older ADB-forwarding example is now [adb-tunnel](../adb-tunnel).

[TypeScript counterpart](https://github.com/limrun-inc/typescript-sdk/tree/319078ef75a7f8a4e1abf36b73570ff11064453d/examples/tunnel) · [All examples](../README.md)
