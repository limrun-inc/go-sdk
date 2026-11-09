package tunnel

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

type bodyCapture struct {
	request, response                   []byte
	requestTruncated, responseTruncated bool
}
type HARAssembler struct {
	BodyLimit int
	mu        sync.Mutex
	pending   map[string]*bodyCapture
}

func NewHARAssembler(limit ...int) (*HARAssembler, error) {
	n := DefaultMaxBodyBytes
	if len(limit) > 0 {
		n = limit[0]
	}
	if n < 1 {
		return nil, errors.New("HAR body limit must be a positive integer")
	}
	return &HARAssembler{BodyLimit: n, pending: map[string]*bodyCapture{}}, nil
}
func (a *HARAssembler) Reset() { a.mu.Lock(); a.pending = map[string]*bodyCapture{}; a.mu.Unlock() }
func (a *HARAssembler) Add(event InspectionEvent) (HAREntry, error) {
	if event.Type != "body" && event.Type != "complete" {
		return nil, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	capture := a.pending[event.RequestID]
	if capture == nil {
		capture = &bodyCapture{}
		a.pending[event.RequestID] = capture
	}
	if event.Type == "body" {
		body, truncated := &capture.request, &capture.requestTruncated
		if event.Direction == "response" {
			body, truncated = &capture.response, &capture.responseTruncated
		}
		take := len(event.Body)
		if remaining := a.BodyLimit - len(*body); take > remaining {
			take = remaining
			*truncated = true
		}
		if take > 0 {
			*body = append(*body, event.Body[:take]...)
		}
		return nil, nil
	}
	defer delete(a.pending, event.RequestID)
	raw, err := json.Marshal(event.Complete)
	if err != nil {
		return nil, err
	}
	var complete HAREntry
	if err = json.Unmarshal(raw, &complete); err != nil {
		return nil, err
	}
	request, ok := complete["request"].(map[string]any)
	if !ok {
		return nil, errors.New("HAR request is missing")
	}
	response, ok := complete["response"].(map[string]any)
	if !ok {
		return nil, errors.New("HAR response is missing")
	}
	extension, ok := complete["_limrun"].(map[string]any)
	if !ok {
		return nil, errors.New("HAR extension is missing")
	}
	requestTruncated := capture.requestTruncated || extension["requestBodyTruncated"] == true
	responseTruncated := capture.responseTruncated || extension["responseBodyTruncated"] == true
	extension["requestBodyTruncated"] = requestTruncated
	extension["responseBodyTruncated"] = responseTruncated
	if len(capture.request) > 0 {
		mime := headerValue(request, "content-type")
		if post, ok := request["postData"].(map[string]any); ok {
			if v, ok := post["mimeType"].(string); ok {
				mime = v
			}
		}
		text, encoding, _ := encodedBody(capture.request, mime, headerValue(request, "content-encoding"), requestTruncated)
		request["postData"] = map[string]any{"mimeType": mime, "text": text}
		if encoding != "" {
			extension["requestBodyEncoding"] = encoding
		}
	}
	if len(capture.response) > 0 {
		content, ok := response["content"].(map[string]any)
		if !ok {
			return nil, errors.New("HAR response content is missing")
		}
		mime, _ := content["mimeType"].(string)
		text, encoding, decodedSize := encodedBody(capture.response, mime, headerValue(response, "content-encoding"), responseTruncated)
		content["text"] = text
		delete(content, "encoding")
		if encoding != "" {
			content["encoding"] = encoding
		}
		if decodedSize != nil {
			content["size"] = *decodedSize
			content["compression"] = *decodedSize - len(capture.response)
		}
	}
	return complete, nil
}
func headerValue(record map[string]any, name string) string {
	values, _ := record["headers"].([]any)
	for _, value := range values {
		if header, ok := value.(map[string]any); ok {
			n, _ := header["name"].(string)
			if strings.EqualFold(n, name) {
				v, _ := header["value"].(string)
				return v
			}
		}
	}
	return ""
}
func encodedBody(body []byte, mime, encoding string, truncated bool) (string, string, *int) {
	var codings []string
	for _, coding := range strings.Split(encoding, ",") {
		coding = strings.ToLower(strings.TrimSpace(coding))
		if coding != "" && coding != "identity" {
			codings = append(codings, coding)
		}
	}
	decoded := body
	var size *int
	if len(codings) > 0 {
		if truncated {
			return base64.StdEncoding.EncodeToString(body), "base64", nil
		}
		for i := len(codings) - 1; i >= 0; i-- {
			var err error
			decoded, err = decodeCoding(decoded, codings[i])
			if err != nil {
				return base64.StdEncoding.EncodeToString(body), "base64", nil
			}
		}
		n := len(decoded)
		size = &n
	}
	mime = strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	textual := strings.HasPrefix(mime, "text/") || strings.HasSuffix(mime, "+json") || strings.HasSuffix(mime, "+xml")
	for _, m := range []string{"application/json", "application/xml", "application/javascript", "application/x-javascript", "application/x-www-form-urlencoded", "application/graphql"} {
		textual = textual || mime == m
	}
	if textual && utf8.Valid(decoded) {
		return string(decoded), "", size
	}
	return base64.StdEncoding.EncodeToString(decoded), "base64", size
}
func decodeCoding(body []byte, coding string) ([]byte, error) {
	read := func(r io.Reader) ([]byte, error) {
		out, err := io.ReadAll(io.LimitReader(r, MaxBodyBytes+1))
		if err == nil && len(out) > MaxBodyBytes {
			err = errors.New("decoded body exceeds limit")
		}
		return out, err
	}
	switch coding {
	case "gzip", "x-gzip":
		r, e := gzip.NewReader(bytes.NewReader(body))
		if e != nil {
			return nil, e
		}
		defer r.Close()
		return read(r)
	case "deflate":
		r, e := zlib.NewReader(bytes.NewReader(body))
		if e == nil {
			out, err := read(r)
			r.Close()
			if err == nil {
				return out, nil
			}
		}
		raw := flate.NewReader(bytes.NewReader(body))
		defer raw.Close()
		return read(raw)
	case "br":
		return read(brotli.NewReader(bytes.NewReader(body)))
	case "zstd":
		r, e := zstd.NewReader(bytes.NewReader(body), zstd.WithDecoderMaxMemory(MaxBodyBytes))
		if e != nil {
			return nil, e
		}
		defer r.Close()
		return read(r)
	default:
		return nil, errors.New("unsupported content encoding: " + coding)
	}
}
