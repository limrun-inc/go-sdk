# AndroidInstances

Response Types:

- <a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk">limrun</a>.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#AndroidInstance">AndroidInstance</a>

Methods:

- <code title="post /v1/android_instances">client.AndroidInstances.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#AndroidInstanceService.New">New</a>(ctx <a href="https://pkg.go.dev/context">context</a>.<a href="https://pkg.go.dev/context#Context">Context</a>, params <a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk">limrun</a>.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#AndroidInstanceNewParams">AndroidInstanceNewParams</a>) (<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk">limrun</a>.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#AndroidInstance">AndroidInstance</a>, <a href="https://pkg.go.dev/builtin#error">error</a>)</code>
- <code title="get /v1/android_instances/{id}">client.AndroidInstances.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#AndroidInstanceService.Get">Get</a>(ctx <a href="https://pkg.go.dev/context">context</a>.<a href="https://pkg.go.dev/context#Context">Context</a>, id <a href="https://pkg.go.dev/builtin#string">string</a>) (<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk">limrun</a>.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#AndroidInstance">AndroidInstance</a>, <a href="https://pkg.go.dev/builtin#error">error</a>)</code>
- <code title="get /v1/android_instances">client.AndroidInstances.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#AndroidInstanceService.List">List</a>(ctx <a href="https://pkg.go.dev/context">context</a>.<a href="https://pkg.go.dev/context#Context">Context</a>, query <a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk">limrun</a>.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#AndroidInstanceListParams">AndroidInstanceListParams</a>) ([]<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk">limrun</a>.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#AndroidInstance">AndroidInstance</a>, <a href="https://pkg.go.dev/builtin#error">error</a>)</code>
- <code title="delete /v1/android_instances/{id}">client.AndroidInstances.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#AndroidInstanceService.Delete">Delete</a>(ctx <a href="https://pkg.go.dev/context">context</a>.<a href="https://pkg.go.dev/context#Context">Context</a>, id <a href="https://pkg.go.dev/builtin#string">string</a>) <a href="https://pkg.go.dev/builtin#error">error</a></code>

# Assets

Response Types:

- <a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk">limrun</a>.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#Asset">Asset</a>
- <a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk">limrun</a>.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#AssetGetOrNewResponse">AssetGetOrNewResponse</a>

Methods:

- <code title="get /v1/assets/{assetId}">client.Assets.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#AssetService.Get">Get</a>(ctx <a href="https://pkg.go.dev/context">context</a>.<a href="https://pkg.go.dev/context#Context">Context</a>, assetID <a href="https://pkg.go.dev/builtin#string">string</a>, query <a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk">limrun</a>.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#AssetGetParams">AssetGetParams</a>) (<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk">limrun</a>.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#Asset">Asset</a>, <a href="https://pkg.go.dev/builtin#error">error</a>)</code>
- <code title="get /v1/assets">client.Assets.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#AssetService.List">List</a>(ctx <a href="https://pkg.go.dev/context">context</a>.<a href="https://pkg.go.dev/context#Context">Context</a>, query <a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk">limrun</a>.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#AssetListParams">AssetListParams</a>) ([]<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk">limrun</a>.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#Asset">Asset</a>, <a href="https://pkg.go.dev/builtin#error">error</a>)</code>
- <code title="put /v1/assets">client.Assets.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#AssetService.GetOrNew">GetOrNew</a>(ctx <a href="https://pkg.go.dev/context">context</a>.<a href="https://pkg.go.dev/context#Context">Context</a>, body <a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk">limrun</a>.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#AssetGetOrNewParams">AssetGetOrNewParams</a>) (<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk">limrun</a>.<a href="https://pkg.go.dev/github.com/limrun-inc/go-sdk#AssetGetOrNewResponse">AssetGetOrNewResponse</a>, <a href="https://pkg.go.dev/builtin#error">error</a>)</code>
