package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/limrun-inc/go-sdk/internal/direct"
)

type ActiveDestinationTunnel struct {
	TunnelID   string           `json:"tunnelId"`
	Name       string           `json:"name,omitempty"`
	State      string           `json:"state"`
	Selectors  []SelectorReport `json:"selectors"`
	Inspection InspectionConfig `json:"inspection"`
}
type TunnelFailure struct {
	TunnelID string `json:"tunnelId"`
	Code     string `json:"code"`
}
type TunnelDialFailure struct {
	TunnelID     string `json:"tunnelId"`
	ConnectionID uint32 `json:"connectionId"`
	SelectorID   string `json:"selectorId"`
	Reason       string `json:"reason"`
	OSCode       string `json:"osCode,omitempty"`
}
type DestinationStatus struct {
	Active          *ActiveDestinationTunnel `json:"active,omitempty"`
	LastFailure     *TunnelFailure           `json:"lastFailure,omitempty"`
	LastDialFailure *TunnelDialFailure       `json:"lastDialFailure,omitempty"`
}

func GetDestinationStatus(ctx context.Context, base, token string, clients ...*http.Client) (*DestinationStatus, error) {
	endpoint, err := deriveEndpoint(base, "tunnel/status", false)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c := direct.Client{Token: token}
	if len(clients) > 0 {
		c.HTTPClient = clients[0]
	}
	res, err := c.Do(ctx, "GET", endpoint, nil, "", -1)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return DecodeDestinationStatus(raw)
}
func StopDestination(ctx context.Context, base, token, tunnelID string, clients ...*http.Client) error {
	if strings.TrimSpace(tunnelID) == "" {
		return fmt.Errorf("tunnelId must not be empty")
	}
	endpoint, err := deriveEndpoint(base, "tunnel/"+url.PathEscape(tunnelID), false)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c := direct.Client{Token: token}
	if len(clients) > 0 {
		c.HTTPClient = clients[0]
	}
	res, err := c.Do(ctx, "DELETE", endpoint, nil, "", -1)
	if err != nil {
		return err
	}
	res.Body.Close()
	return nil
}
func DecodeDestinationStatus(data []byte) (*DestinationStatus, error) {
	r, err := readRecord(data)
	if err != nil {
		return nil, err
	}
	var result DestinationStatus
	if b, ok := r["active"]; ok {
		active, e := readRecord(b)
		if e != nil {
			return nil, e
		}
		a := &ActiveDestinationTunnel{}
		if a.TunnelID, e = requiredString(active, "tunnelId"); e != nil {
			return nil, e
		}
		if a.Name, e = readOptionalString(active, "name"); e != nil {
			return nil, e
		}
		if a.State, e = readString(active, "state"); e != nil {
			return nil, e
		}
		if a.State != "starting" && a.State != "ready" && a.State != "stopping" {
			return nil, protocolErrorf("invalid tunnel state %s", a.State)
		}
		if a.Selectors, e = readSelectorReports(active["selectors"]); e != nil {
			return nil, e
		}
		for _, s := range a.Selectors {
			if s.Value == "" {
				return nil, protocolErrorf("selector value must not be empty")
			}
			for _, bind := range s.Binds {
				if bind.Address == "" {
					return nil, protocolErrorf("bind address must not be empty")
				}
			}
		}
		if a.Inspection, e = readInspectionConfig(active); e != nil {
			return nil, e
		}
		result.Active = a
	}
	if b, ok := r["lastFailure"]; ok {
		failure, e := readRecord(b)
		if e != nil {
			return nil, e
		}
		f := &TunnelFailure{}
		if f.TunnelID, e = requiredString(failure, "tunnelId"); e != nil {
			return nil, e
		}
		if f.Code, e = requiredString(failure, "code"); e != nil {
			return nil, e
		}
		result.LastFailure = f
	}
	if b, ok := r["lastDialFailure"]; ok {
		failure, e := readRecord(b)
		if e != nil {
			return nil, e
		}
		f := &TunnelDialFailure{}
		if f.TunnelID, e = requiredString(failure, "tunnelId"); e != nil {
			return nil, e
		}
		if f.SelectorID, e = requiredString(failure, "selectorId"); e != nil {
			return nil, e
		}
		if f.Reason, e = requiredString(failure, "reason"); e != nil {
			return nil, e
		}
		copyRecord := map[string]json.RawMessage{"connId": failure["connectionId"]}
		if f.ConnectionID, e = readConnectionID(copyRecord); e != nil {
			return nil, e
		}
		if f.OSCode, e = readOptionalString(failure, "osCode"); e != nil {
			return nil, e
		}
		result.LastDialFailure = f
	}
	return &result, nil
}
func requiredString(r map[string]json.RawMessage, key string) (string, error) {
	value, err := readString(r, key)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", protocolErrorf("%s must not be empty", key)
	}
	return value, nil
}
