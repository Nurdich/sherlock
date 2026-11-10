package search

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Nurdich/sherlock/internal/result"
	"github.com/Nurdich/sherlock/internal/sites"
)

type capture struct {
	results []result.Result
}

func (c *capture) Start(string)             {}
func (c *capture) Update(res result.Result) { c.results = append(c.results, res) }
func (c *capture) Finish()                  {}

func TestDescribeFailureUnicode(t *testing.T) {
	err := &EncodingError{Msg: "'utf-8' codec can't decode byte 0xe9 in position 0: invalid continuation byte"}
	kind, text, failed := DescribeFailure(err)
	if !failed || kind != kindEncoding || !strings.Contains(text, "utf-8") {
		t.Fatalf("kind=%s text=%s failed=%v", kind, text, failed)
	}
	err = &EncodingError{Msg: "'ascii' codec can't encode character"}
	kind, text, failed = DescribeFailure(err)
	if !failed || kind != kindEncoding || !strings.Contains(text, "ascii") {
		t.Fatalf("encode kind=%s text=%s failed=%v", kind, text, failed)
	}
}

func TestDecideRules(t *testing.T) {
	message := sites.Site{Name: "Msg", ErrorTypes: []string{"message"}, ErrorMsgs: []string{"missing"}}
	status, ctx := Decide(message, Observation{StatusCode: 200, Body: "hello missing user"})
	if status != result.Available || ctx != nil {
		t.Fatalf("message available = %s %v", status, ctx)
	}
	status, _ = Decide(message, Observation{StatusCode: 200, Body: "hello user"})
	if status != result.Claimed {
		t.Fatalf("message claimed = %s", status)
	}

	code := sites.Site{Name: "Code", ErrorTypeExact: "status_code", ErrorTypes: []string{"status_code"}, ErrorCodePresent: true, ErrorCodes: []int{204}}
	status, _ = Decide(code, Observation{StatusCode: 200, Body: "ok"})
	if status != result.Claimed {
		t.Fatalf("200 claimed = %s", status)
	}
	status, _ = Decide(code, Observation{StatusCode: 204})
	if status != result.Available {
		t.Fatalf("204 available = %s", status)
	}
	plain := sites.Site{Name: "Plain", ErrorTypes: []string{"status_code"}}
	status, _ = Decide(plain, Observation{StatusCode: 404})
	if status != result.Available {
		t.Fatalf("404 available = %s", status)
	}
	status, _ = Decide(plain, Observation{StatusCode: 201})
	if status != result.Claimed {
		t.Fatalf("201 claimed = %s", status)
	}

	both := sites.Site{
		Name: "Both", ErrorTypes: []string{"message", "status_code"}, ErrorMsgs: []string{"gone"},
	}
	status, _ = Decide(both, Observation{StatusCode: 200, Body: "gone"})
	if status != result.Available {
		t.Fatalf("message should win as available, got %s", status)
	}
	status, _ = Decide(both, Observation{StatusCode: 404, Body: "present"})
	if status != result.Available {
		t.Fatalf("status code should mark 404 available, got %s", status)
	}

	redir := sites.Site{Name: "Redir", ErrorTypeExact: "response_url", ErrorTypes: []string{"response_url"}}
	status, _ = Decide(redir, Observation{StatusCode: 200})
	if status != result.Claimed {
		t.Fatalf("redirect 200 = %s", status)
	}
	status, _ = Decide(redir, Observation{StatusCode: 302})
	if status != result.Available {
		t.Fatalf("redirect 302 = %s", status)
	}

	status, _ = Decide(message, Observation{StatusCode: 200, Body: `<span id="challenge-error-text">`})
	if status != result.WAF {
		t.Fatalf("waf = %s", status)
	}
	status, ctx = Decide(message, Observation{Err: &EncodingError{Msg: "utf-8"}})
	if status != result.Unknown || ctx == nil || *ctx != kindEncoding {
		t.Fatalf("transport = %s %v", status, ctx)
	}
	bad := sites.Site{Name: "Bad", ErrorTypes: []string{"nope"}}
	status, ctx = Decide(bad, Observation{StatusCode: 200, Body: "x"})
	if status != result.Unknown || ctx == nil || *ctx != "Unknown error type '['nope']' for Bad" {
		t.Fatalf("unknown type = %s %v", status, ctx)
	}
}

func TestInterpolateAndURLs(t *testing.T) {
	payload := map[string]any{
		"query": "hi {}",
		"variables": map[string]any{
			"name": "{}",
			"n":    1,
		},
		"tags": []any{"{}", "keep"},
	}
	got := Interpolate(payload, "ann").(map[string]any)
	if got["query"] != "hi ann" {
		t.Fatalf("query = %#v", got["query"])
	}
	vars := got["variables"].(map[string]any)
	if vars["name"] != "ann" || vars["n"] != 1 {
		t.Fatalf("vars = %#v", vars)
	}
	tags := got["tags"].([]any)
	if tags[0] != "ann" || tags[1] != "keep" {
		t.Fatalf("tags = %#v", tags)
	}
	if got := ProfileURL("https://example.com/{}/x", "a b"); got != "https://example.com/a%20b/x" {
		t.Fatalf("profile = %s", got)
	}
	if got := ProbeURL("https://probe.example/{}/", "https://example.com/a%20b", "a b"); got != "https://probe.example/a b/" {
		t.Fatalf("probe = %s", got)
	}
}

func TestRunHTTPBehaviors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/head/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method = %s", r.Method)
		}
		if r.Header.Get("User-Agent") != UserAgent {
			t.Errorf("ua = %s", r.Header.Get("User-Agent"))
		}
		if !strings.HasSuffix(r.URL.Path, "/blue") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/msg/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		name := strings.TrimPrefix(r.URL.Path, "/msg/")
		if name == "blue" {
			_, _ = io.WriteString(w, "profile blue")
			return
		}
		_, _ = io.WriteString(w, "Page not found")
	})
	mux.HandleFunc("/post", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"name":"blue"`) && !strings.Contains(string(body), `"name": "blue"`) {
			t.Errorf("body = %s", body)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type = %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("X-Test") != "yes" {
			t.Errorf("header = %s", r.Header.Get("X-Test"))
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/redir", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/landed", http.StatusFound)
	})
	mux.HandleFunc("/landed", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/waf", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `AwsWafIntegration.forceRefreshToken`)
	})
	mux.HandleFunc("/space/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/space/a%20b" && r.URL.RawPath != "/space/a%20b" && !strings.Contains(r.URL.EscapedPath(), "a%20b") {
			t.Errorf("path = %s raw=%s", r.URL.Path, r.URL.RawPath)
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	list := []sites.Site{
		{Name: "Head", URL: srv.URL + "/head/{}", URLMain: srv.URL, ErrorTypeExact: "status_code", ErrorTypes: []string{"status_code"}},
		{Name: "Msg", URL: srv.URL + "/msg/{}", URLMain: srv.URL, ErrorTypes: []string{"message"}, ErrorMsgs: []string{"Page not found"}, ErrorMsgPresent: true},
		{Name: "Post", URL: srv.URL + "/u/{}", URLMain: srv.URL, URLProbe: srv.URL + "/post", ErrorTypeExact: "status_code", ErrorTypes: []string{"status_code"}, RequestMethod: http.MethodPost, RequestPayload: map[string]any{"name": "{}"}, Headers: map[string]string{"X-Test": "yes"}},
		{Name: "Redir", URL: srv.URL + "/redir", URLMain: srv.URL, ErrorTypeExact: "response_url", ErrorTypes: []string{"response_url"}},
		{Name: "WAF", URL: srv.URL + "/waf", URLMain: srv.URL, ErrorTypes: []string{"message"}, ErrorMsgs: []string{"absent"}},
		{Name: "Illegal", URL: srv.URL + "/nope/{}", URLMain: srv.URL, ErrorTypes: []string{"status_code"}, RegexCheck: "^[a-z]+$"},
		{Name: "Space", URL: srv.URL + "/space/{}", URLMain: srv.URL, ErrorTypeExact: "status_code", ErrorTypes: []string{"status_code"}},
	}
	// Compile the illegal regex the same way the loader does.
	loaded, err := sites.Load(context.Background(), writeSites(t, list[5]), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	list[5] = loaded[0]
	list[5].URL = srv.URL + "/nope/{}"

	var cap capture
	var dump bytes.Buffer
	hits, err := Run(context.Background(), "blue", list[:5], &cap, Config{Timeout: 5 * time.Second, Dump: true, DumpWriter: &dump})
	if err != nil {
		t.Fatal(err)
	}
	if hits[0].Result.Status != result.Claimed || hits[0].HTTPStatus != "200" {
		t.Fatalf("head = %+v", hits[0])
	}
	if hits[1].Result.Status != result.Claimed {
		t.Fatalf("msg claimed = %s body %s", hits[1].Result.Status, hits[1].ResponseBody)
	}
	if hits[2].Result.Status != result.Claimed {
		t.Fatalf("post = %s", hits[2].Result.Status)
	}
	if hits[3].Result.Status != result.Available || hits[3].HTTPStatus != "302" {
		t.Fatalf("redir = %+v", hits[3])
	}
	if hits[4].Result.Status != result.WAF {
		t.Fatalf("waf = %s", hits[4].Result.Status)
	}
	if !strings.Contains(dump.String(), "TARGET NAME   : Head") || !strings.Contains(dump.String(), "VERDICT       : Claimed") {
		t.Fatalf("dump = %s", dump.String())
	}

	cap = capture{}
	hits, err = Run(context.Background(), "nope", list[:2], &cap, Config{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if hits[0].Result.Status != result.Available || hits[1].Result.Status != result.Available {
		t.Fatalf("negatives = %s %s", hits[0].Result.Status, hits[1].Result.Status)
	}

	cap = capture{}
	hits, err = Run(context.Background(), "BAD", []sites.Site{list[5]}, &cap, Config{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if hits[0].Result.Status != result.Illegal || hits[0].URLUser != "" {
		t.Fatalf("illegal = %+v", hits[0])
	}
	if len(cap.results) != 1 || cap.results[0].SiteURLUser == "" {
		t.Fatalf("notified illegal url = %+v", cap.results)
	}

	hits, err = Run(context.Background(), "a b", []sites.Site{list[6]}, &cap, Config{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if hits[0].Result.Status != result.Claimed || hits[0].URLUser != srv.URL+"/space/a%20b" {
		t.Fatalf("space = %+v", hits[0])
	}
}

func TestTransportErrors(t *testing.T) {
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer hang.Close()
	site := sites.Site{Name: "Slow", URL: hang.URL, URLMain: hang.URL, ErrorTypeExact: "status_code", ErrorTypes: []string{"status_code"}}
	hits, err := Run(context.Background(), "blue", []sites.Site{site}, nil, Config{Timeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if hits[0].Result.Status != result.Unknown || hits[0].Result.Context == nil || *hits[0].Result.Context != kindTimeout {
		t.Fatalf("timeout = %+v", hits[0].Result)
	}
	if hits[0].HTTPStatus != "?" {
		t.Fatalf("http status = %s", hits[0].HTTPStatus)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	closed := sites.Site{Name: "Closed", URL: "http://" + addr, URLMain: "http://" + addr, ErrorTypes: []string{"message"}, ErrorMsgs: []string{"x"}}
	hits, err = Run(context.Background(), "blue", []sites.Site{closed}, nil, Config{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if hits[0].Result.Context == nil || *hits[0].Result.Context != kindConnect {
		t.Fatalf("connect = %+v err-kind", hits[0].Result)
	}
}

func TestUnsupportedMethod(t *testing.T) {
	site := sites.Site{Name: "Bad", URL: "https://example.com", URLMain: "https://example.com", RequestMethod: "DELETE", ErrorTypes: []string{"status_code"}}
	_, err := Run(context.Background(), "blue", []sites.Site{site}, nil, Config{Timeout: time.Second})
	if err == nil || !strings.Contains(err.Error(), "Unsupported request_method") {
		t.Fatalf("err = %v", err)
	}
}

func TestExpandNotInSearch(t *testing.T) {
	// Placeholder so transport classification of a plain error stays unknown.
	kind, _, failed := DescribeFailure(errors.New("something else"))
	if !failed || kind != kindUnknown {
		t.Fatalf("kind = %s", kind)
	}
}

func writeSites(t *testing.T, site sites.Site) string {
	t.Helper()
	_ = site
	body := `{
	  "Illegal": {
	    "url": "https://example.invalid/{}",
	    "urlMain": "https://example.invalid",
	    "username_claimed": "blue",
	    "errorType": "status_code",
	    "regexCheck": "^[a-z]+$"
	  }
	}`
	path := t.TempDir() + "/data.json"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
