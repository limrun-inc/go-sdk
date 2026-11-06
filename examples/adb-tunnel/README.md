# Connect local ADB

The existing Go ADB example now waits for a ready instance and cleans it up.
Run from the repository root with `LIM_API_KEY` set:

```sh
go run ./examples/adb-tunnel
```

Run the printed `adb connect` command in a second terminal. Ctrl+C or the
five-minute timeout closes the tunnel and deletes the emulator.
For destination routing and HTTP inspection, see [tunnel](../tunnel).
