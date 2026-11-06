package tunnel

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestTypeScriptProtocolFixtures(t *testing.T) {
	raw, e := os.ReadFile("testdata/protocol.json")
	if e != nil {
		t.Fatal(e)
	}
	var f struct {
		RouteCases       []struct{ Input, Canonical Route }
		InvalidRouteSets []struct {
			Name   string
			Routes []Route
			Error  SelectorErrorCode
		}
		DomainCases       []struct{ Input, Canonical string }
		InvalidDomainSets []struct {
			Name    string
			Domains []string
			Error   SelectorErrorCode
		}
		DomainMatchCases []struct {
			Pattern, Host string
			Matches       bool
		}
		ConfigHashCases []struct {
			Name       string
			Selectors  []string
			Inspection InspectionConfig
			SHA256     string
		}
		OpenAllowedCases []struct {
			Name      string
			Selectors []string
			Open      struct {
				SelectorID string `json:"selectorId"`
				Host       string
				Port       int
			}
			Allowed bool
		}
		Client, Server []struct {
			Name    string
			Message json.RawMessage
		}
	}
	if e = json.Unmarshal(raw, &f); e != nil {
		t.Fatal(e)
	}
	for _, v := range f.RouteCases {
		got, e := ValidateRoutes([]Route{v.Input}, 1)
		if e != nil || !reflect.DeepEqual(got, []Route{v.Canonical}) {
			t.Errorf("route %v: %v %v", v.Input, got, e)
		}
	}
	for _, v := range f.InvalidRouteSets {
		_, e := ValidateRoutes(v.Routes, 1)
		var invalid *SelectorError
		if !errors.As(e, &invalid) || invalid.Code != v.Error {
			t.Errorf("%s: %v", v.Name, e)
		}
	}
	for _, v := range f.DomainCases {
		got, e := ValidateDomains([]string{v.Input})
		if e != nil || !reflect.DeepEqual(got, []string{v.Canonical}) {
			t.Errorf("domain %s: %v %v", v.Input, got, e)
		}
	}
	for _, v := range f.InvalidDomainSets {
		_, e := ValidateDomains(v.Domains)
		var invalid *SelectorError
		if !errors.As(e, &invalid) || invalid.Code != v.Error {
			t.Errorf("%s: %v", v.Name, e)
		}
	}
	for _, v := range f.DomainMatchCases {
		if DomainMatches(v.Pattern, v.Host) != v.Matches {
			t.Errorf("domain match %s/%s", v.Pattern, v.Host)
		}
	}
	for _, v := range f.ConfigHashCases {
		s, e := ParseSelectors(v.Selectors, ValidateOptions{})
		if e != nil {
			t.Fatal(e)
		}
		got, e := ConfigHashWithInspection(s, v.Inspection)
		if e != nil || got != v.SHA256 {
			t.Errorf("%s: %s %v", v.Name, got, e)
		}
	}
	for _, v := range f.OpenAllowedCases {
		_, e := AssertOpenAllowed(ServerMessage{SelectorID: v.Open.SelectorID, Host: v.Open.Host, Port: v.Open.Port}, v.Selectors)
		if (e == nil) != v.Allowed {
			t.Errorf("%s: %v", v.Name, e)
		}
	}
	for _, v := range f.Client {
		t.Run("client/"+v.Name, func(t *testing.T) {
			m, e := DecodeClientMessage(v.Message)
			if e != nil {
				t.Fatal(e)
			}
			got, e := EncodeClientMessage(m)
			if e != nil {
				t.Fatal(e)
			}
			if !equalJSON(got, v.Message) {
				t.Fatalf("%s != %s", got, v.Message)
			}
		})
	}
	for _, v := range f.Server {
		t.Run("server/"+v.Name, func(t *testing.T) {
			m, e := DecodeServerMessage(v.Message)
			if e != nil {
				t.Fatal(e)
			}
			got, e := EncodeServerMessage(m)
			if e != nil {
				t.Fatal(e)
			}
			if !equalJSON(got, v.Message) {
				t.Fatalf("%s != %s", got, v.Message)
			}
		})
	}
}
func equalJSON(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}
func TestTypeScriptBinaryFixtures(t *testing.T) {
	raw, e := os.ReadFile("testdata/binary.json")
	if e != nil {
		t.Fatal(e)
	}
	var f struct {
		DataFrames []struct {
			Name           string
			ConnID         uint32 `json:"connId"`
			Payload, Frame []byte
		}
		InvalidFrames []struct {
			Name  string
			Frame []byte
		}
	}
	if e = json.Unmarshal(raw, &f); e != nil {
		t.Fatal(e)
	}
	for _, v := range f.DataFrames {
		got, e := EncodeDataFrame(v.ConnID, v.Payload)
		if e != nil || !bytes.Equal(got, v.Frame) {
			t.Fatalf("%s: %v %v", v.Name, got, e)
		}
		id, p, e := DecodeDataFrame(v.Frame)
		if e != nil || id != v.ConnID || !bytes.Equal(p, v.Payload) {
			t.Fatalf("decode %s: %v", v.Name, e)
		}
	}
	for _, v := range f.InvalidFrames {
		if _, _, e := DecodeDataFrame(v.Frame); e == nil {
			t.Errorf("accepted %s", v.Name)
		}
	}
}
