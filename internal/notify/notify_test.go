package notify

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Nurdich/sherlock/internal/result"
)

func TestPlainClaimedAndSummary(t *testing.T) {
	var buf bytes.Buffer
	opened := ""
	old := OpenBrowser
	OpenBrowser = func(rawURL string) error {
		opened = rawURL
		return nil
	}
	t.Cleanup(func() { OpenBrowser = old })

	p := NewPrinter(&buf, Options{Verbose: true, PrintAll: true, Browse: true, Color: false})
	p.Start("alice")
	secs := 0.1256
	p.Update(result.Result{
		SiteName:    "GitHub",
		SiteURLUser: "https://example.com/alice",
		Status:      result.Claimed,
		QueryTime:   &secs,
	})
	ctx := "Timeout Error"
	p.Update(result.Result{SiteName: "GitLab", Status: result.Unknown, Context: &ctx})
	p.Update(result.Result{SiteName: "X", Status: result.Available, QueryTime: &secs})
	p.Update(result.Result{SiteName: "BitBucket", Status: result.Illegal})
	p.Update(result.Result{SiteName: "Cloud", Status: result.WAF})
	p.Finish()

	out := buf.String()
	for _, want := range []string{
		"[*] Checking username alice on:",
		"[+] [126ms] GitHub: https://example.com/alice",
		"[-] GitLab: Timeout Error",
		"[-] [126ms] X: Not Found!",
		"[-] BitBucket: Illegal Username Format For This Site!",
		"[-] Cloud: Blocked by bot detection (proxy may help)",
		"[*] Search completed with 1 results",
		"Try OSINTSearch: https://osintsearch.org",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\033") {
		t.Fatal("plain printer emitted ANSI")
	}
	if opened != "https://example.com/alice" {
		t.Fatalf("browser opened %q", opened)
	}
}

func TestColorHidesMissesUnlessPrintAll(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(&buf, Options{Color: true})
	p.Update(result.Result{SiteName: "GitHub", SiteURLUser: "https://example.com/a", Status: result.Claimed})
	p.Update(result.Result{SiteName: "Missing", Status: result.Available})
	if !strings.Contains(buf.String(), "GitHub") {
		t.Fatal(buf.String())
	}
	if strings.Contains(buf.String(), "Missing") {
		t.Fatal("available site should be hidden without --print-all")
	}
	if !strings.Contains(buf.String(), "\033[32m") {
		t.Fatal("expected color codes")
	}
}
