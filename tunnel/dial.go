package tunnel

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"syscall"

	"github.com/limrun-inc/go-sdk/internal/proxy"
)

type codedError struct{ code, message string }

func (e *codedError) Error() string { return e.message }
func osCode(err error) string {
	var proxyError *proxy.Error
	if errors.As(err, &proxyError) {
		return proxyError.Code
	}
	var coded *codedError
	if errors.As(err, &coded) {
		return coded.code
	}
	for _, pair := range []struct {
		value error
		name  string
	}{{syscall.ECONNREFUSED, "ECONNREFUSED"}, {syscall.ECONNRESET, "ECONNRESET"}, {syscall.ETIMEDOUT, "ETIMEDOUT"}, {syscall.ENETUNREACH, "ENETUNREACH"}, {syscall.EHOSTUNREACH, "EHOSTUNREACH"}, {syscall.EACCES, "EACCES"}, {syscall.EPERM, "EPERM"}, {syscall.EMFILE, "EMFILE"}, {syscall.ENFILE, "ENFILE"}, {syscall.ENOBUFS, "ENOBUFS"}, {syscall.ENOMEM, "ENOMEM"}} {
		if errors.Is(err, pair.value) {
			return pair.name
		}
	}
	if errors.Is(err, context.Canceled) {
		return "ECANCELED"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "ETIMEDOUT"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		if dns.IsTemporary || dns.IsTimeout {
			return "EAI_AGAIN"
		}
		return "ENOTFOUND"
	}
	return ""
}
func ClassifyOpenFailure(err error) string {
	switch osCode(err) {
	case "ENOTFOUND":
		return OpenFailDNSNotFound
	case "EAI_AGAIN":
		return OpenFailDNSTemporaryFailure
	case "ECONNREFUSED":
		return OpenFailConnectionRefused
	case "ECONNRESET":
		return OpenFailConnectionReset
	case "ETIMEDOUT":
		return OpenFailConnectionTimedOut
	case "ENETUNREACH", "EHOSTUNREACH":
		return OpenFailUnreachable
	case "EACCES", "EPERM", "EPROXYAUTH":
		return OpenFailPermissionDenied
	case "EMFILE", "ENFILE", "ENOBUFS", "ENOMEM":
		return OpenFailResourceExhausted
	case "ECANCELED":
		return OpenFailCancelled
	case "EBLOCKED":
		return OpenFailSelectorNotAllowed
	}
	var cert *tls.CertificateVerificationError
	var invalid x509.CertificateInvalidError
	var host x509.HostnameError
	var authority x509.UnknownAuthorityError
	if errors.As(err, &cert) || errors.As(err, &invalid) || errors.As(err, &host) || errors.As(err, &authority) {
		return OpenFailTLSValidationFailed
	}
	var record tls.RecordHeaderError
	if errors.As(err, &record) {
		return OpenFailTLSProtocolError
	}
	if strings.Contains(err.Error(), "tls:") {
		return OpenFailTLSHandshakeFailed
	}
	return OpenFailInternal
}

var blockedResolvedPrefixes = func() []netip.Prefix {
	var values []netip.Prefix
	for _, s := range []string{"0.0.0.0/8", "127.0.0.0/8", "169.254.0.0/16", "224.0.0.0/3", "::/96", "fe80::/10", "ff00::/8"} {
		values = append(values, netip.MustParsePrefix(s))
	}
	return values
}()

func IsDialableResolvedAddress(address string, port int, routes []Route) bool {
	for _, route := range routes {
		if route.Port == port && (route.Host == address || route.Host == "localhost" && (address == "127.0.0.1" || address == "::1")) {
			return true
		}
	}
	addr, err := netip.ParseAddr(address)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, p := range blockedResolvedPrefixes {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

func connectDestination(ctx context.Context, host, lookupHost string, port int, protocol string) (net.Conn, string, error) {
	endpoint, err := proxy.Destination(protocol, lookupHost, port)
	if err != nil {
		return nil, "", err
	}
	target := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := proxy.Connect(ctx, target, endpoint, nil)
	if err != nil {
		return nil, "", err
	}
	if endpoint != nil {
		return conn, host, nil
	}
	address, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
	return conn, address, nil
}
