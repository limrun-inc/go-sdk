# Port Forwarding Example

This example demonstrates how you can access any TCP port of the iOS instance locally over a tunnel.
The main use case is the WebDriverAgent which is an HTTP server application that Appium framework uses to expose
test functionality.

With port-forwarding, you can make the WebDriverAgent server available to be accessed by Appium that runs locally.

Run the example:
```bash
LIM_API_KEY=lim_somevalue

go run examples/port-forward/main.go
```

It will print a port number opened locally for you to access. The example sets the optional local port to `8100`.
While it's running, you can make a request like the following:
```bash
curl http://127.0.0.1:8100/health
```
