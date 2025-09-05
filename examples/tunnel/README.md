# Tunnel Example

This example shows how you can connect to Android instances from your local `adb` daemon so that you can have all your
Android tooling to recognize and use them.

Run the example:
```bash
LIM_TOKEN=lim_somevalue

go run main.go
```

Once it's running, you can see it pop up in Android Studio and others. An easy way to test is to use `scrcpy` which
is a GUI program to stream the screen of Android devices.

```bash
scrcpy -s <reported address>
```
