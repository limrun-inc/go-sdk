package limrun

import "github.com/limrun-inc/go-sdk/internal/direct"

// DirectInstanceHTTPError reports a non-success response from an instance daemon.
// A 404 also matches *Error through errors.As, like a missing REST resource.
type DirectInstanceHTTPError = direct.HTTPError

func IsDirectInstanceHTTPError(err error, status int) bool { return direct.IsHTTPError(err, status) }
