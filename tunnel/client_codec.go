package tunnel

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const DefaultWindow = 1024 * 1024

func ValidateSelectorValues(values []string, options ...ValidateOptions) ([]string, error) {
	var opts ValidateOptions
	if len(options) > 0 {
		opts = options[0]
	}
	s, e := ParseSelectors(values, opts)
	if e != nil {
		return nil, e
	}
	return FormatSelectors(s), nil
}
func EncodeClientMessage(m ClientMessage) ([]byte, error) {
	p := map[string]any{"type": m.Type}
	switch m.Type {
	case "start":
		if m.Version != Version {
			return nil, protocolErrorf("unsupported tunnel version %d", m.Version)
		}
		values, err := ValidateSelectorValues(m.SelectorValues)
		if err != nil {
			return nil, err
		}
		inspection, err := NormalizeInspectionConfig(m.Inspection)
		if err != nil {
			return nil, err
		}
		p["version"] = m.Version
		p["selectors"] = values
		p["inspection"] = inspection
		p["window"] = m.Window
		if m.Name != "" {
			p["name"] = m.Name
		}
		if m.SkipsInactivityReset {
			p["resetsInactivityTimeout"] = false
		}
	case "openOk":
		p["connId"] = m.ConnID
		p["transport"] = m.TransportResult
		p["window"] = m.Window
	case "openFail", "reset":
		p["connId"] = m.ConnID
		p["reason"] = m.Reason
		if m.OSCode != "" {
			p["osCode"] = m.OSCode
		}
	case "fin":
		p["connId"] = m.ConnID
	case "windowUpdate":
		p["connId"] = m.ConnID
		p["increment"] = m.Increment
	default:
		return nil, protocolErrorf("unknown tunnel control message type %s", m.Type)
	}
	data, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	if _, err = DecodeClientMessage(data); err != nil {
		return nil, err
	}
	return data, nil
}
func DecodeServerMessage(data []byte) (m ServerMessage, err error) {
	r, err := readRecord(data)
	if err != nil {
		return m, err
	}
	m.Type, err = readString(r, "type")
	if err != nil {
		return m, err
	}
	switch m.Type {
	case "ready":
		version, e := readInteger(r, "version")
		if e != nil {
			return m, e
		}
		m.Version = int(version)
		if m.TunnelID, err = readString(r, "tunnelId"); err != nil {
			return m, err
		}
		if m.ConfigHash, err = readString(r, "configHash"); err != nil {
			return m, err
		}
		if m.Name, err = readOptionalString(r, "name"); err != nil {
			return m, err
		}
		m.Selectors, err = readSelectorReports(r["selectors"])
	case "open":
		if m.ConnID, err = readConnectionID(r); err != nil {
			return m, err
		}
		if m.SelectorID, err = readString(r, "selectorId"); err != nil {
			return m, err
		}
		if m.Host, err = readString(r, "host"); err != nil {
			return m, err
		}
		if m.Port, err = readPort(r, "port"); err != nil {
			return m, err
		}
		if m.Window, err = readRequiredWindow(r); err != nil {
			return m, err
		}
		tr, e := readRecord(r["transport"])
		if e != nil {
			return m, e
		}
		kind, e := readString(tr, "type")
		if e != nil {
			return m, e
		}
		m.Transport.Type = TransportType(kind)
		if m.Transport.Type == TransportTLS {
			if m.Transport.ServerName, err = readString(tr, "serverName"); err != nil {
				return m, err
			}
			if m.Transport.ALPNProtocols, err = readStringArray(tr, "alpnProtocols", "ALPN protocol"); err != nil {
				return m, err
			}
		}
		m.Transport, err = normalizeTransportRequest(m.Transport)
	case "fin":
		m.ConnID, err = readConnectionID(r)
	case "reset":
		if m.ConnID, err = readConnectionID(r); err != nil {
			return m, err
		}
		if m.Reason, err = readString(r, "reason"); err != nil {
			return m, err
		}
		m.OSCode, err = readOptionalString(r, "osCode")
	case "windowUpdate":
		if m.ConnID, err = readConnectionID(r); err != nil {
			return m, err
		}
		m.Increment, err = readWindowIncrement(r)
	case "error":
		m.Code, err = readString(r, "code")
	default:
		err = protocolErrorf("unknown tunnel control message type %s", m.Type)
	}
	return m, err
}
func readSelectorReports(data []byte) ([]SelectorReport, error) {
	var records []json.RawMessage
	if err := json.Unmarshal(data, &records); err != nil || records == nil {
		return nil, protocolErrorf("selectors must be an array")
	}
	result := make([]SelectorReport, 0, len(records))
	for i, raw := range records {
		r, err := readRecord(raw)
		if err != nil {
			return nil, err
		}
		var report SelectorReport
		if report.ID, err = readString(r, "id"); err != nil {
			return nil, err
		}
		if report.ID != fmt.Sprintf("selector-%d", i+1) {
			return nil, protocolErrorf("invalid tunnel selector id %s", report.ID)
		}
		kind, err := readString(r, "kind")
		if err != nil {
			return nil, err
		}
		if kind != "route" && kind != "domain" {
			return nil, protocolErrorf("invalid tunnel selector kind %s", kind)
		}
		report.Kind = SelectorKind(kind)
		if report.Value, err = readString(r, "value"); err != nil {
			return nil, err
		}
		if b, ok := r["binds"]; ok {
			var binds []json.RawMessage
			if err = json.Unmarshal(b, &binds); err != nil || binds == nil {
				return nil, protocolErrorf("binds must be an array")
			}
			report.Binds = []BindReport{}
			for _, raw := range binds {
				br, err := readRecord(raw)
				if err != nil {
					return nil, err
				}
				var bind BindReport
				if bind.Address, err = readString(br, "address"); err != nil {
					return nil, err
				}
				status, err := readString(br, "status")
				if err != nil {
					return nil, err
				}
				bind.Status = BindStatus(status)
				if bind.Status != BindStatusOK && bind.Status != BindStatusConflict && bind.Status != BindStatusError {
					return nil, protocolErrorf("invalid tunnel bind status %s", status)
				}
				if bind.OSCode, err = readOptionalString(br, "osCode"); err != nil {
					return nil, err
				}
				report.Binds = append(report.Binds, bind)
			}
		}
		result = append(result, report)
	}
	return result, nil
}
func AssertOpenAllowed(m ServerMessage, selectors []string) (SelectorKind, error) {
	parsed, err := ParseSelectors(selectors, ValidateOptions{})
	if err != nil {
		return "", err
	}
	index := -1
	if strings.HasPrefix(m.SelectorID, "selector-") {
		text := strings.TrimPrefix(m.SelectorID, "selector-")
		if text != "" && text[0] != '0' {
			if i, e := strconv.Atoi(text); e == nil {
				index = i - 1
			}
		}
	}
	fail := func() (SelectorKind, error) {
		return "", protocolErrorf("server requested undeclared selector %s %s:%d", m.SelectorID, m.Host, m.Port)
	}
	if index < 0 || index >= len(parsed.Entries) || m.Port == 53 {
		return fail()
	}
	entry := parsed.Entries[index]
	if entry.Kind == SelectorKindRoute {
		one, e := ParseSelectors([]string{entry.Value}, ValidateOptions{})
		if e != nil {
			return "", e
		}
		r := one.Routes[0]
		if m.Host != r.Host || m.Port != r.Port {
			return fail()
		}
	} else if !DomainMatches(entry.Value, m.Host) {
		return fail()
	}
	return entry.Kind, nil
}
func AssertReady(m ServerMessage) error {
	if m.Version != Version {
		return protocolErrorf("unsupported tunnel version %d", m.Version)
	}
	return nil
}
