package sites

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

var allowedSiteProps = map[string]struct{}{
	"url": {}, "urlMain": {}, "urlProbe": {}, "username_claimed": {},
	"regexCheck": {}, "isNSFW": {}, "headers": {}, "request_payload": {},
	"__comment__": {}, "tags": {}, "request_method": {}, "errorType": {},
	"errorMsg": {}, "errorCode": {}, "errorUrl": {}, "response_url": {},
}

var allowedTags = map[string]struct{}{"adult": {}, "gaming": {}}
var allowedMethods = map[string]struct{}{"GET": {}, "POST": {}, "HEAD": {}, "PUT": {}}
var allowedErrTypes = map[string]struct{}{"message": {}, "response_url": {}, "status_code": {}}

// ValidateManifest checks data.json against the rules of data.schema.json.
// The schema's patternProperties expression uses lookahead, which Go's RE2
// engine cannot compile, so the constraints are applied directly.
func ValidateManifest(data []byte) error {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return err
	}
	if schema, ok := root["$schema"]; ok {
		if _, ok := schema.(string); !ok {
			return fmt.Errorf("$schema must be a string")
		}
	}
	for name, raw := range root {
		if name == "$schema" {
			continue
		}
		if strings.HasPrefix(name, "$") {
			return fmt.Errorf("site name %q must not start with $", name)
		}
		obj, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: site entry must be an object", name)
		}
		if err := validateSite(name, obj); err != nil {
			return err
		}
	}
	return nil
}

func validateSite(name string, obj map[string]any) error {
	for _, key := range []string{"url", "urlMain", "errorType", "username_claimed"} {
		if _, ok := obj[key]; !ok {
			return fmt.Errorf("%s: missing %s", name, key)
		}
	}
	for key := range obj {
		if _, ok := allowedSiteProps[key]; !ok {
			return fmt.Errorf("%s: unexpected property %s", name, key)
		}
	}
	for _, key := range []string{"url", "urlMain", "urlProbe", "username_claimed", "regexCheck", "errorUrl", "response_url", "__comment__"} {
		if v, ok := obj[key]; ok {
			if _, ok := v.(string); !ok {
				return fmt.Errorf("%s: %s must be a string", name, key)
			}
		}
	}
	if v, ok := obj["isNSFW"]; ok {
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("%s: isNSFW must be a boolean", name)
		}
	}
	if v, ok := obj["request_method"]; ok {
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("%s: request_method must be a string", name)
		}
		if _, ok := allowedMethods[s]; !ok {
			return fmt.Errorf("%s: invalid request_method %s", name, s)
		}
	}
	types, err := stringOrStrings(obj["errorType"])
	if err != nil {
		return fmt.Errorf("%s: errorType: %w", name, err)
	}
	for _, typ := range types {
		if _, ok := allowedErrTypes[typ]; !ok {
			return fmt.Errorf("%s: invalid errorType %s", name, typ)
		}
	}
	has := func(want string) bool {
		for _, typ := range types {
			if typ == want {
				return true
			}
		}
		return false
	}
	if msg, ok := obj["errorMsg"]; ok {
		if _, err := stringOrStrings(msg); err != nil {
			return fmt.Errorf("%s: errorMsg: %w", name, err)
		}
	} else if has("message") {
		return fmt.Errorf("%s: errorType message requires errorMsg", name)
	}
	if code, ok := obj["errorCode"]; ok {
		if err := intOrInts(code); err != nil {
			return fmt.Errorf("%s: errorCode: %w", name, err)
		}
	}
	if has("response_url") {
		if _, ok := obj["errorUrl"].(string); !ok {
			return fmt.Errorf("%s: errorType response_url requires errorUrl", name)
		}
	}
	if headers, ok := obj["headers"]; ok {
		m, ok := headers.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: headers must be an object", name)
		}
		for hk, hv := range m {
			if _, ok := hv.(string); !ok {
				return fmt.Errorf("%s: header %s must be a string", name, hk)
			}
		}
	}
	if payload, ok := obj["request_payload"]; ok {
		if _, ok := payload.(map[string]any); !ok {
			return fmt.Errorf("%s: request_payload must be an object", name)
		}
	}
	if tags, ok := obj["tags"]; ok {
		if err := validateTags(tags); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func validateTags(v any) error {
	switch t := v.(type) {
	case string:
		if _, ok := allowedTags[t]; !ok {
			return fmt.Errorf("invalid tag %s", t)
		}
	case []any:
		if len(t) == 0 {
			return fmt.Errorf("tags must not be empty")
		}
		for _, item := range t {
			s, ok := item.(string)
			if !ok {
				return fmt.Errorf("tag must be a string")
			}
			if _, ok := allowedTags[s]; !ok {
				return fmt.Errorf("invalid tag %s", s)
			}
		}
	default:
		return fmt.Errorf("tags must be a string or array")
	}
	return nil
}

func stringOrStrings(v any) ([]string, error) {
	switch t := v.(type) {
	case string:
		return []string{t}, nil
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string")
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected string or array of strings")
	}
}

func intOrInts(v any) error {
	switch t := v.(type) {
	case float64:
		if t != math.Trunc(t) {
			return fmt.Errorf("expected integer")
		}
		return nil
	case []any:
		for _, item := range t {
			n, ok := item.(float64)
			if !ok || n != math.Trunc(n) {
				return fmt.Errorf("expected integer")
			}
		}
		return nil
	default:
		return fmt.Errorf("expected integer or array of integers")
	}
}
