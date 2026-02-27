package common

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func ProxyFromSettings(req *http.Request) (*url.URL, error) {
	proxy := strings.TrimSpace(Proxy)

	// Log proxy configuration for debugging
	if proxy != "" {
		scheme := ""
		if req != nil && req.URL != nil {
			scheme = strings.ToLower(req.URL.Scheme)
		}
		SysLog(fmt.Sprintf("ProxyFromSettings: request scheme=%s, proxy=%s", scheme, maskProxyUrl(proxy)))
	}

	if proxy == "" {
		return http.ProxyFromEnvironment(req)
	}

	proxyURL, err := url.Parse(proxy)
	if err != nil {
		SysLog(fmt.Sprintf("ProxyFromSettings: failed to parse proxy URL '%s': %v", maskProxyUrl(proxy), err))
		return nil, err
	}

	SysLog(fmt.Sprintf("ProxyFromSettings: using proxy %s", maskProxyUrl(proxy)))
	return proxyURL, nil
}

// maskProxyUrl masks sensitive parts of the proxy URL for logging
func maskProxyUrl(proxyUrl string) string {
	if proxyUrl == "" {
		return "(none)"
	}
	// Mask username/password if present
	masked := strings.Replace(proxyUrl, "@", ":***@", 1)
	if masked != proxyUrl {
		// URL contains credentials, mask them
		if idx := strings.Index(masked, "://"); idx > 0 {
			schemeStart := idx + 3
			if atIdx := strings.Index(masked[schemeStart:], "@"); atIdx > 0 {
				return masked[:schemeStart] + "***" + masked[schemeStart+atIdx:]
			}
		}
	}
	return masked
}

func CloneTransportWithProxy() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = ProxyFromSettings

	// Log transport configuration
	if Proxy != "" {
		SysLog(fmt.Sprintf("CloneTransportWithProxy: configured transport with proxy=%s", maskProxyUrl(Proxy)))
	}

	return transport
}
