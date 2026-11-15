package utils

import (
	"net"
	"net/url"
	"strings"
)

// NormalizeApiUrl turns whatever --api-url / FUTROU_API_URL holds into a base
// URL: a missing scheme is added (https, or http for localhost and other
// loopback addresses, where development servers run), ws/wss become http/https,
// and a trailing slash and a trailing "/v2" are dropped, so "api.futrou.com",
// "https://api.futrou.com/" and "https://api.futrou.com/v2" all resolve alike.
// Every package that keys or builds an API URL uses this one function, so a
// token is always stored and looked up under the same spelling.
func NormalizeApiUrl(apiUrl string) string {
	apiUrl = strings.TrimSpace(apiUrl)
	if apiUrl == "" {
		return ""
	}
	if !strings.Contains(apiUrl, "://") {
		scheme := "https"
		if parsed, err := url.Parse("//" + apiUrl); err == nil && isLoopbackHost(parsed.Hostname()) {
			scheme = "http"
		}
		apiUrl = scheme + "://" + apiUrl
	}
	switch {
	case strings.HasPrefix(strings.ToLower(apiUrl), "wss://"):
		apiUrl = "https://" + apiUrl[len("wss://"):]
	case strings.HasPrefix(strings.ToLower(apiUrl), "ws://"):
		apiUrl = "http://" + apiUrl[len("ws://"):]
	}
	apiUrl = strings.TrimRight(apiUrl, "/")
	apiUrl = strings.TrimSuffix(apiUrl, "/v2")
	return strings.TrimRight(apiUrl, "/")
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
