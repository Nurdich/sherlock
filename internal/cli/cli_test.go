package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nurdich/sherlock/internal/sites"
	"github.com/Nurdich/sherlock/internal/version"
)

func TestHelpAndVersion(t *testing.T) {
	var out, err bytes.Buffer
	_, code, halt := Parse([]string{"--help"}, &out, &err)
	if !halt || code != 0 {
		t.Fatalf("help halt=%v code=%d", halt, code)
	}
	help := out.String()
	if !strings.Contains(help, "Sherlock: Find Usernames Across Social Networks (Version "+version.Version+")") {
		t.Fatalf("description missing:\n%s", help)
	}
	for _, flag := range []string{
		"--verbose", "-v", "-d", "--debug", "--folderoutput", "-fo", "--output", "-o",
		"--csv", "--xlsx", "--site", "--proxy", "-p", "--dump-response", "--json", "-j",
		"--timeout", "--print-all", "--print-found", "--no-color", "--browse", "-b",
		"--local", "-l", "--nsfw", "--txt", "--ignore-exclusions", "USERNAMES",
	} {
		if !strings.Contains(help, flag) {
			t.Fatalf("help missing %s", flag)
		}
	}
	if err.Len() != 0 {
		t.Fatalf("help stderr = %s", err.String())
	}

	out.Reset()
	_, code, halt = Parse([]string{"--version"}, &out, &err)
	if !halt || code != 0 || out.String() != "Sherlock v"+version.Version+"\n" {
		t.Fatalf("version = %q halt=%v code=%d", out.String(), halt, code)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{nil, "the following arguments are required: USERNAMES"},
		{[]string{"--"}, "the following arguments are required: USERNAMES"},
		{[]string{"--site", "urghrtuight", "--egiotr"}, "the following arguments are required: USERNAMES"},
		{[]string{"--egiotr", "user"}, "unrecognized arguments: --egiotr"},
		{[]string{"--timeout", "0", "user"}, "Invalid timeout value: 0. Timeout must be a positive number."},
		{[]string{"--timeout", "-1", "user"}, "Invalid timeout value: -1. Timeout must be a positive number."},
		{[]string{"--timeout", "abc", "user"}, "invalid float value: 'abc'"},
		{[]string{"--csv=true", "user"}, "ignored explicit argument 'true'"},
		{[]string{"--site"}, "argument --site: expected one argument"},
		{[]string{"--ver", "user"}, "ambiguous option: --ver could match --version, --verbose"},
	}
	for _, tc := range cases {
		var out, err bytes.Buffer
		_, code, halt := Parse(tc.args, &out, &err)
		if !halt || code != 2 || !strings.Contains(err.String(), tc.want) {
			t.Fatalf("args %v code=%d halt=%v stderr=%q want %q", tc.args, code, halt, err.String(), tc.want)
		}
		if !strings.HasPrefix(err.String(), "usage: sherlock ") {
			t.Fatalf("stderr missing usage: %q", err.String())
		}
	}
}

func TestParseValues(t *testing.T) {
	opt, code, halt := Parse([]string{
		"-vd", "--local", "--nsfw", "--txt", "--csv", "--xlsx", "--print-all", "--no-color",
		"--site", "GitHub", "--site", "gitlab", "--proxy", "socks5://127.0.0.1:1080",
		"--timeout", "1.5", "--folderoutput", "out", "--ignore-exclusions", "--json", "data.json",
		"user{?}", "other",
	}, ioDiscard(), ioDiscard())
	if halt || code != 0 {
		t.Fatalf("halt=%v code=%d", halt, code)
	}
	if !opt.Verbose || !opt.Local || !opt.NSFW || !opt.TXT || !opt.CSV || !opt.XLSX || !opt.PrintAll || !opt.NoColor || !opt.IgnoreExclusions {
		t.Fatalf("flags = %+v", opt)
	}
	if opt.Timeout != 1.5 || opt.FolderOutput != "out" || opt.Proxy != "socks5://127.0.0.1:1080" || opt.JSONFile != "data.json" {
		t.Fatalf("values = %+v", opt)
	}
	if len(opt.Sites) != 2 || opt.Sites[0] != "GitHub" || opt.Sites[1] != "gitlab" {
		t.Fatalf("sites = %v", opt.Sites)
	}
	if !opt.PrintFound {
		t.Fatal("print-found should default to true")
	}
	got := ExpandUsernames(opt.Usernames)
	want := []string{"user_", "user-", "user.", "other"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("expand = %v", got)
	}
	if got := ExpandUsernames([]string{"test{?}test"}); strings.Join(got, ",") != "test_test,test-test,test.test" {
		t.Fatalf("wildcard = %v", got)
	}
	for _, name := range []string{"test{.}test", "test{}test", "testtest", "test{?test", "test?}test"} {
		if expanded := ExpandUsernames([]string{name}); len(expanded) != 1 || expanded[0] != name {
			t.Fatalf("%s expanded to %v", name, expanded)
		}
	}

	opt, _, halt = Parse([]string{"-ofile", "--", "--weird"}, ioDiscard(), ioDiscard())
	if halt || opt.Output != "file" || len(opt.Usernames) != 1 || opt.Usernames[0] != "--weird" {
		t.Fatalf("attached = %+v halt=%v", opt, halt)
	}
}

func TestCheckUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Errorf("missing user agent")
		}
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","html_url":"https://example.com/release"}`))
	}))
	defer srv.Close()
	old := ReleaseURL
	ReleaseURL = srv.URL
	t.Cleanup(func() { ReleaseURL = old })
	var buf bytes.Buffer
	CheckUpdate(context.Background(), &buf)
	if !strings.Contains(buf.String(), "Update available! "+version.Version+" --> 9.9.9") || !strings.Contains(buf.String(), "https://example.com/release") {
		t.Fatalf("update = %q", buf.String())
	}

	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closed.Close()
	ReleaseURL = closed.URL
	buf.Reset()
	CheckUpdate(context.Background(), &buf)
	if !strings.Contains(buf.String(), "A problem occurred while checking for an update:") {
		t.Fatalf("update error = %q", buf.String())
	}
}

func TestExecuteLocalJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/u/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/ada") {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/release", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v` + version.Version + `","html_url":"https://example.com"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	manifest := filepath.Join(dir, "data.json")
	body := `{
	  "Example": {
	    "errorType": "status_code",
	    "url": "` + srv.URL + `/u/{}",
	    "urlMain": "` + srv.URL + `",
	    "username_claimed": "ada"
	  },
	  "Xvideos": {
	    "errorType": "status_code",
	    "isNSFW": true,
	    "url": "` + srv.URL + `/u/{}",
	    "urlMain": "` + srv.URL + `",
	    "username_claimed": "ada"
	  }
	}`
	if err := os.WriteFile(manifest, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	oldRelease := ReleaseURL
	oldExcl := sites.ExclusionsURL
	ReleaseURL = srv.URL + "/release"
	sites.ExclusionsURL = srv.URL + "/nope"
	t.Cleanup(func() {
		ReleaseURL = oldRelease
		sites.ExclusionsURL = oldExcl
	})

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	opt, code, halt := Parse([]string{
		"--json", manifest, "--ignore-exclusions", "--no-color", "--txt", "--csv", "--xlsx",
		"--timeout", "5", "--print-all", "ada",
	}, ioDiscard(), ioDiscard())
	if halt || code != 0 {
		t.Fatalf("parse halt=%v code=%d", halt, code)
	}
	var out bytes.Buffer
	if code := Execute(context.Background(), opt, &out); code != 0 {
		t.Fatalf("execute = %d\n%s", code, out.String())
	}
	text := out.String()
	if !strings.Contains(text, "[+] Example: "+srv.URL+"/u/ada") {
		t.Fatalf("missing claimed line:\n%s", text)
	}
	if strings.Contains(text, "Xvideos") {
		t.Fatalf("nsfw site should be skipped:\n%s", text)
	}
	if !strings.Contains(text, "Search completed with 1 results") {
		t.Fatalf("summary missing:\n%s", text)
	}
	txt, err := os.ReadFile(filepath.Join(dir, "ada.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(txt), srv.URL+"/u/ada") || !strings.Contains(string(txt), "Total Websites Username Detected On : 1") {
		t.Fatalf("txt = %s", txt)
	}
	if _, err := os.Stat(filepath.Join(dir, "ada.csv")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ada.xlsx")); err != nil {
		t.Fatal(err)
	}

	var errBuf bytes.Buffer
	opt.Output = "out.txt"
	opt.FolderOutput = "folder"
	if code := Execute(context.Background(), opt, &errBuf); code != 1 || !strings.Contains(errBuf.String(), "You can only use one of the output methods.") {
		t.Fatalf("conflict code=%d out=%s", code, errBuf.String())
	}
}

func TestMissingSites(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v` + version.Version + `","html_url":"https://example.com"}`))
	}))
	defer srv.Close()
	old := ReleaseURL
	ReleaseURL = srv.URL
	t.Cleanup(func() { ReleaseURL = old })

	opt := Options{
		Usernames:        []string{"ada"},
		Local:            true,
		NSFW:             true,
		IgnoreExclusions: true,
		NoColor:          true,
		Timeout:          1,
		PrintFound:       true,
		Sites:            []string{"not-a-real-site"},
	}
	var out bytes.Buffer
	if code := Execute(context.Background(), opt, &out); code != 1 {
		t.Fatalf("code = %d\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "Error: Desired sites not found: 'not-a-real-site'.") {
		t.Fatalf("out = %s", out.String())
	}
}

func ioDiscard() *bytes.Buffer { return &bytes.Buffer{} }
