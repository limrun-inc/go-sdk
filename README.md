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

Create your first instance:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/limrun-inc/go-sdk/api"
)

func main() {
	organizationId := os.Getenv("ORGANIZATION_ID") // org_somevalue
	token := os.Getenv("LIM_TOKEN")                // lim_somevalue

	limrun, err := api.NewClient("https://edge.limrun.net", api.WithToken(token))
	if err != nil {
		log.Fatalf("failed to create limrun client: %s", err)
	}

	instance, err := limrun.CreateAndroidInstance(context.TODO(), &api.AndroidInstanceCreate{}, api.CreateAndroidInstanceParams{
		OrganizationId: organizationId,
		Wait:           api.NewOptBool(true),
	})
	if err != nil {
		log.Fatalf("failed to create an android instance: %s", err)
    }
	fmt.Printf("Streaming URL: %s\n", instance.Status.EndpointWebSocketUrl.Value)
	fmt.Printf("Instance Token: %s\n", instance.Status.Token)
}
```

## Examples

See [examples](./examples) folder for more complex cases.

## Contact Us

Reach out to Limrun at `contact@limrun.com`

## License

Limrun Go SDK is under the Apache 2.0 license.
