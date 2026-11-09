# Forward Safari Web Inspector

Opens example.com, finds Safari’s UNIX socket with `Lsof`, and forwards it to `127.0.0.1:27753` with `tunnel.StartTCP` in multiplexed mode. Connect a Web Inspector protocol client to that TCP address. Ctrl+C closes the tunnel and deletes the simulator.

Run from the repository root with Go 1.25+ and `LIM_API_KEY` set:

```sh
go run ./examples/web-inspector-ios
```

This exposes the Web Inspector transport; it does not provide an inspector UI. The program also exits if the tunnel disconnects.

[TypeScript counterpart](https://github.com/limrun-inc/typescript-sdk/tree/319078ef75a7f8a4e1abf36b73570ff11064453d/examples/web-inspector-ios) · [All examples](../README.md)
