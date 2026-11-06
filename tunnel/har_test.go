package tunnel

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestHARMatchesTypeScriptFixtures(t *testing.T) {
	var fixtures struct {
		Cases []struct {
			Name, Body         string
			Limit              int
			Complete, Expected HAREntry
		}
	}
	data, err := os.ReadFile("testdata/har.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, tc := range fixtures.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			limit := tc.Limit
			if limit == 0 {
				limit = DefaultMaxBodyBytes
			}
			assembler, err := NewHARAssembler(limit)
			if err != nil {
				t.Fatal(err)
			}
			body, err := base64.StdEncoding.DecodeString(tc.Body)
			if err != nil {
				t.Fatal(err)
			}
			_, err = assembler.Add(InspectionEvent{Type: "body", RequestID: "req", Direction: "request", Body: []byte("hello")})
			if err != nil {
				t.Fatal(err)
			}
			_, err = assembler.Add(InspectionEvent{Type: "body", RequestID: "req", Direction: "response", Body: body})
			if err != nil {
				t.Fatal(err)
			}
			got, err := assembler.Add(InspectionEvent{Type: "complete", RequestID: "req", Complete: tc.Complete})
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(got)
			var normalized HAREntry
			_ = json.Unmarshal(encoded, &normalized)
			if !reflect.DeepEqual(normalized, tc.Expected) {
				want, _ := json.Marshal(tc.Expected)
				t.Fatalf("got=%s\nTS=%s", encoded, want)
			}
		})
	}
}
