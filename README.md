# Limrun Go SDK

The Limrun Go SDK aims to provide idiomatic way of interacting with Limrun APIs in Go. The source of truth for the API
is defined in [`openapiv3.yaml`](./openapiv3.yaml) which we generate the base Go client from.

On top of the generated client, there are several helper methods that make it easier to consume the API as well as
structs that help with interacting with the instances directly, such as WebSocket-based TCP tunnel for `adb` to connect
to Android instances.

## Getting Started

Import the SDK to your Go program:

```bash
go get -u github.com/limrun-inc/go-sdk
```

Get an API token from Limrun Console.

```bash
export LIM_TOKEN=lim_somevalue
```

Create your first instance:

```go
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/limrun-inc/go-sdk/api"
)

func main() {
	token := os.Getenv("LIM_TOKEN") // lim_yourtoken
	limrun := api.NewDefaultClient(token)
	
	init := time.Now()
	instance, err := limrun.CreateAndroidInstance(context.TODO(), &api.AndroidInstanceCreate{}, api.CreateAndroidInstanceParams{
		Wait:           api.NewOptBool(true),
	})
	if err != nil {
		log.Fatalf("failed to create an android instance: %s", err)
    }
	log.Printf("Instance created in %s\n", time.Since(init))
	log.Printf("Streaming URL: %s\n", instance.Status.EndpointWebSocketUrl.Value)
	log.Printf("Instance Token: %s\n", instance.Status.Token)
}
```

It will automatically redirect the request to the nearest region to get you the closest instance for low latency streaming.

In addition, you can see [`server`](./examples/server) example where you can provide a ClientIP as scheduling clue so
that it gives you an instance closest to that IP address, which is especially useful if you embed Limrun instances in
your product for your end users.

## Examples

See [examples](./examples) folder for more complex cases.

## Contact Us

Reach out to Limrun at `contact@limrun.com`

## License

Limrun Go SDK is under the Apache 2.0 license.
