package tunnel

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

type HTTPProxyOptions struct {
	LocalPort     int
	RemoteBaseURL string
	Headers       http.Header
}
type ForwardHTTPProxyOptions struct {
	MatchPort     int
	RemoteBaseURL string
	Headers       http.Header
}
type HTTPProxy struct {
	Port      int
	server    *http.Server
	transport *http.Transport
	once      sync.Once
}

func (p *HTTPProxy) Close() error {
	var err error
	p.once.Do(func() { err = p.server.Close(); p.transport.CloseIdleConnections() })
	return err
}
func StartHTTPProxy(opts HTTPProxyOptions) (*HTTPProxy, error) {
	return startHTTPProxy(opts.LocalPort, 0, opts.RemoteBaseURL, opts.Headers)
}
func StartForwardHTTPProxy(opts ForwardHTTPProxyOptions) (*HTTPProxy, error) {
	return startHTTPProxy(0, opts.MatchPort, opts.RemoteBaseURL, opts.Headers)
}
func startHTTPProxy(port, matchPort int, base string, headers http.Header) (*HTTPProxy, error) {
	if _, err := url.ParseRequestURI(base); err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{}
	if defaults, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = defaults.Clone()
	}
	transport.Proxy = nil
	p := &HTTPProxy{Port: listener.Addr().(*net.TCPAddr).Port, transport: transport}
	p.server = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := r.URL
		if matchPort > 0 {
			if r.Method == "CONNECT" {
				http.Error(w, "Limrun forward proxy does not tunnel CONNECT.", 501)
				return
			}
			if !target.IsAbs() || (target.Scheme != "http" && target.Scheme != "https") {
				http.Error(w, "Limrun forward proxy only accepts absolute-form request targets.", 400)
				return
			}
			n, _ := strconv.Atoi(target.Port())
			host := target.Hostname()
			ip := net.ParseIP(host)
			loopback := host == "localhost" || host == "::1" || ip != nil && ip.To4() != nil && ip.To4()[0] == 127
			if !loopback || n != matchPort {
				http.Error(w, fmt.Sprintf("Limrun forward proxy only forwards 127.0.0.1:%d.", matchPort), 403)
				return
			}
		}
		path := target.EscapedPath()
		if path == "" {
			path = "/"
		}
		if target.RawQuery != "" {
			path += "?" + target.RawQuery
		}
		upstream, err := url.Parse(strings.TrimRight(base, "/") + path)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		request := r.Clone(r.Context())
		request.URL = upstream
		request.Host = upstream.Host
		request.RequestURI = ""
		request.Header = r.Header.Clone()
		if matchPort > 0 {
			dropHopHeaders(request.Header)
		}
		for k, v := range headers {
			request.Header[k] = append([]string(nil), v...)
		}
		response, err := transport.RoundTrip(request)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		defer response.Body.Close()
		if matchPort > 0 {
			dropHopHeaders(response.Header)
		}
		for key, values := range response.Header {
			w.Header()[key] = values
		}
		w.WriteHeader(response.StatusCode)
		if _, err = io.Copy(w, response.Body); err != nil {
			panic(http.ErrAbortHandler)
		}
	})}
	go p.server.Serve(listener)
	return p, nil
}
func dropHopHeaders(headers http.Header) {
	for _, key := range []string{"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authorization", "Proxy-Authenticate", "Te", "Trailer", "Upgrade"} {
		headers.Del(key)
	}
}
