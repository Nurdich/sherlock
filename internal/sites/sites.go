// Copyright (c) 2019 Sherlock Project
// SPDX-License-Identifier: MIT

// Package sites loads Sherlock site definitions from data.json.
package sites

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dlclark/regexp2"

	"github.com/Nurdich/sherlock/internal/version"
	"github.com/Nurdich/sherlock/resources"
)

const (
	// DefaultManifestURL is the upstream live site list.
	DefaultManifestURL = "https://data.sherlockproject.xyz"
	// DefaultExclusionsURL is the upstream false-positive exclusion list.
	DefaultExclusionsURL = "https://raw.githubusercontent.com/sherlock-project/sherlock/refs/heads/exclusions/false_positive_exclusions.txt"
)

// ManifestURL and ExclusionsURL can be overridden in tests.
var (
	ManifestURL             = DefaultManifestURL
	ExclusionsURL           = DefaultExclusionsURL
	MessageWriter io.Writer = os.Stdout
)

// Site is one social network probe definition.
type Site struct {
	Name             string
	URL              string
	URLMain          string
	URLProbe         string
	UsernameClaimed  string
	RegexCheck       string
	IsNSFW           bool
	Headers          map[string]string
	RequestPayload   any
	RequestMethod    string
	ErrorTypeExact   string
	ErrorTypes       []string
	ErrorMsgs        []string
	ErrorMsgPresent  bool
	ErrorMsgIsList   bool
	ErrorCodes       []int
	ErrorCodePresent bool
	ErrorCodeIsList  bool
	ErrorURL         string
	regex            *regexp2.Regexp
}

// HasRegex reports whether a regexCheck compiled successfully.
func (s Site) HasRegex() bool { return s.regex != nil }

// UsernameAllowed reports whether username satisfies the site regex.
// Sites without a regex allow every username. A regex that fails to compile
// does not block the probe.
func (s Site) UsernameAllowed(username string) bool {
	if s.regex == nil {
		return true
	}
	ok, err := s.regex.MatchString(username)
	if err != nil {
		return true
	}
	return ok
}

// LoadLocal returns the site list embedded in the binary.
// Exclusions are not applied, matching --local.
func LoadLocal() ([]Site, error) {
	return parseSites(resources.DataJSON, "resources/data.json", false, nil)
}

// Load reads a site list from a file path or URL.
// An empty location uses the upstream manifest.
// keep names are not removed when exclusions are honored (exact match).
func Load(ctx context.Context, location string, honorExclusions bool, keep []string) ([]Site, error) {
	if strings.TrimSpace(location) == "" {
		location = ManifestURL
	}
	if strings.HasPrefix(strings.ToLower(location), "http") {
		body, status, err := get(ctx, location, 30*time.Second)
		if err != nil {
			return nil, fmt.Errorf("Problem while attempting to access data file URL '%s':  %s", location, err.Error())
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("Bad response while accessing data file URL '%s'.", location)
		}
		return parseSites(body, location, honorExclusions, keep)
	}
	body, err := os.ReadFile(location)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("Problem while attempting to access data file '%s'.", location)
		}
		return nil, fmt.Errorf("Problem while attempting to access data file '%s'.", location)
	}
	return parseSites(body, location, honorExclusions, keep)
}

func parseSites(data []byte, source string, honorExclusions bool, keep []string) ([]Site, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("Problem parsing json contents at '%s':  %s.", source, err.Error())
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, fmt.Errorf("Problem parsing json contents at '%s':  expected a JSON object.", source)
	}

	var list []Site
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("Problem parsing json contents at '%s':  %s.", source, err.Error())
		}
		name, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("Problem parsing json contents at '%s':  expected a string key.", source)
		}
		var raw any
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("Problem parsing json contents at '%s':  %s.", source, err.Error())
		}
		if name == "$schema" {
			continue
		}
		obj, ok := raw.(map[string]any)
		if !ok {
			fmt.Fprintf(messageWriter(), "Encountered TypeError parsing json contents for target '%s' at %s\nSkipping target.\n", name, source)
			continue
		}
		site, err := siteFromMap(name, obj, source)
		if err != nil {
			return nil, err
		}
		list = append(list, site)
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("Problem parsing json contents at '%s':  %s.", source, err.Error())
	}

	if honorExclusions {
		filtered, err := applyExclusions(context.Background(), list, keep)
		if err != nil {
			fmt.Fprintln(messageWriter(), "Warning: Could not load exclusions, continuing without them.")
			return list, nil
		}
		list = filtered
	}
	return list, nil
}

func messageWriter() io.Writer {
	if MessageWriter == nil {
		return os.Stdout
	}
	return MessageWriter
}

func siteFromMap(name string, obj map[string]any, source string) (Site, error) {
	normalizeTree(obj)
	url, ok := obj["url"].(string)
	if !ok {
		return Site{}, fmt.Errorf("Problem parsing json contents at '%s':  Missing attribute '%s'.", source, "url")
	}
	urlMain, ok := obj["urlMain"].(string)
	if !ok {
		return Site{}, fmt.Errorf("Problem parsing json contents at '%s':  Missing attribute '%s'.", source, "urlMain")
	}
	claimed, ok := obj["username_claimed"].(string)
	if !ok {
		return Site{}, fmt.Errorf("Problem parsing json contents at '%s':  Missing attribute '%s'.", source, "username_claimed")
	}
	site := Site{
		Name:            name,
		URL:             url,
		URLMain:         urlMain,
		UsernameClaimed: claimed,
	}
	if v, ok := obj["urlProbe"].(string); ok {
		site.URLProbe = v
	}
	if v, ok := obj["regexCheck"].(string); ok {
		site.RegexCheck = v
		re, err := regexp2.Compile(v, regexp2.None)
		if err == nil {
			re.MatchTimeout = time.Second
			site.regex = re
		}
	}
	if v, ok := obj["isNSFW"].(bool); ok {
		site.IsNSFW = v
	}
	if v, ok := obj["request_method"].(string); ok {
		site.RequestMethod = v
	}
	if v, ok := obj["errorUrl"].(string); ok {
		site.ErrorURL = v
	}
	if payload, ok := obj["request_payload"]; ok {
		site.RequestPayload = payload
	}
	if headers, ok := obj["headers"].(map[string]any); ok {
		site.Headers = make(map[string]string, len(headers))
		for k, v := range headers {
			site.Headers[k] = fmt.Sprint(v)
		}
	}
	if rawType, ok := obj["errorType"]; ok {
		switch typed := rawType.(type) {
		case string:
			site.ErrorTypeExact = typed
			site.ErrorTypes = []string{typed}
		case []any:
			for _, item := range typed {
				s, ok := item.(string)
				if !ok {
					return Site{}, fmt.Errorf("Problem parsing json contents at '%s':  Missing attribute '%s'.", source, "errorType")
				}
				site.ErrorTypes = append(site.ErrorTypes, s)
			}
		default:
			return Site{}, fmt.Errorf("Problem parsing json contents at '%s':  Missing attribute '%s'.", source, "errorType")
		}
	}
	if rawMsg, ok := obj["errorMsg"]; ok {
		site.ErrorMsgPresent = true
		switch typed := rawMsg.(type) {
		case string:
			site.ErrorMsgs = []string{typed}
		case []any:
			site.ErrorMsgIsList = true
			for _, item := range typed {
				s, ok := item.(string)
				if !ok {
					return Site{}, fmt.Errorf("Problem parsing json contents at '%s':  Missing attribute '%s'.", source, "errorMsg")
				}
				site.ErrorMsgs = append(site.ErrorMsgs, s)
			}
		default:
			return Site{}, fmt.Errorf("Problem parsing json contents at '%s':  Missing attribute '%s'.", source, "errorMsg")
		}
	}
	if rawCode, ok := obj["errorCode"]; ok {
		site.ErrorCodePresent = true
		switch typed := rawCode.(type) {
		case int:
			site.ErrorCodes = []int{typed}
		case int64:
			site.ErrorCodes = []int{int(typed)}
		case float64:
			site.ErrorCodes = []int{int(typed)}
		case []any:
			site.ErrorCodeIsList = true
			for _, item := range typed {
				n, ok := asInt(item)
				if !ok {
					return Site{}, fmt.Errorf("Problem parsing json contents at '%s':  Missing attribute '%s'.", source, "errorCode")
				}
				site.ErrorCodes = append(site.ErrorCodes, n)
			}
		default:
			if n, ok := asInt(typed); ok {
				site.ErrorCodes = []int{n}
			} else {
				return Site{}, fmt.Errorf("Problem parsing json contents at '%s':  Missing attribute '%s'.", source, "errorCode")
			}
		}
	}
	return site, nil
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), n == float64(int(n))
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

func normalizeTree(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			t[k] = normalizeValue(child)
		}
	}
}

func normalizeValue(v any) any {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return int(i)
		}
		f, err := t.Float64()
		if err != nil {
			return t.String()
		}
		return f
	case map[string]any:
		for k, child := range t {
			t[k] = normalizeValue(child)
		}
		return t
	case []any:
		for i, child := range t {
			t[i] = normalizeValue(child)
		}
		return t
	default:
		return v
	}
}

func applyExclusions(ctx context.Context, list []Site, keep []string) ([]Site, error) {
	body, status, err := get(ctx, ExclusionsURL, 10*time.Second)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return list, nil
	}
	keepSet := map[string]struct{}{}
	for _, name := range keep {
		keepSet[name] = struct{}{}
	}
	drop := map[string]struct{}{}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, kept := keepSet[line]; kept {
			continue
		}
		drop[line] = struct{}{}
	}
	out := make([]Site, 0, len(list))
	for _, site := range list {
		if _, skip := drop[site.Name]; skip {
			continue
		}
		out = append(out, site)
	}
	return out, nil
}

func get(ctx context.Context, rawURL string, timeout time.Duration) ([]byte, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", version.UserAgent())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// RemoveNSFW drops sites flagged as NSFW unless their name is in keep
// (case-insensitive).
func RemoveNSFW(list []Site, keep []string) []Site {
	out := make([]Site, 0, len(list))
	for _, site := range list {
		if site.IsNSFW && !containsFold(keep, site.Name) {
			continue
		}
		out = append(out, site)
	}
	return out
}

// Select limits the list to wanted site names (case-insensitive).
// An empty wanted list returns list unchanged.
// missing contains wanted names that matched nothing.
// Selected sites follow the order of wanted, using manifest names.
func Select(list []Site, wanted []string) (selected []Site, missing []string) {
	if len(wanted) == 0 {
		return list, nil
	}
	seen := map[string]struct{}{}
	for _, name := range wanted {
		found := false
		for _, site := range list {
			if !strings.EqualFold(site.Name, name) {
				continue
			}
			found = true
			if _, ok := seen[site.Name]; ok {
				continue
			}
			seen[site.Name] = struct{}{}
			selected = append(selected, site)
		}
		if !found {
			missing = append(missing, name)
		}
	}
	return selected, missing
}

func containsFold(list []string, name string) bool {
	for _, item := range list {
		if strings.EqualFold(item, name) {
			return true
		}
	}
	return false
}

// ByName returns the site with this exact name.
func ByName(list []Site, name string) (Site, bool) {
	for _, site := range list {
		if site.Name == name {
			return site, true
		}
	}
	return Site{}, false
}
