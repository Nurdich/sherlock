package sites

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nurdich/sherlock/resources"
)

func TestLoadLocalManifest(t *testing.T) {
	list, err := LoadLocal()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 400 {
		t.Fatalf("expected 400+ sites, got %d", len(list))
	}
	if list[0].Name != "1337x" {
		t.Fatalf("manifest order changed, first site = %s", list[0].Name)
	}
	github, ok := ByName(list, "GitHub")
	if !ok || len(github.ErrorTypes) != 1 || github.ErrorTypes[0] != "status_code" {
		t.Fatalf("GitHub error type = %+v", github.ErrorTypes)
	}
	gitlab, ok := ByName(list, "GitLab")
	if !ok || gitlab.ErrorTypes[0] != "message" {
		t.Fatalf("GitLab error type = %+v", gitlab.ErrorTypes)
	}
	if gitlab.URLProbe == "" {
		t.Fatal("GitLab should have a probe URL")
	}
	for _, site := range list {
		if site.RegexCheck != "" && !site.HasRegex() {
			t.Fatalf("regex failed to compile for %s: %s", site.Name, site.RegexCheck)
		}
	}
	githubOK := github.UsernameAllowed("blue")
	if !githubOK || github.UsernameAllowed("-bad") || github.UsernameAllowed("a-") {
		t.Fatalf("GitHub regex mismatch: blue=%v -bad=%v a-=%v a-b=%v", githubOK, github.UsernameAllowed("-bad"), github.UsernameAllowed("a-"), github.UsernameAllowed("a-b"))
	}
	if !github.UsernameAllowed("a-b") {
		t.Fatal("GitHub should allow a-b")
	}
}

func TestManifestMatchesSchema(t *testing.T) {
	if err := ValidateManifest(resources.DataJSON); err != nil {
		t.Fatal(err)
	}
	if err := ValidateManifest(resources.DataSchema); err == nil {
		t.Fatal("schema document is not a site manifest")
	}
}

func TestRemoveNSFW(t *testing.T) {
	list, err := LoadLocal()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ByName(list, "Xvideos"); !ok {
		t.Fatal("expected Xvideos in the full manifest")
	}
	filtered := RemoveNSFW(list, nil)
	if _, ok := ByName(filtered, "Xvideos"); ok {
		t.Fatal("Xvideos should be removed")
	}
	kept := RemoveNSFW(list, []string{"xvideos", "Erome"})
	if _, ok := ByName(kept, "Xvideos"); !ok {
		t.Fatal("explicit Xvideos selection should be kept")
	}
	if _, ok := ByName(kept, "Erome"); !ok {
		t.Fatal("explicit Erome selection should be kept")
	}
	if _, ok := ByName(kept, "Motherless"); ok {
		t.Fatal("Motherless should still be removed")
	}
}

func TestSelectSites(t *testing.T) {
	list, err := LoadLocal()
	if err != nil {
		t.Fatal(err)
	}
	selected, missing := Select(list, []string{"github", "GitLab", "not-a-site"})
	if len(missing) != 1 || missing[0] != "not-a-site" {
		t.Fatalf("missing = %v", missing)
	}
	if len(selected) != 2 || selected[0].Name != "GitHub" || selected[1].Name != "GitLab" {
		t.Fatalf("selected = %+v", names(selected))
	}
	all, missing := Select(list, nil)
	if len(missing) != 0 || len(all) != len(list) {
		t.Fatalf("empty filter = %d missing %v", len(all), missing)
	}
}

func TestExclusions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("GitHub\n\nGitLab\n"))
	}))
	defer srv.Close()
	old := ExclusionsURL
	ExclusionsURL = srv.URL
	t.Cleanup(func() { ExclusionsURL = old })

	path := writeTempManifest(t)
	list, err := Load(context.Background(), path, true, []string{"GitHub"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ByName(list, "GitHub"); !ok {
		t.Fatal("kept exclusion should remain")
	}
	if _, ok := ByName(list, "GitLab"); ok {
		t.Fatal("GitLab should be excluded")
	}
	if _, ok := ByName(list, "Other"); !ok {
		t.Fatal("unlisted site should remain")
	}
}

func TestExclusionWarning(t *testing.T) {
	oldURL := ExclusionsURL
	oldWriter := MessageWriter
	ExclusionsURL = "http://127.0.0.1:1"
	var buf strings.Builder
	MessageWriter = &buf
	t.Cleanup(func() {
		ExclusionsURL = oldURL
		MessageWriter = oldWriter
	})
	list, err := Load(context.Background(), writeTempManifest(t), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("sites = %d", len(list))
	}
	if !strings.Contains(buf.String(), "Warning: Could not load exclusions, continuing without them.") {
		t.Fatalf("warning = %q", buf.String())
	}
}

func TestMissingFile(t *testing.T) {
	_, err := Load(context.Background(), filepath.Join(t.TempDir(), "missing.json"), false, nil)
	if err == nil || !strings.Contains(err.Error(), "Problem while attempting to access data file") {
		t.Fatalf("err = %v", err)
	}
}

func TestBadJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(context.Background(), path, false, nil)
	if err == nil || !strings.Contains(err.Error(), "Problem parsing json contents") {
		t.Fatalf("err = %v", err)
	}
}

func TestMissingAttribute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	if err := os.WriteFile(path, []byte(`{"GitHub": {"urlMain": "https://example.com"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(context.Background(), path, false, nil)
	if err == nil || !strings.Contains(err.Error(), "Missing attribute 'url'") {
		t.Fatalf("err = %v", err)
	}
}

func writeTempManifest(t *testing.T) string {
	t.Helper()
	body := `{
	  "GitHub": {"url": "https://example.com/{}", "urlMain": "https://example.com", "username_claimed": "blue", "errorType": "status_code"},
	  "GitLab": {"url": "https://example.com/{}", "urlMain": "https://example.com", "username_claimed": "blue", "errorType": "message", "errorMsg": "missing"},
	  "Other": {"url": "https://example.com/{}", "urlMain": "https://example.com", "username_claimed": "blue", "errorType": "status_code"}
	}`
	path := filepath.Join(t.TempDir(), "data.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func names(list []Site) []string {
	out := make([]string, len(list))
	for i, site := range list {
		out[i] = site.Name
	}
	return out
}
