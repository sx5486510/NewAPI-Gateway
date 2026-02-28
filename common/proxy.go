package common

import (
	"net/http"
	"net/url"
	"strings"
)

func ProxyFromSettings(req *http.Request) (*url.URL, error) {
	proxy := strings.TrimSpace(Proxy)

	if proxy == "" {
		return http.ProxyFromEnvironment(req)
	}

	proxyURL, err := url.Parse(proxy)
	if err != nil {
		return nil, err
	}
	return proxyURL, nil
}

func CloneTransportWithProxy() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = ProxyFromSettings
	return transport
}
