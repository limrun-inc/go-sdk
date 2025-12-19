# Assets Example

This example shows how to upload an asset or multiple assets, e.g. APK file, to Limrun and provision an Android instance
that has it pre-installed.

Run the example:
```bash
LIM_API_KEY=lim_somevalue

# Single APK
go run main.go <path to APK file>

# Split APKs of the same app
go run main.go <path to folder with APKs>
```
