# Use a Go backend with remote control

The Go backend creates iOS or Android instances, optionally installs Expo Go, and reports app exits and crash details. The React frontend is copied from the TypeScript example and displays the `@limrun/ui` remote control.

Run from the repository root with Go 1.22+ and `LIM_API_KEY` set:

```sh
go run ./examples/fullstack/backend
```

Start the frontend in another terminal:

```sh
yarn --cwd examples/fullstack/frontend install --frozen-lockfile
yarn --cwd examples/fullstack/frontend dev
```

Open `http://localhost:5173`, choose a platform, and click **Create Instance**. **Stop Instance** deletes it. Closing the backend disconnects monitoring but leaves instances running. `LIM_API_KEY` stays in Go; the browser receives an instance-scoped connection token. See [backend/main.go](backend/main.go) for `New`, `CreateClient`, `LaunchApp` callbacks, and `Delete`. The backend listens on loopback port 3000.

[TypeScript counterpart](https://github.com/limrun-inc/typescript-sdk/tree/319078ef75a7f8a4e1abf36b73570ff11064453d/examples/fullstack) · [All examples](../README.md)
