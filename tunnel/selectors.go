// Package tunnel connects local services and tools to Limrun instances.
package tunnel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/idna"
)

const (
	// Version is the only protocol version this implementation speaks.
	Version = 1
	// MaxRoutes bounds the exact routes accepted in one start message.
	MaxRoutes = 10
	// MaxDomains bounds the domain selectors accepted in one start message.
	MaxDomains = 64
	// ConnIDHeaderBytes is the length of the connection ID prefix of every
	// binary data frame.
	ConnIDHeaderBytes = 4
	// FakeRange is the IPv4 range used for synthetic DNS answers of matched
	// domain selectors.
	FakeRange = "198.18.0.0/15"
	// MaxWindow is the upper bound for per-flow receive windows advertised
	// in open/openOk.
	MaxWindow = 64 * 1024 * 1024
	// MaxWindowIncrement is the upper bound for a single windowUpdate.
	MaxWindowIncrement = 0x7fff_ffff
	// DefaultMaxBodyBytes is the default per-request and per-response body
	// capture limit used by inspection.
	DefaultMaxBodyBytes = 10 * 1024 * 1024
	// MaxBodyBytes is the hard upper bound for inspection body capture.
	MaxBodyBytes = 64 * 1024 * 1024
	// DefaultInspectionTTLSeconds is the default lifetime of a persisted HAR.
	DefaultInspectionTTLSeconds = 72 * 60 * 60
	// MaxInspectionTTLSeconds is the longest permitted persisted HAR lifetime.
	MaxInspectionTTLSeconds = 30 * 24 * 60 * 60
)

// Route is one exact host:port destination selector.
type Route struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// Selectors is the server's internal classification of public selector values.
// Routes use bind listeners; domains use fake-IP transparent interception.
type Selectors struct {
	Entries []SelectorEntry `json:"-"`
	Routes  []Route         `json:"routes,omitempty"`
	Domains []string        `json:"domains,omitempty"`
}

// SelectorEntry preserves one public selector's position after classification.
type SelectorEntry struct {
	ID    string
	Kind  SelectorKind
	Value string
}

// InspectionConfig controls HTTP inspection and bounded body capture.
type InspectionConfig struct {
	Enabled       bool `json:"enabled"`
	CaptureBodies bool `json:"captureBodies"`
	MaxBodyBytes  int  `json:"maxBodyBytes"`
	Persist       bool `json:"persist"`
	TTLSeconds    int  `json:"ttlSeconds"`
}

// NormalizeInspectionConfig applies defaults and validates the inspection
// settings carried by start and status and covered by the canonical config hash.
func NormalizeInspectionConfig(config InspectionConfig) (InspectionConfig, error) {
	if config.CaptureBodies && !config.Enabled {
		return InspectionConfig{}, protocolErrorf("captureBodies requires inspection to be enabled")
	}
	if config.Persist && !config.Enabled {
		return InspectionConfig{}, protocolErrorf("persist requires inspection to be enabled")
	}
	if config.MaxBodyBytes == 0 {
		config.MaxBodyBytes = DefaultMaxBodyBytes
	}
	if config.MaxBodyBytes < 1 || config.MaxBodyBytes > MaxBodyBytes {
		return InspectionConfig{}, protocolErrorf("maxBodyBytes must be between 1 and %d", MaxBodyBytes)
	}
	if config.TTLSeconds == 0 {
		config.TTLSeconds = DefaultInspectionTTLSeconds
	}
	if config.TTLSeconds < 1 || config.TTLSeconds > MaxInspectionTTLSeconds {
		return InspectionConfig{}, protocolErrorf("ttlSeconds must be between 1 and %d", MaxInspectionTTLSeconds)
	}
	return config, nil
}

// DisabledInspection is the canonical configuration for an opaque raw tunnel.
func DisabledInspection() InspectionConfig {
	config, _ := NormalizeInspectionConfig(InspectionConfig{})
	return config
}

// SelectorKind identifies the selector family of a selector ID.
type SelectorKind string

const (
	SelectorKindRoute  SelectorKind = "route"
	SelectorKindDomain SelectorKind = "domain"
)

// BindStatus is the outcome of one bind-listener address.
type BindStatus string

const (
	BindStatusOK       BindStatus = "ok"
	BindStatusConflict BindStatus = "conflict"
	BindStatusError    BindStatus = "error"
)

// BindReport is the per-address bind outcome reported for exact routes.
type BindReport struct {
	Address string     `json:"address"`
	Status  BindStatus `json:"status"`
	OSCode  string     `json:"osCode,omitempty"`
}

// SelectorReport is one normalized selector acknowledged in ready and status.
type SelectorReport struct {
	ID    string       `json:"id"`
	Kind  SelectorKind `json:"kind"`
	Value string       `json:"value"`
	Binds []BindReport `json:"binds,omitempty"`
}

// SelectorErrorCode mirrors DestinationTunnelSelectorErrorCode in the
// TypeScript authority.
type SelectorErrorCode string

const (
	SelectorErrorEmpty         SelectorErrorCode = "empty"
	SelectorErrorTooMany       SelectorErrorCode = "too_many"
	SelectorErrorInvalidHost   SelectorErrorCode = "invalid_host"
	SelectorErrorInvalidPort   SelectorErrorCode = "invalid_port"
	SelectorErrorInvalidDomain SelectorErrorCode = "invalid_domain"
	SelectorErrorDuplicate     SelectorErrorCode = "duplicate"
)

// SelectorError reports why a selector set was rejected.
type SelectorError struct {
	Code    SelectorErrorCode
	Message string
}

func (e *SelectorError) Error() string { return e.Message }

func selectorErrorf(code SelectorErrorCode, format string, args ...any) error {
	return &SelectorError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// ValidateOptions carries product-specific selector policy.
type ValidateOptions struct {
	// MinRoutePort rejects exact routes below this port. Android bind
	// listeners use 1024; zero means the protocol default of 1.
	MinRoutePort int
}

// ParseSelectors classifies and canonicalizes public selector values. Exact
// host:port values become bind-listener routes; values without a port become
// exact or wildcard domain selectors for transparent interception.
func ParseSelectors(values []string, options ValidateOptions) (Selectors, error) {
	if len(values) == 0 {
		return Selectors{}, selectorErrorf(SelectorErrorEmpty, "at least one tunnel selector is required")
	}
	selectors := Selectors{}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			return Selectors{}, selectorErrorf(SelectorErrorEmpty, "tunnel selector must not be empty")
		}
		var canonical string
		var kind SelectorKind
		if !strings.HasPrefix(value, "[") && !strings.Contains(value, ":") {
			domain, err := canonicalizeDomain(value)
			if err != nil {
				return Selectors{}, err
			}
			if len(selectors.Domains) >= MaxDomains {
				return Selectors{}, selectorErrorf(SelectorErrorTooMany, "at most %d domain tunnel selectors are allowed", MaxDomains)
			}
			canonical = domain
			kind = SelectorKindDomain
			selectors.Domains = append(selectors.Domains, domain)
		} else {
			host, portText, err := net.SplitHostPort(value)
			if err != nil {
				return Selectors{}, selectorErrorf(SelectorErrorInvalidHost, "invalid tunnel selector %s", value)
			}
			if portText == "" || strings.IndexFunc(portText, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
				return Selectors{}, selectorErrorf(SelectorErrorInvalidPort, "invalid tunnel selector port %s", portText)
			}
			port, err := strconv.Atoi(portText)
			if err != nil {
				return Selectors{}, selectorErrorf(SelectorErrorInvalidPort, "invalid tunnel selector port %s", portText)
			}
			route, err := canonicalizeRoute(Route{Host: host, Port: port}, options.MinRoutePort)
			if err != nil {
				return Selectors{}, err
			}
			if len(selectors.Routes) >= MaxRoutes {
				return Selectors{}, selectorErrorf(SelectorErrorTooMany, "at most %d exact tunnel selectors are allowed", MaxRoutes)
			}
			canonical = formatRoute(route)
			kind = SelectorKindRoute
			selectors.Routes = append(selectors.Routes, route)
		}
		if _, duplicate := seen[canonical]; duplicate {
			return Selectors{}, selectorErrorf(SelectorErrorDuplicate, "duplicate tunnel selector %s", canonical)
		}
		seen[canonical] = struct{}{}
		selectors.Entries = append(selectors.Entries, SelectorEntry{
			ID:    fmt.Sprintf("selector-%d", len(selectors.Entries)+1),
			Kind:  kind,
			Value: canonical,
		})
	}
	return selectors, nil
}

// FormatSelectors returns the canonical public values in declaration order.
func FormatSelectors(selectors Selectors) []string {
	if len(selectors.Entries) > 0 {
		values := make([]string, 0, len(selectors.Entries))
		for _, entry := range selectors.Entries {
			values = append(values, entry.Value)
		}
		return values
	}
	values := make([]string, 0, len(selectors.Routes)+len(selectors.Domains))
	for _, route := range selectors.Routes {
		values = append(values, formatRoute(route))
	}
	return append(values, selectors.Domains...)
}

// ValidateSelectors validates and canonicalizes a full selector set. At
// least one selector of any kind is required.
func ValidateSelectors(selectors Selectors, options ValidateOptions) (Selectors, error) {
	if len(selectors.Entries) > 0 {
		return ParseSelectors(FormatSelectors(selectors), options)
	}
	if len(selectors.Routes) == 0 && len(selectors.Domains) == 0 {
		return Selectors{}, selectorErrorf(SelectorErrorEmpty, "at least one tunnel selector is required")
	}
	canonical := Selectors{}
	if len(selectors.Routes) > 0 {
		routes, err := ValidateRoutes(selectors.Routes, options.MinRoutePort)
		if err != nil {
			return Selectors{}, err
		}
		canonical.Routes = routes
	}
	if len(selectors.Domains) > 0 {
		domains, err := ValidateDomains(selectors.Domains)
		if err != nil {
			return Selectors{}, err
		}
		canonical.Domains = domains
	}
	return canonical, nil
}

// ValidateRoutes validates and canonicalizes exact routes. minPort of zero
// means the protocol default of 1.
func ValidateRoutes(routes []Route, minPort int) ([]Route, error) {
	if len(routes) == 0 {
		return nil, selectorErrorf(SelectorErrorEmpty, "at least one tunnel route is required")
	}
	if len(routes) > MaxRoutes {
		return nil, selectorErrorf(SelectorErrorTooMany, "at most %d tunnel routes are allowed", MaxRoutes)
	}
	canonical := make([]Route, 0, len(routes))
	seen := make(map[Route]struct{}, len(routes))
	for _, route := range routes {
		c, err := canonicalizeRoute(route, minPort)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[c]; dup {
			return nil, selectorErrorf(SelectorErrorDuplicate, "duplicate tunnel route %s:%d", c.Host, c.Port)
		}
		seen[c] = struct{}{}
		canonical = append(canonical, c)
	}
	return canonical, nil
}

// ValidateDomains validates and canonicalizes domain selectors.
func ValidateDomains(domains []string) ([]string, error) {
	if len(domains) > MaxDomains {
		return nil, selectorErrorf(SelectorErrorTooMany, "at most %d tunnel domains are allowed", MaxDomains)
	}
	canonical := make([]string, 0, len(domains))
	seen := make(map[string]struct{}, len(domains))
	for _, domain := range domains {
		normalized, err := canonicalizeDomain(domain)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[normalized]; dup {
			return nil, selectorErrorf(SelectorErrorDuplicate, "duplicate tunnel domain %s", normalized)
		}
		seen[normalized] = struct{}{}
		canonical = append(canonical, normalized)
	}
	return canonical, nil
}

// ConfigHash returns the canonical hash for an opaque raw tunnel. It keeps
// existing raw callers concise while including disabled inspection settings.
func ConfigHash(selectors Selectors) (string, error) {
	return ConfigHashWithInspection(selectors, DisabledInspection())
}

// ConfigHashWithInspection returns the deterministic SHA-256 of the canonical
// selector and inspection configuration. The JSON shape and key order must
// match the TypeScript implementation byte for byte.
func ConfigHashWithInspection(selectors Selectors, inspection InspectionConfig) (string, error) {
	canonical, err := ValidateSelectors(selectors, ValidateOptions{})
	if err != nil {
		return "", err
	}
	inspection, err = NormalizeInspectionConfig(inspection)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(`{"version":`)
	b.WriteString(strconv.Itoa(Version))
	b.WriteString(`,"selectors":[`)
	if err := writeJSONStrings(&b, FormatSelectors(canonical)); err != nil {
		return "", err
	}
	b.WriteByte(']')
	b.WriteString(`,"inspection":{"enabled":`)
	b.WriteString(strconv.FormatBool(inspection.Enabled))
	b.WriteString(`,"captureBodies":`)
	b.WriteString(strconv.FormatBool(inspection.CaptureBodies))
	b.WriteString(`,"maxBodyBytes":`)
	b.WriteString(strconv.Itoa(inspection.MaxBodyBytes))
	b.WriteString(`,"persist":`)
	b.WriteString(strconv.FormatBool(inspection.Persist))
	b.WriteString(`,"ttlSeconds":`)
	b.WriteString(strconv.Itoa(inspection.TTLSeconds))
	b.WriteByte('}')
	b.WriteByte('}')
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:]), nil
}

func writeJSONStrings(b *strings.Builder, values []string) error {
	for i, value := range values {
		if i > 0 {
			b.WriteByte(',')
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		b.Write(encoded)
	}
	return nil
}

// SelectorIDs returns dense one-based selector-N IDs for a
// canonical selector set, in report order.
func SelectorIDs(selectors Selectors) []string {
	reports := SelectorReports(selectors)
	ids := make([]string, 0, len(reports))
	for _, report := range reports {
		ids = append(ids, report.ID)
	}
	return ids
}

// SelectorReports returns the ready/status selector reports for a canonical
// selector set, without bind outcomes.
func SelectorReports(selectors Selectors) []SelectorReport {
	if len(selectors.Entries) > 0 {
		reports := make([]SelectorReport, 0, len(selectors.Entries))
		for _, entry := range selectors.Entries {
			reports = append(reports, SelectorReport{
				ID:    entry.ID,
				Kind:  entry.Kind,
				Value: entry.Value,
			})
		}
		return reports
	}
	reports := make([]SelectorReport, 0, len(selectors.Routes)+len(selectors.Domains))
	for i, route := range selectors.Routes {
		reports = append(reports, SelectorReport{
			ID:    fmt.Sprintf("selector-%d", i+1),
			Kind:  SelectorKindRoute,
			Value: formatRoute(route),
		})
	}
	for i, domain := range selectors.Domains {
		reports = append(reports, SelectorReport{
			ID:    fmt.Sprintf("selector-%d", len(selectors.Routes)+i+1),
			Kind:  SelectorKindDomain,
			Value: domain,
		})
	}
	return reports
}

// RouteSelectorID returns the public ID for one classified exact route.
func RouteSelectorID(selectors Selectors, index int) string {
	seen := 0
	for _, entry := range selectors.Entries {
		if entry.Kind != SelectorKindRoute {
			continue
		}
		if seen == index {
			return entry.ID
		}
		seen++
	}
	return fmt.Sprintf("selector-%d", index+1)
}

// DomainSelectorID returns the public ID for one classified domain.
func DomainSelectorID(selectors Selectors, index int) string {
	seen := 0
	for _, entry := range selectors.Entries {
		if entry.Kind != SelectorKindDomain {
			continue
		}
		if seen == index {
			return entry.ID
		}
		seen++
	}
	return fmt.Sprintf("selector-%d", len(selectors.Routes)+index+1)
}

// DomainMatches reports whether a canonical domain selector covers host.
// Wildcards are label-bound: *.example.com covers a.example.com but not
// example.com itself.
func DomainMatches(pattern, host string) bool {
	candidate := strings.ToLower(host)
	if base, ok := strings.CutPrefix(pattern, "*."); ok {
		return len(candidate) > len(base)+1 && strings.HasSuffix(candidate, "."+base)
	}
	return candidate == pattern
}

// MatchDomain returns the one-based index of the first domain selector
// covering host, or zero when none does.
func MatchDomain(domains []string, host string) int {
	for i, domain := range domains {
		if DomainMatches(domain, host) {
			return i + 1
		}
	}
	return 0
}

func formatRoute(route Route) string {
	return net.JoinHostPort(route.Host, strconv.Itoa(route.Port))
}

func canonicalizeRoute(route Route, minPort int) (Route, error) {
	if minPort < 1 {
		minPort = 1
	}
	if route.Port < minPort || route.Port > 65535 || route.Port == 53 {
		return Route{}, selectorErrorf(SelectorErrorInvalidPort, "invalid tunnel route port %d", route.Port)
	}
	if isASCII(route.Host) && strings.EqualFold(route.Host, "localhost") {
		return Route{Host: "localhost", Port: route.Port}, nil
	}
	addr, err := netip.ParseAddr(route.Host)
	if err != nil || addr.Zone() != "" {
		return Route{}, selectorErrorf(SelectorErrorInvalidHost, "invalid tunnel route host %s", route.Host)
	}
	if addr.Is4() {
		return Route{Host: addr.String(), Port: route.Port}, nil
	}
	if addr.Is4In6() {
		// IPv4-mapped IPv6 canonicalizes to the embedded IPv4 address.
		return Route{Host: addr.Unmap().String(), Port: route.Port}, nil
	}
	canonical := addr.String()
	if canonical != "::1" && ipv4CompatiblePattern.MatchString(canonical) {
		// Deprecated IPv4-compatible IPv6 (::a.b.c.d and friends) is
		// rejected rather than silently treated as IPv6.
		return Route{}, selectorErrorf(SelectorErrorInvalidHost, "invalid tunnel route host %s", route.Host)
	}
	return Route{Host: canonical, Port: route.Port}, nil
}

var ipv4CompatiblePattern = regexp.MustCompile(`^::(?:[0-9a-f]{1,4}:)?[0-9a-f]{1,4}$`)

var domainLabelPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

var allDigitsPattern = regexp.MustCompile(`^\d+$`)

// domainProfile validates ASCII domains the same way Node's domainToASCII
// does for the subset the label regex already permits: it normalizes
// punycode (xn--) labels, which is the only transformation possible after
// the [a-z0-9-] restriction.
var domainProfile = idna.New(idna.MapForLookup(), idna.CheckHyphens(false), idna.StrictDomainName(false))

func canonicalizeDomain(domain string) (string, error) {
	invalid := func() (string, error) {
		return "", selectorErrorf(SelectorErrorInvalidDomain, "invalid tunnel domain %s", domain)
	}
	if len(domain) == 0 || len(domain) > 260 || !isASCII(domain) {
		return invalid()
	}
	lowered := strings.ToLower(domain)
	base, wildcard := strings.CutPrefix(lowered, "*.")
	if len(base) == 0 || len(base) > 253 {
		return invalid()
	}
	if strings.Contains(base, "*") {
		return invalid()
	}
	if strings.HasPrefix(base, ".") || strings.HasSuffix(base, ".") {
		return invalid()
	}
	if addr, err := netip.ParseAddr(base); err == nil && (addr.Is4() || addr.Is6()) {
		return invalid()
	}
	if base == "localhost" {
		return invalid()
	}
	if mapped, err := domainProfile.ToASCII(base); err != nil || mapped != base {
		return invalid()
	}
	labels := strings.Split(base, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || !domainLabelPattern.MatchString(label) {
			return invalid()
		}
	}
	// All-numeric final labels would be ambiguous with IPv4 literals.
	if allDigitsPattern.MatchString(labels[len(labels)-1]) {
		return invalid()
	}
	if wildcard {
		return "*." + base, nil
	}
	return base, nil
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}
