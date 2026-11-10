// Copyright (c) 2019 Sherlock Project
// SPDX-License-Identifier: MIT

// Package notify prints Sherlock query progress.
package notify

import (
	"fmt"
	"io"
	"math"
	"os/exec"
	"runtime"

	"github.com/Nurdich/sherlock/internal/result"
)

const (
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiWhite  = "\033[37m"
	ansiRed    = "\033[31m"
	ansiBright = "\033[1m"
	ansiReset  = "\033[0m"
)

// Notifier receives query lifecycle events.
type Notifier interface {
	Start(username string)
	Update(result.Result)
	Finish()
}

// OpenBrowser launches a URL when --browse is set.
var OpenBrowser = openBrowser

// Printer writes the classic Sherlock terminal report.
type Printer struct {
	out       io.Writer
	verbose   bool
	printAll  bool
	browse    bool
	color     bool
	claimed   int
	sawUpdate bool
}

// Options controls terminal reporting.
type Options struct {
	Verbose  bool
	PrintAll bool
	Browse   bool
	Color    bool
}

// NewPrinter returns a terminal reporter.
func NewPrinter(out io.Writer, opt Options) *Printer {
	return &Printer{
		out:      out,
		verbose:  opt.Verbose,
		printAll: opt.PrintAll,
		browse:   opt.Browse,
		color:    opt.Color,
	}
}

// Start prints the banner for one username.
func (p *Printer) Start(username string) {
	title := "Checking username"
	line := "[*] " + title + " " + username + " on:"
	if p.color {
		line = ansiBright + ansiGreen + "[" + ansiYellow + "*" + ansiGreen + "] " + title +
			ansiWhite + " " + username + ansiGreen + " on:" + ansiReset
	}
	fmt.Fprintln(p.out, line)
	fmt.Fprintln(p.out)
}

// Update prints one site result.
func (p *Printer) Update(res result.Result) {
	p.sawUpdate = true
	responseTime := ""
	if res.QueryTime != nil && p.verbose {
		ms := int(math.Round(*res.QueryTime * 1000))
		responseTime = fmt.Sprintf(" [%dms]", ms)
	}
	switch res.Status {
	case result.Claimed:
		p.claimed++
		line := "[+]" + responseTime + " " + res.SiteName + ": " + res.SiteURLUser
		if p.color {
			line = ansiBright + ansiWhite + "[" + ansiGreen + "+" + ansiWhite + "]" + responseTime +
				ansiGreen + " " + res.SiteName + ": " + ansiReset + res.SiteURLUser
		}
		fmt.Fprintln(p.out, line)
		if p.browse && OpenBrowser != nil {
			_ = OpenBrowser(res.SiteURLUser)
		}
	case result.Available:
		if p.printAll {
			line := "[-]" + responseTime + " " + res.SiteName + ": Not Found!"
			if p.color {
				line = ansiBright + ansiWhite + "[" + ansiRed + "-" + ansiWhite + "]" + responseTime +
					ansiGreen + " " + res.SiteName + ":" + ansiYellow + " Not Found!" + ansiReset
			}
			fmt.Fprintln(p.out, line)
		}
	case result.Unknown:
		if p.printAll {
			ctx := ""
			if res.Context != nil {
				ctx = *res.Context
			}
			line := "[-] " + res.SiteName + ": " + ctx
			if p.color {
				line = ansiBright + ansiWhite + "[" + ansiRed + "-" + ansiWhite + "]" +
					ansiGreen + " " + res.SiteName + ":" + ansiRed + " " + ctx + ansiYellow + " " + ansiReset
			}
			fmt.Fprintln(p.out, line)
		}
	case result.Illegal:
		if p.printAll {
			msg := "Illegal Username Format For This Site!"
			line := "[-] " + res.SiteName + ": " + msg
			if p.color {
				line = ansiBright + ansiWhite + "[" + ansiRed + "-" + ansiWhite + "]" +
					ansiGreen + " " + res.SiteName + ":" + ansiYellow + " " + msg + ansiReset
			}
			fmt.Fprintln(p.out, line)
		}
	case result.WAF:
		if p.printAll {
			line := "[-] " + res.SiteName + ": Blocked by bot detection (proxy may help)"
			if p.color {
				line = ansiBright + ansiWhite + "[" + ansiRed + "-" + ansiWhite + "]" +
					ansiGreen + " " + res.SiteName + ":" + ansiRed + " Blocked by bot detection" +
					ansiYellow + " (proxy may help)" + ansiReset
			}
			fmt.Fprintln(p.out, line)
		}
	default:
		fmt.Fprintf(p.out, "Unknown Query Status '%s' for site '%s'\n", res.Status, res.SiteName)
	}
}

// Finish prints the summary line and the upstream project note.
func (p *Printer) Finish() {
	n := p.claimed
	line := fmt.Sprintf("[*] Search completed with %d results", n)
	if p.color {
		line = ansiBright + ansiGreen + "[" + ansiYellow + "*" + ansiGreen + "] Search completed with" +
			ansiWhite + fmt.Sprintf(" %d ", n) + ansiGreen + "results" + ansiReset
	}
	fmt.Fprintln(p.out, line)
	fmt.Fprint(p.out, "\nGo deeper than a username. Explore public profiles and export your findings.\n")
	if p.color {
		fmt.Fprint(p.out, "Try OSINTSearch: \033]8;;https://osintsearch.org/go/sherlock\033\\https://osintsearch.org\033]8;;\033\\\n")
	} else {
		fmt.Fprintln(p.out, "Try OSINTSearch: https://osintsearch.org")
	}
}

// ClaimedCount is the number of claimed hits reported so far.
func (p *Printer) ClaimedCount() int { return p.claimed }

func openBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}
