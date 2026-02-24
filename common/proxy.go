package common

import (
	"net/http"
	"net/url"
	"strings"
)

func ProxyFromSettings(req *http.Request) (*url.URL, error) {
	httpProxy := strings.TrimSpace(HTTPProxy)
	httpsProxy := strings.TrimSpace(HTTPSProxy)
	if httpProxy == "" && httpsProxy == "" {
		return http.ProxyFromEnvironment(req)
	}
	scheme := ""
	if req != nil && req.URL != nil {
		scheme = strings.ToLower(req.URL.Scheme)
	}
	if scheme == "https" && httpsProxy != "" {
		return url.Parse(httpsProxy)
	}
	if scheme == "http" && httpProxy != "" {
		return url.Parse(httpProxy)
	}
	if httpsProxy != "" {
		return url.Parse(httpsProxy)
	}
	if httpProxy != "" {
		return url.Parse(httpProxy)
	}
	return nil, nil
}

func CloneTransportWithProxy() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = ProxyFromSettings
	return transport
}
