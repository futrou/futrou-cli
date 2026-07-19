package services

import (
	"crypto/tls"
	"net/http"
	"os"
	"time"

	"github.com/quic-go/quic-go/http3"
)

func init() {
	// Suppress quic-go's stderr warning about the OS UDP receive buffer
	// being smaller than it would like — informational only, and noisy
	// for a CLI since it can't tune the host's net.core.rmem_max.
	if os.Getenv("QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING") == "" {
		os.Setenv("QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING", "true")
	}
}

// newHttpClient builds an http.Client restricted to TLS 1.2/1.3, with
// HTTP/2 support over TLS and an opportunistic upgrade to HTTP/3 (QUIC)
// when the server advertises support for it, falling back transparently
// to HTTP/1.1 or HTTP/2 otherwise.
func newHttpClient(timeout time.Duration) *http.Client {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
	}

	h3Transport := &http3.Transport{TLSClientConfig: tlsConfig}
	h1h2Transport := &http.Transport{
		TLSClientConfig:   tlsConfig,
		ForceAttemptHTTP2: true,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: &fallbackTransport{primary: h3Transport, fallback: h1h2Transport},
	}
}

// fallbackTransport tries primary (HTTP/3) first and falls back to a
// secondary transport (HTTP/1.1 or HTTP/2) if the primary can't complete
// the round trip — e.g. the server doesn't speak QUIC, or the network
// blocks UDP.
type fallbackTransport struct {
	primary  http.RoundTripper
	fallback http.RoundTripper
}

func (t *fallbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme == "https" {
		if resp, err := t.primary.RoundTrip(req.Clone(req.Context())); err == nil {
			return resp, nil
		}
	}
	return t.fallback.RoundTrip(req)
}
