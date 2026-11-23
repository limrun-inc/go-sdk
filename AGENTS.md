# Agent Instructions

Run `./scripts/lint` and `./scripts/test` before pushing a commit.

## Keep the TypeScript SDK in sync

The TypeScript SDK (github.com/limrun-inc/typescript-sdk) offers the same instance clients and helpers as
this SDK. A change to either SDK lands in the other one too, in the same piece of work:

- New or changed instance client calls, options, result fields and wire messages
  (the `websocket/ios` and `websocket/android` packages here, `src/ios-client.ts` and
  `src/instance-client.ts` there).
- Helpers built on those clients, such as the `xcrun` shims, the Maestro wiring and the Argent bridge.
- Behavior and bug fixes. A bug fixed in one SDK is fixed in both.

Open the two pull requests together and link each from the other. Use the same names, adjusted to each
language's conventions (`StartArgentBridge` and `startArgentBridge`), and the same defaults.

Some files are shared verbatim. Edit both copies and keep them byte-identical:

- `websocket/ios/shim/xcrun.js` and the script `xcrunShimSource()` renders without options in the TypeScript
  SDK's `src/ios-shim.ts`.
- `websocket/ios/shim/argent-xcrun.js` and `ARGENT_XCRUN_SHIM` in the TypeScript SDK's `src/ios-argent.ts`.

The `lim` CLI lives only in the TypeScript repository. A CLI-only change needs no Go counterpart, but the
SDK calls it uses do.
