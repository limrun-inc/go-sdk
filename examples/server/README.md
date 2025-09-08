# Server Example

This example shows how you can call Limrun APIs from your server.

The most important piece is that you can provide `ClientIP` **scheduling clue**. This ability lets you provide your end-user's
IP address so that we can choose the closest region automatically, and they have the best experience.

Run the server:
```bash
LIM_TOKEN=lim_somevalue

go run main.go
```

Make a request to trigger creation:
```bash
curl localhost:3000
```

## Scheduling Clues

The server code uses `X-Forwarded-For` header to get the very first IP address that the request originated from, which
will likely be available in your server as well.

It's **highly recommended** that you propagate the IP of the host that will stream, otherwise, the region closest to your
server will be selected.

To test, you can mock that header to give different client IPs as scheduling clue:
```bash
# An IP in Los Angeles will provision from our US West-1 region.
curl -H 'X-Forwarded-For: 38.32.68.57' localhost:3000

# An IP from Helsinki will provision from our EU North-1 region.
curl -H 'X-Forwarded-For: 37.27.234.232' localhost:3000
```

In cases where you'd like to specify the region explicitly, you can always set `spec.region` field in the creation call.
