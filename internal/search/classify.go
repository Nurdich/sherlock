// Copyright (c) 2019 Sherlock Project
// SPDX-License-Identifier: MIT

package search

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/Nurdich/sherlock/internal/result"
	"github.com/Nurdich/sherlock/internal/sites"
)

const (
	kindHTTP     = "HTTP Error"
	kindProxy    = "Proxy Error"
	kindConnect  = "Error Connecting"
	kindTimeout  = "Timeout Error"
	kindUnknown  = "Unknown Error"
	kindEncoding = "Encoding Error"
)

// EncodingError is a Unicode failure while handling a response.
// It maps to the Python UnicodeError branch.
type EncodingError struct {
	Msg string
}

func (e *EncodingError) Error() string {
	if e == nil {
		return ""
	}
	return e.Msg
}

// ProxyError is a failure caused by the configured proxy.
type ProxyError struct {
	Msg string
}

func (e *ProxyError) Error() string {
	if e == nil {
		return ""
	}
	return e.Msg
}

// DescribeFailure maps a transport error onto Sherlock's short error labels.
// failed is false when err is nil.
func DescribeFailure(err error) (kind, text string, failed bool) {
	if err == nil {
		return "", "", false
	}
	var enc *EncodingError
	if errors.As(err, &enc) {
		return kindEncoding, err.Error(), true
	}
	var proxyErr *ProxyError
	if errors.As(err, &proxyErr) {
		return kindProxy, err.Error(), true
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "proxy") {
		return kindProxy, msg, true
	}
	if isTimeout(err) {
		return kindTimeout, msg, true
	}
	var op *net.OpError
	var urlErr *url.Error
	if errors.As(err, &op) || errors.As(err, &urlErr) || strings.Contains(lower, "connection refused") || strings.Contains(lower, "no such host") {
		return kindConnect, msg, true
	}
	return kindUnknown, msg, true
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// Observation is the HTTP outcome used to classify a site.
type Observation struct {
	StatusCode int
	Body       string
	Err        error
	Elapsed    *float64
}

// Decide applies Sherlock's message, status-code, response-url, and WAF rules.
func Decide(site sites.Site, obs Observation) (result.Status, *string) {
	if obs.Err != nil {
		kind, _, failed := DescribeFailure(obs.Err)
		if !failed || kind == "" {
			kind = kindUnknown
		}
		return result.Unknown, &kind
	}
	if wafHit(obs.Body) {
		return result.WAF, nil
	}
	types := site.ErrorTypes
	for _, typ := range types {
		if typ != "message" && typ != "status_code" && typ != "response_url" {
			msg := fmt.Sprintf("Unknown error type '%s' for %s", pyList(types), site.Name)
			return result.Unknown, &msg
		}
	}
	if len(types) == 0 {
		msg := fmt.Sprintf("Unknown error type '%s' for %s", pyList(types), site.Name)
		return result.Unknown, &msg
	}

	status := result.Unknown
	if contains(types, "message") {
		hit := false
		for _, msg := range site.ErrorMsgs {
			if strings.Contains(obs.Body, msg) {
				hit = true
				break
			}
		}
		if hit {
			status = result.Available
		} else {
			status = result.Claimed
		}
	}
	if contains(types, "status_code") && status != result.Available {
		status = result.Claimed
		if site.ErrorCodePresent && containsInt(site.ErrorCodes, obs.StatusCode) {
			status = result.Available
		} else if obs.StatusCode >= 300 || obs.StatusCode < 200 {
			status = result.Available
		}
	}
	if contains(types, "response_url") && status != result.Available {
		if obs.StatusCode >= 200 && obs.StatusCode < 300 {
			status = result.Claimed
		} else {
			status = result.Available
		}
	}
	return status, nil
}

func wafHit(body string) bool {
	for _, marker := range wafMarkers {
		if strings.Contains(body, marker) {
			return true
		}
	}
	return false
}

var wafMarkers = []string{
	`.loading-spinner{visibility:hidden}body.no-js .challenge-running{display:none}body.dark{background-color:#222;color:#d9d9d9}body.dark a{color:#fff}body.dark a:hover{color:#ee730a;text-decoration:underline}body.dark .lds-ring div{border-color:#999 transparent transparent}body.dark .font-red{color:#b20f03}body.dark`,
	`<span id="challenge-error-text">`,
	`AwsWafIntegration.forceRefreshToken`,
	`{return l.onPageView}}),Object.defineProperty(r,"perimeterxIdentifiers",{enumerable:`,
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func containsInt(list []int, want int) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func pyList(items []string) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = "'" + item + "'"
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func pyInts(items []int) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = fmt.Sprintf("%d", item)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
