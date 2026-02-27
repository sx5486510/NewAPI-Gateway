package common

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func ProxyFromSettings(req *http.Request) (*url.URL, error) {
	httpProxy := strings.TrimSpace(HTTPProxy)
	httpsProxy := strings.TrimSpace(HTTPSProxy)

	// Log proxy configuration for debugging
	if httpProxy != "" || httpsProxy != "" {
		scheme := ""
		if req != nil && req.URL != nil {
			scheme = strings.ToLower(req.URL.Scheme)
		}
		SysLog(fmt.Sprintf("ProxyFromSettings: request scheme=%s, httpProxy=%s, httpsProxy=%s",
			scheme, maskProxyUrl(httpProxy), maskProxyUrl(httpsProxy)))
	}

	if httpProxy == "" && httpsProxy == "" {
		return http.ProxyFromEnvironment(req)
	}

	scheme := ""
	if req != nil && req.URL != nil {
		scheme = strings.ToLower(req.URL.Scheme)
	}

	var proxyURL *url.URL
	var err error

	if scheme == "https" && httpsProxy != "" {
		proxyURL, err = url.Parse(httpsProxy)
		if err != nil {
			SysLog(fmt.Sprintf("ProxyFromSettings: failed to parse HTTPS proxy URL '%s': %v", maskProxyUrl(httpsProxy), err))
			return nil, err
		}
		SysLog(fmt.Sprintf("ProxyFromSettings: using HTTPS proxy %s for %s request", maskProxyUrl(httpsProxy), scheme))
		return proxyURL, nil
	}

	if scheme == "http" && httpProxy != "" {
		proxyURL, err = url.Parse(httpProxy)
		if err != nil {
			SysLog(fmt.Sprintf("ProxyFromSettings: failed to parse HTTP proxy URL '%s': %v", maskProxyUrl(httpProxy), err))
			return nil, err
		}
		SysLog(fmt.Sprintf("ProxyFromSettings: using HTTP proxy %s for %s request", maskProxyUrl(httpProxy), scheme))
		return proxyURL, nil
	}

	if httpsProxy != "" {
		proxyURL, err = url.Parse(httpsProxy)
		if err != nil {
			SysLog(fmt.Sprintf("ProxyFromSettings: failed to parse HTTPS proxy URL '%s': %v", maskProxyUrl(httpsProxy), err))
			return nil, err
		}
		SysLog(fmt.Sprintf("ProxyFromSettings: using HTTPS proxy %s as default", maskProxyUrl(httpsProxy)))
		return proxyURL, nil
	}

	if httpProxy != "" {
		proxyURL, err = url.Parse(httpProxy)
		if err != nil {
			SysLog(fmt.Sprintf("ProxyFromSettings: failed to parse HTTP proxy URL '%s': %v", maskProxyUrl(httpProxy), err))
			return nil, err
		}
		SysLog(fmt.Sprintf("ProxyFromSettings: using HTTP proxy %s as default", maskProxyUrl(httpProxy)))
		return proxyURL, nil
	}

	return nil, nil
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

	// Increase timeouts for better proxy compatibility
	transport.DialContext = transport.DialContext

	// Log transport configuration
	if HTTPProxy != "" || HTTPSProxy != "" {
		SysLog(fmt.Sprintf("CloneTransportWithProxy: configured transport with proxy (HTTP=%s, HTTPS=%s)",
			maskProxyUrl(HTTPProxy), maskProxyUrl(HTTPSProxy)))
	}

	return transport
}
