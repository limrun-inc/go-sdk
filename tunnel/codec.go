package tunnel

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
)

// ProtocolError reports a violation of the destination-tunnel wire contract.
// It terminates the whole session.
type ProtocolError struct {
	Message string
}

func (e *ProtocolError) Error() string { return e.Message }

func protocolErrorf(format string, args ...any) error {
	return &ProtocolError{Message: fmt.Sprintf(format, args...)}
}

// OpenFailureReason values mirror DestinationTunnelOpenFailureReason.
// Unknown values are preserved but normalized before metric use.
const (
	OpenFailDNSNotFound         = "dns_not_found"
	OpenFailDNSTemporaryFailure = "dns_temporary_failure"
	OpenFailConnectionRefused   = "connection_refused"
	OpenFailConnectionReset     = "connection_reset"
	OpenFailConnectionTimedOut  = "connection_timed_out"
	OpenFailUnreachable         = "unreachable"
	OpenFailPermissionDenied    = "permission_denied"
	OpenFailResourceExhausted   = "resource_exhausted"
	OpenFailSelectorNotAllowed  = "selector_not_allowed"
	OpenFailTLSHandshakeFailed  = "tls_handshake_failed"
	OpenFailTLSValidationFailed = "tls_validation_failed"
	OpenFailTLSProtocolError    = "tls_protocol_error"
	OpenFailCancelled           = "cancelled"
	OpenFailInternal            = "internal"
)

// TransportType identifies how the client connects to the destination.
type TransportType string

const (
	TransportTCP TransportType = "tcp"
	TransportTLS TransportType = "tls"
)

// TransportRequest describes the destination connection requested by open.
type TransportRequest struct {
	Type          TransportType `json:"type"`
	ServerName    string        `json:"serverName,omitempty"`
	ALPNProtocols []string      `json:"alpnProtocols,omitempty"`
}

// TransportResult reports the destination connection established by openOk.
// Timing fields are optional because not every dial path can measure them.
type TransportResult struct {
	Type          TransportType `json:"type"`
	ALPNProtocol  string        `json:"alpnProtocol,omitempty"`
	RemoteAddress string        `json:"remoteAddress,omitempty"`
	DNSMs         *int64        `json:"dnsMs,omitempty"`
	ConnectMs     *int64        `json:"connectMs,omitempty"`
	TLSMs         *int64        `json:"tlsMs,omitempty"`
}

// TCPTransport requests an opaque TCP stream.
func TCPTransport() TransportRequest {
	return TransportRequest{Type: TransportTCP}
}

// TCPTransportResult reports an opaque TCP stream when no dial diagnostics
// are available.
func TCPTransportResult() TransportResult {
	return TransportResult{Type: TransportTCP}
}

// ResetReason values mirror DestinationTunnelResetReason.
const (
	ResetCancelled         = "cancelled"
	ResetConnectionError   = "connection_error"
	ResetProtocolError     = "protocol_error"
	ResetResourceExhausted = "resource_exhausted"
	ResetInternal          = "internal"
)

// SessionErrorCode values mirror DestinationTunnelSessionErrorCode.
const (
	SessionErrorUnsupportedVersion = "unsupported_version"
	SessionErrorInvalidMessage     = "invalid_message"
	SessionErrorInvalidSelector    = "invalid_selector"
	SessionErrorAlreadyActive      = "already_active"
	SessionErrorUnavailable        = "unavailable"
	SessionErrorInternal           = "internal"
)

// ClientMessage is one decoded control message from the tunnel client. Type
// determines which of the remaining fields are meaningful.
type ClientMessage struct {
	Type string

	// start
	Version        int
	SelectorValues []string
	Inspection     InspectionConfig
	// Name is the private tunnel a connector serves; empty for a tunnel
	// started for one instance. The pod only echoes it.
	Name string
	// SkipsInactivityReset is set when start carried
	// resetsInactivityTimeout false: relayed data then does not count as
	// instance activity.
	SkipsInactivityReset bool

	// start and openOk: per-flow receive window granted by the client.
	Window int

	// openOk, openFail, fin, reset, windowUpdate
	ConnID uint32

	// openOk
	TransportResult TransportResult

	// openFail and reset
	Reason string
	OSCode string

	// windowUpdate
	Increment int
}

// ServerMessage is one control message sent to the tunnel client.
type ServerMessage struct {
	Type string

	// ready
	Version    int
	TunnelID   string
	Selectors  []SelectorReport
	ConfigHash string
	Name       string

	// open
	ConnID     uint32
	SelectorID string
	Host       string
	Port       int
	Window     int
	Transport  TransportRequest

	// reset
	Reason string
	OSCode string

	// windowUpdate
	Increment int

	// error
	Code string
}

// DecodeClientMessage parses and validates one text control message from
// the client. Selector contents of start messages are NOT canonicalized
// here; sessions validate them with product policy and reply with a typed
// session error rather than a bare protocol error.
func DecodeClientMessage(data []byte) (ClientMessage, error) {
	record, err := readRecord(data)
	if err != nil {
		return ClientMessage{}, err
	}
	msgType, err := readString(record, "type")
	if err != nil {
		return ClientMessage{}, err
	}
	switch msgType {
	case "start":
		version, err := readInteger(record, "version")
		if err != nil {
			return ClientMessage{}, err
		}
		selectors, err := readStartSelectors(record)
		if err != nil {
			return ClientMessage{}, err
		}
		window, err := readRequiredWindow(record)
		if err != nil {
			return ClientMessage{}, err
		}
		inspection, err := readInspectionConfig(record)
		if err != nil {
			return ClientMessage{}, err
		}
		name, err := readOptionalString(record, "name")
		if err != nil {
			return ClientMessage{}, err
		}
		resetsInactivityTimeout, err := readOptionalBool(record, "resetsInactivityTimeout", true)
		if err != nil {
			return ClientMessage{}, err
		}
		return ClientMessage{
			Type: msgType, Version: int(version), SelectorValues: selectors,
			Inspection: inspection, Window: window, Name: name,
			SkipsInactivityReset: !resetsInactivityTimeout,
		}, nil
	case "openOk":
		connID, err := readConnectionID(record)
		if err != nil {
			return ClientMessage{}, err
		}
		window, err := readRequiredWindow(record)
		if err != nil {
			return ClientMessage{}, err
		}
		transport, err := readTransportResult(record)
		if err != nil {
			return ClientMessage{}, err
		}
		return ClientMessage{
			Type: msgType, ConnID: connID, Window: window,
			TransportResult: transport,
		}, nil
	case "fin":
		connID, err := readConnectionID(record)
		if err != nil {
			return ClientMessage{}, err
		}
		return ClientMessage{Type: msgType, ConnID: connID}, nil
	case "openFail", "reset":
		connID, err := readConnectionID(record)
		if err != nil {
			return ClientMessage{}, err
		}
		reason, err := readString(record, "reason")
		if err != nil {
			return ClientMessage{}, err
		}
		osCode, err := readOptionalString(record, "osCode")
		if err != nil {
			return ClientMessage{}, err
		}
		return ClientMessage{Type: msgType, ConnID: connID, Reason: reason, OSCode: osCode}, nil
	case "windowUpdate":
		connID, err := readConnectionID(record)
		if err != nil {
			return ClientMessage{}, err
		}
		increment, err := readWindowIncrement(record)
		if err != nil {
			return ClientMessage{}, err
		}
		return ClientMessage{Type: msgType, ConnID: connID, Increment: increment}, nil
	default:
		return ClientMessage{}, protocolErrorf("unknown tunnel control message type %s", msgType)
	}
}

// EncodeServerMessage serializes one control message for the client.
func EncodeServerMessage(message ServerMessage) ([]byte, error) {
	switch message.Type {
	case "ready":
		if message.ConfigHash == "" {
			return nil, protocolErrorf("configHash must be a non-empty string")
		}
		payload := map[string]any{
			"type":       message.Type,
			"version":    message.Version,
			"tunnelId":   message.TunnelID,
			"configHash": message.ConfigHash,
		}
		if message.Selectors != nil {
			payload["selectors"] = message.Selectors
		}
		if message.Name != "" {
			payload["name"] = message.Name
		}
		return json.Marshal(payload)
	case "open":
		if message.Port < 1 || message.Port > 65535 {
			return nil, protocolErrorf("port must be between 1 and 65535")
		}
		transport, err := normalizeTransportRequest(message.Transport)
		if err != nil {
			return nil, err
		}
		payload := map[string]any{
			"type":       message.Type,
			"connId":     message.ConnID,
			"selectorId": message.SelectorID,
			"host":       message.Host,
			"port":       message.Port,
			"transport":  transportRequestPayload(transport),
		}
		if message.Window < 1 || message.Window > MaxWindow {
			return nil, protocolErrorf("window must be an integer between 1 and %d", MaxWindow)
		}
		payload["window"] = message.Window
		return json.Marshal(payload)
	case "fin":
		return json.Marshal(map[string]any{"type": message.Type, "connId": message.ConnID})
	case "reset":
		payload := map[string]any{
			"type":   message.Type,
			"connId": message.ConnID,
			"reason": message.Reason,
		}
		if message.OSCode != "" {
			payload["osCode"] = message.OSCode
		}
		return json.Marshal(payload)
	case "windowUpdate":
		if message.Increment < 1 || message.Increment > MaxWindowIncrement {
			return nil, protocolErrorf("increment must be an integer between 1 and %d", MaxWindowIncrement)
		}
		return json.Marshal(map[string]any{
			"type":      message.Type,
			"connId":    message.ConnID,
			"increment": message.Increment,
		})
	case "error":
		return json.Marshal(map[string]any{"type": message.Type, "code": message.Code})
	default:
		return nil, protocolErrorf("unknown tunnel control message type %s", message.Type)
	}
}

func transportRequestPayload(transport TransportRequest) map[string]any {
	payload := map[string]any{"type": transport.Type}
	if transport.Type == TransportTLS {
		protocols := transport.ALPNProtocols
		if protocols == nil {
			protocols = []string{}
		}
		payload["serverName"] = transport.ServerName
		payload["alpnProtocols"] = protocols
	}
	return payload
}

func normalizeTransportRequest(transport TransportRequest) (TransportRequest, error) {
	switch transport.Type {
	case TransportTCP:
		return TCPTransport(), nil
	case TransportTLS:
		if transport.ServerName == "" {
			return TransportRequest{}, protocolErrorf("TLS transport requires serverName")
		}
		for _, protocol := range transport.ALPNProtocols {
			if protocol == "" {
				return TransportRequest{}, protocolErrorf("alpnProtocols must contain non-empty strings")
			}
		}
		return transport, nil
	default:
		return TransportRequest{}, protocolErrorf("unsupported tunnel transport %s", transport.Type)
	}
}

func readInspectionConfig(record map[string]json.RawMessage) (InspectionConfig, error) {
	raw, ok := record["inspection"]
	if !ok {
		return InspectionConfig{}, protocolErrorf("inspection must be an object")
	}
	configRecord, err := readRecord(raw)
	if err != nil {
		return InspectionConfig{}, protocolErrorf("inspection must be an object")
	}
	enabled, err := readBool(configRecord, "enabled")
	if err != nil {
		return InspectionConfig{}, err
	}
	captureBodies, err := readBool(configRecord, "captureBodies")
	if err != nil {
		return InspectionConfig{}, err
	}
	maxBodyBytes, err := readInteger(configRecord, "maxBodyBytes")
	if err != nil {
		return InspectionConfig{}, err
	}
	if maxBodyBytes < 1 || maxBodyBytes > MaxBodyBytes {
		return InspectionConfig{}, protocolErrorf("maxBodyBytes must be between 1 and %d", MaxBodyBytes)
	}
	persist, err := readBool(configRecord, "persist")
	if err != nil {
		return InspectionConfig{}, err
	}
	ttlSeconds, err := readInteger(configRecord, "ttlSeconds")
	if err != nil {
		return InspectionConfig{}, err
	}
	if ttlSeconds < 1 || ttlSeconds > MaxInspectionTTLSeconds {
		return InspectionConfig{}, protocolErrorf("ttlSeconds must be between 1 and %d", MaxInspectionTTLSeconds)
	}
	return NormalizeInspectionConfig(InspectionConfig{
		Enabled: enabled, CaptureBodies: captureBodies, MaxBodyBytes: int(maxBodyBytes),
		Persist: persist, TTLSeconds: int(ttlSeconds),
	})
}

func readTransportResult(record map[string]json.RawMessage) (TransportResult, error) {
	raw, ok := record["transport"]
	if !ok {
		return TransportResult{}, protocolErrorf("transport must be an object")
	}
	transportRecord, err := readRecord(raw)
	if err != nil {
		return TransportResult{}, protocolErrorf("transport must be an object")
	}
	transportType, err := readString(transportRecord, "type")
	if err != nil {
		return TransportResult{}, err
	}
	result := TransportResult{Type: TransportType(transportType)}
	if result.Type != TransportTCP && result.Type != TransportTLS {
		return TransportResult{}, protocolErrorf("unsupported tunnel transport %s", result.Type)
	}
	if result.ALPNProtocol, err = readOptionalString(transportRecord, "alpnProtocol"); err != nil {
		return TransportResult{}, err
	}
	if result.RemoteAddress, err = readOptionalString(transportRecord, "remoteAddress"); err != nil {
		return TransportResult{}, err
	}
	if result.DNSMs, err = readOptionalNonNegativeInteger(transportRecord, "dnsMs"); err != nil {
		return TransportResult{}, err
	}
	if result.ConnectMs, err = readOptionalNonNegativeInteger(transportRecord, "connectMs"); err != nil {
		return TransportResult{}, err
	}
	if result.TLSMs, err = readOptionalNonNegativeInteger(transportRecord, "tlsMs"); err != nil {
		return TransportResult{}, err
	}
	return result, nil
}

// EncodeDataFrame builds one binary frame: u32 big-endian connId || payload.
func EncodeDataFrame(connID uint32, payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, protocolErrorf("tunnel data frame must include a payload")
	}
	frame := make([]byte, ConnIDHeaderBytes+len(payload))
	binary.BigEndian.PutUint32(frame, connID)
	copy(frame[ConnIDHeaderBytes:], payload)
	return frame, nil
}

// DecodeDataFrame splits one binary frame into its connection ID and
// payload. The payload aliases the input slice.
func DecodeDataFrame(frame []byte) (uint32, []byte, error) {
	if len(frame) <= ConnIDHeaderBytes {
		return 0, nil, protocolErrorf("tunnel data frame must include a payload")
	}
	return binary.BigEndian.Uint32(frame), frame[ConnIDHeaderBytes:], nil
}

func readStartSelectors(record map[string]json.RawMessage) ([]string, error) {
	return readStringArray(record, "selectors", "tunnel selector")
}

func readRecord(data []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var record map[string]json.RawMessage
	if err := decoder.Decode(&record); err != nil || record == nil {
		return nil, protocolErrorf("tunnel control message must be an object")
	}
	return record, nil
}

func readStringArray(record map[string]json.RawMessage, key, name string) ([]string, error) {
	raw, ok := record[key]
	if !ok {
		return nil, nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, protocolErrorf("%s must be an array", key)
	}
	values := make([]string, 0, len(entries))
	for _, entry := range entries {
		var value string
		if err := json.Unmarshal(entry, &value); err != nil {
			return nil, protocolErrorf("%s must be a string", name)
		}
		values = append(values, value)
	}
	return values, nil
}

func readString(record map[string]json.RawMessage, key string) (string, error) {
	raw, ok := record[key]
	if !ok {
		return "", protocolErrorf("%s must be a string", key)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", protocolErrorf("%s must be a string", key)
	}
	return value, nil
}

func readOptionalString(record map[string]json.RawMessage, key string) (string, error) {
	raw, ok := record[key]
	if !ok {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", protocolErrorf("%s must be a string", key)
	}
	return value, nil
}

func readBool(record map[string]json.RawMessage, key string) (bool, error) {
	raw, ok := record[key]
	if !ok {
		return false, protocolErrorf("%s must be a boolean", key)
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, protocolErrorf("%s must be a boolean", key)
	}
	return value, nil
}

// readOptionalBool returns fallback when key is absent or null, like
// readOptionalString treats them.
func readOptionalBool(record map[string]json.RawMessage, key string, fallback bool) (bool, error) {
	if raw, ok := record[key]; !ok || string(raw) == "null" {
		return fallback, nil
	}
	return readBool(record, key)
}

func readOptionalNonNegativeInteger(record map[string]json.RawMessage, key string) (*int64, error) {
	if _, ok := record[key]; !ok {
		return nil, nil
	}
	value, err := readInteger(record, key)
	if err != nil || value < 0 {
		return nil, protocolErrorf("%s must be a non-negative integer", key)
	}
	return &value, nil
}

func readInteger(record map[string]json.RawMessage, key string) (int64, error) {
	raw, ok := record[key]
	if !ok {
		return 0, protocolErrorf("%s must be an integer", key)
	}
	var value json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return 0, protocolErrorf("%s must be an integer", key)
	}
	parsed, err := value.Int64()
	if err != nil {
		// Reject floats including integral ones with a fraction/exponent
		// notation the same way Number.isInteger accepts 1.0; JSON never
		// distinguishes 1 from 1.0, so accept integral floats.
		float, ferr := value.Float64()
		if ferr != nil || float != math.Trunc(float) || math.IsInf(float, 0) {
			return 0, protocolErrorf("%s must be an integer", key)
		}
		if float > math.MaxInt64 || float < math.MinInt64 {
			return 0, protocolErrorf("%s must be an integer", key)
		}
		return int64(float), nil
	}
	return parsed, nil
}

func readPort(record map[string]json.RawMessage, key string) (int, error) {
	value, err := readInteger(record, key)
	if err != nil {
		return 0, err
	}
	if value < 1 || value > 65535 {
		return 0, protocolErrorf("%s must be between 1 and 65535", key)
	}
	return int(value), nil
}

func readRequiredWindow(record map[string]json.RawMessage) (int, error) {
	value, err := readInteger(record, "window")
	if err != nil {
		return 0, protocolErrorf("window must be an integer between 1 and %d", MaxWindow)
	}
	if value < 1 || value > MaxWindow {
		return 0, protocolErrorf("window must be an integer between 1 and %d", MaxWindow)
	}
	return int(value), nil
}

func readWindowIncrement(record map[string]json.RawMessage) (int, error) {
	value, err := readInteger(record, "increment")
	if err != nil {
		return 0, err
	}
	if value < 1 || value > MaxWindowIncrement {
		return 0, protocolErrorf("increment must be an integer between 1 and %d", MaxWindowIncrement)
	}
	return int(value), nil
}

func readConnectionID(record map[string]json.RawMessage) (uint32, error) {
	value, err := readInteger(record, "connId")
	if err != nil {
		return 0, protocolErrorf("connId must be an unsigned 32-bit integer")
	}
	if value < 0 || value > math.MaxUint32 {
		return 0, protocolErrorf("connId must be an unsigned 32-bit integer")
	}
	return uint32(value), nil
}
