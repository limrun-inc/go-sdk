package limrun_test

import (
	"encoding/json"
	"strings"
	"testing"

	limrun "github.com/limrun-inc/go-sdk"
)

// A location on the equator or the prime meridian is valid and must reach the
// API; the TypeScript SDK sends zero coordinates unchanged.
func TestClientLocationKeepsZeroCoordinates(t *testing.T) {
	params := map[string]json.Marshaler{
		"ios": limrun.IosInstanceNewParams{Spec: limrun.IosInstanceNewParamsSpec{Clues: []limrun.IosInstanceNewParamsSpecClue{{
			Kind:           "ClientLocation",
			ClientLocation: &limrun.IosInstanceNewParamsSpecClueClientLocation{Latitude: 0, Longitude: 0},
		}}}},
		"android": limrun.AndroidInstanceNewParams{Spec: limrun.AndroidInstanceNewParamsSpec{Clues: []limrun.AndroidInstanceNewParamsSpecClue{{
			Kind:           "ClientLocation",
			ClientLocation: &limrun.AndroidInstanceNewParamsSpecClueClientLocation{Latitude: 0, Longitude: 0},
		}}}},
		"xcode": limrun.XcodeInstanceNewParams{Spec: limrun.XcodeInstanceNewParamsSpec{Clues: []limrun.XcodeInstanceNewParamsSpecClue{{
			Kind:           "ClientLocation",
			ClientLocation: &limrun.XcodeInstanceNewParamsSpecClueClientLocation{Latitude: 0, Longitude: 0},
		}}}},
		"gradle": limrun.GradleInstanceNewParams{Spec: limrun.GradleInstanceNewParamsSpec{Clues: []limrun.GradleInstanceNewParamsSpecClue{{
			Kind:           "ClientLocation",
			ClientLocation: &limrun.GradleInstanceNewParamsSpecClueClientLocation{Latitude: 0, Longitude: 0},
		}}}},
	}
	for name, value := range params {
		data, err := value.MarshalJSON()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(string(data), `"latitude":0`) || !strings.Contains(string(data), `"longitude":0`) {
			t.Errorf("%s dropped zero coordinates: %s", name, data)
		}
	}
}
