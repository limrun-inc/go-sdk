# Monitor app exits, crashes, hangs, and syslog

Attaches to an existing iOS instance without launching or relaunching the app.
`MonitorApp` receives exit details, crash reports, and hang events. `StreamSyslog`
prints the simulator’s syslog through a Go channel.

Run from the repository root with `LIM_API_KEY` set:

```sh
go run ./examples/ios-monitor -instance ios_example -bundle com.example.app
```

Use a ready instance ID and an installed app’s bundle ID. Press Ctrl+C to stop
monitoring. The example leaves the instance and app running.
See [main.go](main.go) for callback and stream cleanup, or
[fullstack](../fullstack) for monitoring attached to `LaunchApp`.
