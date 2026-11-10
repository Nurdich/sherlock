// Copyright (c) 2019 Sherlock Project
// SPDX-License-Identifier: MIT

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nurdich/sherlock/internal/notify"
	"github.com/Nurdich/sherlock/internal/output"
	"github.com/Nurdich/sherlock/internal/search"
	"github.com/Nurdich/sherlock/internal/sites"
	"github.com/Nurdich/sherlock/internal/version"
)

// ReleaseURL is the upstream GitHub releases endpoint used for update checks.
var ReleaseURL = "https://api.github.com/repos/sherlock-project/sherlock/releases/latest"

// PullError is returned when --json is given a pull request number that does not exist.
type PullError struct {
	Number string
}

func (e *PullError) Error() string {
	return fmt.Sprintf("Pull request #%s not found.", e.Number)
}

// Main parses args and runs Sherlock.
func Main(args []string) int {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		os.Exit(0)
	}()
	opt, code, halt := Parse(args, os.Stdout, os.Stderr)
	if halt {
		return code
	}
	return Execute(context.Background(), opt, os.Stdout)
}

// Execute runs a parsed search and writes reports.
func Execute(ctx context.Context, opt Options, stdout io.Writer) int {
	if ctx == nil {
		ctx = context.Background()
	}
	CheckUpdate(ctx, stdout)
	if opt.Proxy != "" {
		fmt.Fprintf(stdout, "Using the proxy: %s\n", opt.Proxy)
	}
	if opt.Output != "" && opt.FolderOutput != "" {
		fmt.Fprintln(stdout, "You can only use one of the output methods.")
		return 1
	}
	if opt.Output != "" && len(opt.Usernames) != 1 {
		fmt.Fprintln(stdout, "You can only use --output with a single username")
		return 1
	}

	list, err := loadSites(ctx, opt)
	if err != nil {
		var pull *PullError
		if errors.As(err, &pull) {
			fmt.Fprintf(stdout, "ERROR: %s\n", pull.Error())
			return 1
		}
		fmt.Fprintf(stdout, "ERROR:  %s\n", err)
		return 1
	}
	if !opt.NSFW {
		list = sites.RemoveNSFW(list, opt.Sites)
	}
	selected, missing := sites.Select(list, opt.Sites)
	if len(missing) > 0 {
		quoted := make([]string, len(missing))
		for i, name := range missing {
			quoted[i] = "'" + name + "'"
		}
		fmt.Fprintf(stdout, "Error: Desired sites not found: %s.\n", strings.Join(quoted, ", "))
	}
	if len(opt.Sites) > 0 && len(selected) == 0 {
		return 1
	}

	printer := notify.NewPrinter(stdout, notify.Options{
		Verbose:  opt.Verbose,
		PrintAll: opt.PrintAll,
		Browse:   opt.Browse,
		Color:    !opt.NoColor,
	})
	for _, username := range ExpandUsernames(opt.Usernames) {
		hits, err := search.Run(ctx, username, selected, printer, search.Config{
			Timeout:    time.Duration(opt.Timeout * float64(time.Second)),
			Proxy:      opt.Proxy,
			Dump:       opt.DumpResponse,
			DumpWriter: stdout,
		})
		if err != nil {
			fmt.Fprintf(stdout, "ERROR:  %s\n", err)
			return 1
		}
		if err := writeReports(opt, username, hits); err != nil {
			fmt.Fprintf(stdout, "ERROR:  %s\n", err)
			return 1
		}
		fmt.Fprintln(stdout)
	}
	printer.Finish()
	return 0
}

// ExpandUsernames replaces each "{?}" marker with "_", "-", and ".".
func ExpandUsernames(names []string) []string {
	var out []string
	for _, name := range names {
		if strings.Contains(name, "{?}") {
			for _, symbol := range []string{"_", "-", "."} {
				out = append(out, strings.ReplaceAll(name, "{?}", symbol))
			}
			continue
		}
		out = append(out, name)
	}
	return out
}

func loadSites(ctx context.Context, opt Options) ([]sites.Site, error) {
	if opt.Local {
		return sites.LoadLocal()
	}
	location := opt.JSONFile
	if location != "" && isDigits(location) {
		resolved, err := resolvePullRequest(ctx, location)
		if err != nil {
			return nil, err
		}
		location = resolved
	}
	return sites.Load(ctx, location, !opt.IgnoreExclusions, opt.Sites)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func resolvePullRequest(ctx context.Context, number string) (string, error) {
	rawURL := "https://api.github.com/repos/sherlock-project/sherlock/pulls/" + number
	body, err := getJSON(ctx, rawURL, 10*time.Second)
	if err != nil {
		return "", err
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", err
	}
	if _, ok := doc["message"]; ok {
		return "", &PullError{Number: number}
	}
	head, _ := doc["head"].(map[string]any)
	sha, _ := head["sha"].(string)
	if sha == "" {
		return "", &PullError{Number: number}
	}
	return "https://raw.githubusercontent.com/sherlock-project/sherlock/" + sha + "/sherlock_project/resources/data.json", nil
}

// CheckUpdate prints an upstream release notice. Failures are reported and ignored.
func CheckUpdate(ctx context.Context, out io.Writer) {
	body, err := getJSON(ctx, ReleaseURL, 10*time.Second)
	if err != nil {
		fmt.Fprintf(out, "A problem occurred while checking for an update: %s\n", err)
		return
	}
	var doc struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		fmt.Fprintf(out, "A problem occurred while checking for an update: %s\n", err)
		return
	}
	if doc.TagName == "" {
		fmt.Fprintf(out, "A problem occurred while checking for an update: %s\n", "missing tag_name")
		return
	}
	remote := doc.TagName[1:]
	if remote != version.Version {
		fmt.Fprintf(out, "Update available! %s --> %s\n%s\n", version.Version, remote, doc.HTMLURL)
	}
}

func getJSON(ctx context.Context, rawURL string, timeout time.Duration) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.UserAgent())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return body, nil
}

func writeReports(opt Options, username string, hits []search.Hit) error {
	if opt.FolderOutput != "" {
		if err := os.MkdirAll(opt.FolderOutput, 0o755); err != nil {
			return err
		}
	}
	if opt.TXT {
		path := username + ".txt"
		switch {
		case opt.Output != "":
			path = opt.Output
		case opt.FolderOutput != "":
			path = filepath.Join(opt.FolderOutput, username+".txt")
		}
		if err := output.WriteTXT(path, hits); err != nil {
			return err
		}
	}
	if opt.CSV {
		path := username + ".csv"
		if opt.FolderOutput != "" {
			path = filepath.Join(opt.FolderOutput, path)
		}
		if err := output.WriteCSV(path, username, hits, opt.PrintFound, opt.PrintAll); err != nil {
			return err
		}
	}
	if opt.XLSX {
		if err := output.WriteXLSX(username+".xlsx", username, hits, opt.PrintFound, opt.PrintAll); err != nil {
			return err
		}
	}
	return nil
}
