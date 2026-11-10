package search

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

type redirectKey struct{}

func newClient(timeout time.Duration, proxyURL string) (*http.Client, error) {
	transport, err := newTransport(proxyURL)
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			allow, _ := req.Context().Value(redirectKey{}).(bool)
			if !allow {
				return http.ErrUseLastResponse
			}
			if len(via) >= 30 {
				return fmt.Errorf("stopped after 30 redirects")
			}
			return nil
		},
	}
	return client, nil
}

func newTransport(proxyURL string) (*http.Transport, error) {
	var transport *http.Transport
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = base.Clone()
	} else {
		transport = &http.Transport{}
	}
	if proxyURL == "" {
		transport.Proxy = http.ProxyFromEnvironment
		return transport, nil
	}
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL %q: %w", proxyURL, err)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "socks5", "socks5h", "socks4", "socks4a":
		dialer, err := proxy.FromURL(parsed, proxy.Direct)
		if err != nil {
			return nil, err
		}
		transport.Proxy = nil
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialContext(ctx, dialer, network, addr)
		}
	default:
		transport.Proxy = http.ProxyURL(parsed)
	}
	return transport, nil
}

func dialContext(ctx context.Context, dialer proxy.Dialer, network, addr string) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	if cd, ok := dialer.(proxy.ContextDialer); ok {
		conn, err := cd.DialContext(ctx, network, addr)
		if err != nil {
			return nil, &ProxyError{Msg: err.Error()}
		}
		return conn, nil
	}
	ch := make(chan result, 1)
	go func() {
		conn, err := dialer.Dial(network, addr)
		ch <- result{conn, err}
	}()
	select {
	case <-ctx.Done():
		return nil, &ProxyError{Msg: ctx.Err().Error()}
	case res := <-ch:
		if res.err != nil {
			return nil, &ProxyError{Msg: res.err.Error()}
		}
		return res.conn, nil
	}
}

func requestURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		raw = strings.ReplaceAll(raw, " ", "%20")
		u, err = url.Parse(raw)
		if err != nil {
			return "", err
		}
	}
	return u.String(), nil
}
