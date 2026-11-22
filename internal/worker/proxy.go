package worker

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"time"
)

func newUnixTransport(socketPath string) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
		DisableKeepAlives: false,
		MaxIdleConns:      10,
		IdleConnTimeout:   30 * time.Second,
	}
}

func ProxyWebDAV(socketPath, username string, w http.ResponseWriter, r *http.Request, quotaLimit, usedBytes int64) (int64, error) {
	var delta int64
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = "unix"
			pr.Out.Header.Set("X-Andvari-User", username)
			pr.Out.Header.Set("X-Andvari-Quota-Limit", strconv.FormatInt(quotaLimit, 10))
			pr.Out.Header.Set("X-Andvari-Quota-Used", strconv.FormatInt(usedBytes, 10))
		},
		Transport: newUnixTransport(socketPath),
		ModifyResponse: func(resp *http.Response) error {
			if deltaStr := resp.Header.Get("X-Andvari-Usage-Delta"); deltaStr != "" {
				if d, err := strconv.ParseInt(deltaStr, 10, 64); err == nil {
					delta = d
				}
				resp.Header.Del("X-Andvari-Usage-Delta")
			}
			return nil
		},
	}

	proxy.ServeHTTP(w, r)
	return delta, nil
}

func ProxyFilesListOrDelete(socketPath, username string, w http.ResponseWriter, r *http.Request) (int64, error) {
	var delta int64
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = "unix"
			pr.Out.URL.Path = "/_worker/files"
			pr.Out.Header.Set("X-Andvari-User", username)
		},
		Transport: newUnixTransport(socketPath),
		ModifyResponse: func(resp *http.Response) error {
			if deltaStr := resp.Header.Get("X-Andvari-Usage-Delta"); deltaStr != "" {
				if d, err := strconv.ParseInt(deltaStr, 10, 64); err == nil {
					delta = d
				}
				resp.Header.Del("X-Andvari-Usage-Delta")
			}
			return nil
		},
	}

	proxy.ServeHTTP(w, r)
	return delta, nil
}

func ProxyFileDownload(socketPath, username string, w http.ResponseWriter, r *http.Request) error {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = "unix"
			pr.Out.URL.Path = "/_worker/files/download"
			pr.Out.Header.Set("X-Andvari-User", username)
		},
		Transport: newUnixTransport(socketPath),
	}

	proxy.ServeHTTP(w, r)
	return nil
}

func ProxyFileUpload(socketPath, username string, w http.ResponseWriter, r *http.Request, quotaLimit, usedBytes int64) (int64, error) {
	var delta int64
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = "unix"
			pr.Out.URL.Path = "/_worker/files/upload"
			pr.Out.Header.Set("X-Andvari-User", username)
			pr.Out.Header.Set("X-Andvari-Quota-Limit", strconv.FormatInt(quotaLimit, 10))
			pr.Out.Header.Set("X-Andvari-Quota-Used", strconv.FormatInt(usedBytes, 10))
		},
		Transport: newUnixTransport(socketPath),
		ModifyResponse: func(resp *http.Response) error {
			if deltaStr := resp.Header.Get("X-Andvari-Usage-Delta"); deltaStr != "" {
				if d, err := strconv.ParseInt(deltaStr, 10, 64); err == nil {
					delta = d
				}
				resp.Header.Del("X-Andvari-Usage-Delta")
			}
			return nil
		},
	}

	proxy.ServeHTTP(w, r)
	return delta, nil
}

func ProxyCreateFolder(socketPath, username string, w http.ResponseWriter, r *http.Request) error {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = "unix"
			pr.Out.URL.Path = "/_worker/files/folder"
			pr.Out.Header.Set("X-Andvari-User", username)
		},
		Transport: newUnixTransport(socketPath),
	}
	proxy.ServeHTTP(w, r)
	return nil
}

func ProxyRenamePath(socketPath, username string, w http.ResponseWriter, r *http.Request) (int64, error) {
	var delta int64
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = "unix"
			pr.Out.URL.Path = "/_worker/files/rename"
			pr.Out.Header.Set("X-Andvari-User", username)
		},
		Transport: newUnixTransport(socketPath),
		ModifyResponse: func(resp *http.Response) error {
			if deltaStr := resp.Header.Get("X-Andvari-Usage-Delta"); deltaStr != "" {
				if d, err := strconv.ParseInt(deltaStr, 10, 64); err == nil {
					delta = d
				}
				resp.Header.Del("X-Andvari-Usage-Delta")
			}
			return nil
		},
	}
	proxy.ServeHTTP(w, r)
	return delta, nil
}

