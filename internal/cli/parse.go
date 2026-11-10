// Copyright (c) 2019 Sherlock Project
// SPDX-License-Identifier: MIT

package cli

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/Nurdich/sherlock/internal/version"
)

const exitUsage = 2

// Options is the parsed Sherlock command line.
type Options struct {
	Verbose          bool
	FolderOutput     string
	Output           string
	CSV              bool
	XLSX             bool
	Sites            []string
	Proxy            string
	DumpResponse     bool
	JSONFile         string
	Timeout          float64
	PrintAll         bool
	PrintFound       bool
	NoColor          bool
	Usernames        []string
	Browse           bool
	Local            bool
	NSFW             bool
	TXT              bool
	IgnoreExclusions bool
}

type flagKind int

const (
	kindBool flagKind = iota
	kindString
	kindAppend
	kindFloat
	kindHelp
	kindVersion
)

type flagDef struct {
	names []string
	kind  flagKind
	field string
}

var flagDefs = []flagDef{
	{names: []string{"--version"}, kind: kindVersion},
	{names: []string{"--verbose", "-v", "-d", "--debug"}, kind: kindBool, field: "verbose"},
	{names: []string{"--folderoutput", "-fo"}, kind: kindString, field: "folderoutput"},
	{names: []string{"--output", "-o"}, kind: kindString, field: "output"},
	{names: []string{"--csv"}, kind: kindBool, field: "csv"},
	{names: []string{"--xlsx"}, kind: kindBool, field: "xlsx"},
	{names: []string{"--site"}, kind: kindAppend, field: "site"},
	{names: []string{"--proxy", "-p"}, kind: kindString, field: "proxy"},
	{names: []string{"--dump-response"}, kind: kindBool, field: "dump"},
	{names: []string{"--json", "-j"}, kind: kindString, field: "json"},
	{names: []string{"--timeout"}, kind: kindFloat, field: "timeout"},
	{names: []string{"--print-all"}, kind: kindBool, field: "printall"},
	{names: []string{"--print-found"}, kind: kindBool, field: "printfound"},
	{names: []string{"--no-color"}, kind: kindBool, field: "nocolor"},
	{names: []string{"--browse", "-b"}, kind: kindBool, field: "browse"},
	{names: []string{"--local", "-l"}, kind: kindBool, field: "local"},
	{names: []string{"--nsfw"}, kind: kindBool, field: "nsfw"},
	{names: []string{"--txt"}, kind: kindBool, field: "txt"},
	{names: []string{"--ignore-exclusions"}, kind: kindBool, field: "ignore"},
	{names: []string{"-h", "--help"}, kind: kindHelp},
}

var negativeNumber = regexp.MustCompile(`^-\d+$|^-\d*\.\d+$`)

// Parse reads arguments the way the Python argparse CLI does.
// halt is true when the process should exit with code (help, version, or usage error).
func Parse(args []string, stdout, stderr io.Writer) (Options, int, bool) {
	opt := Options{Timeout: 60, PrintFound: true}
	exact := map[string]*flagDef{}
	for i := range flagDefs {
		for _, name := range flagDefs[i].names {
			exact[name] = &flagDefs[i]
		}
	}

	var extras []string
	end := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if end {
			opt.Usernames = append(opt.Usernames, arg)
			continue
		}
		if arg == "--" {
			end = true
			continue
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") || negativeNumber.MatchString(arg) {
			opt.Usernames = append(opt.Usernames, arg)
			continue
		}

		resolved, explicit, hasExplicit, unknown, errMsg := resolve(arg, exact)
		if errMsg != "" {
			return opt, usageError(stderr, errMsg), true
		}
		if unknown {
			extras = append(extras, arg)
			continue
		}
		if resolved.def.kind == kindHelp {
			fmt.Fprint(stdout, formatHelp(version.Version))
			return opt, 0, true
		}
		if resolved.def.kind == kindVersion {
			if hasExplicit {
				return opt, usageError(stderr, fmt.Sprintf("argument %s: ignored explicit argument '%s'", resolved.def.display(), explicit)), true
			}
			fmt.Fprintf(stdout, "%s v%s\n", version.ShortName, version.Version)
			return opt, 0, true
		}

		if resolved.def.kind == kindBool {
			if hasExplicit {
				rest := explicit
				name := resolved.matched
				if strings.HasPrefix(name, "--") || strings.HasPrefix(rest, "-") || strings.HasPrefix(rest, "=") {
					return opt, usageError(stderr, fmt.Sprintf("argument %s: ignored explicit argument '%s'", name, rest)), true
				}
				if err := opt.set(resolved.field, "true"); err != nil {
					return opt, usageError(stderr, err.Error()), true
				}
				for rest != "" {
					name = "-" + rest[:1]
					rest = rest[1:]
					next, ok := exact[name]
					if !ok {
						extras = append(extras, name+rest)
						break
					}
					if next.kind == kindBool {
						if err := opt.set(next.field, "true"); err != nil {
							return opt, usageError(stderr, err.Error()), true
						}
						continue
					}
					if next.kind == kindHelp {
						fmt.Fprint(stdout, formatHelp(version.Version))
						return opt, 0, true
					}
					if next.kind == kindVersion {
						fmt.Fprintf(stdout, "%s v%s\n", version.ShortName, version.Version)
						return opt, 0, true
					}
					value := rest
					if value == "" {
						if i+1 >= len(args) {
							return opt, usageError(stderr, fmt.Sprintf("argument %s: expected one argument", next.display())), true
						}
						i++
						value = args[i]
					}
					if err := opt.set(next.field, value); err != nil {
						return opt, usageError(stderr, fmt.Sprintf("argument %s: %s", next.display(), err.Error())), true
					}
					break
				}
				continue
			}
			if err := opt.set(resolved.field, "true"); err != nil {
				return opt, usageError(stderr, err.Error()), true
			}
			continue
		}

		value := explicit
		if !hasExplicit {
			if i+1 >= len(args) || isOptionToken(args[i+1]) {
				return opt, usageError(stderr, fmt.Sprintf("argument %s: expected one argument", resolved.def.display())), true
			}
			i++
			value = args[i]
		}
		if err := opt.set(resolved.field, value); err != nil {
			return opt, usageError(stderr, fmt.Sprintf("argument %s: %s", resolved.def.display(), err.Error())), true
		}
	}

	if len(opt.Usernames) == 0 {
		return opt, usageError(stderr, "the following arguments are required: USERNAMES"), true
	}
	if len(extras) > 0 {
		return opt, usageError(stderr, "unrecognized arguments: "+strings.Join(extras, " ")), true
	}
	return opt, 0, false
}

type resolvedFlag struct {
	def     *flagDef
	field   string
	matched string
}

func (f *flagDef) display() string { return strings.Join(f.names, "/") }

func resolve(arg string, exact map[string]*flagDef) (resolvedFlag, string, bool, bool, string) {
	if def, ok := exact[arg]; ok {
		return resolvedFlag{def: def, field: def.field, matched: arg}, "", false, false, ""
	}
	name, explicit, hasEq := strings.Cut(arg, "=")
	if hasEq {
		if def, ok := exact[name]; ok {
			return resolvedFlag{def: def, field: def.field, matched: name}, explicit, true, false, ""
		}
	}
	if strings.HasPrefix(arg, "--") {
		prefix := name
		var matches []resolvedFlag
		seen := map[*flagDef]struct{}{}
		for i := range flagDefs {
			def := &flagDefs[i]
			for _, optName := range def.names {
				if !strings.HasPrefix(optName, "--") || !strings.HasPrefix(optName, prefix) {
					continue
				}
				if _, ok := seen[def]; ok {
					continue
				}
				seen[def] = struct{}{}
				matches = append(matches, resolvedFlag{def: def, field: def.field, matched: optName})
			}
		}
		if len(matches) > 1 {
			names := make([]string, len(matches))
			for i, match := range matches {
				names[i] = match.matched
			}
			return resolvedFlag{}, "", false, false, fmt.Sprintf("ambiguous option: %s could match %s", arg, strings.Join(names, ", "))
		}
		if len(matches) == 1 {
			return matches[0], explicit, hasEq, false, ""
		}
		return resolvedFlag{}, "", false, true, ""
	}

	if len(arg) >= 2 {
		prefix2 := arg[:2]
		rest := arg[2:]
		if def, ok := exact[prefix2]; ok && !hasEq {
			exp := rest
			has := rest != ""
			return resolvedFlag{def: def, field: def.field, matched: prefix2}, exp, has, false, ""
		}
		var matches []resolvedFlag
		for i := range flagDefs {
			def := &flagDefs[i]
			for _, optName := range def.names {
				if strings.HasPrefix(optName, arg) && optName != arg && !strings.HasPrefix(optName, "--") {
					matches = append(matches, resolvedFlag{def: def, field: def.field, matched: optName})
				}
			}
		}
		if len(matches) > 1 {
			names := make([]string, len(matches))
			for i, match := range matches {
				names[i] = match.matched
			}
			return resolvedFlag{}, "", false, false, fmt.Sprintf("ambiguous option: %s could match %s", arg, strings.Join(names, ", "))
		}
		if len(matches) == 1 {
			return matches[0], "", false, false, ""
		}
	}
	return resolvedFlag{}, "", false, true, ""
}

func isOptionToken(arg string) bool {
	if arg == "--" || arg == "-" {
		return true
	}
	if negativeNumber.MatchString(arg) {
		return false
	}
	return strings.HasPrefix(arg, "-")
}

func (o *Options) set(field, value string) error {
	switch field {
	case "verbose":
		o.Verbose = true
	case "folderoutput":
		o.FolderOutput = value
	case "output":
		o.Output = value
	case "csv":
		o.CSV = true
	case "xlsx":
		o.XLSX = true
	case "site":
		o.Sites = append(o.Sites, value)
	case "proxy":
		o.Proxy = value
	case "dump":
		o.DumpResponse = true
	case "json":
		o.JSONFile = value
	case "timeout":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid float value: '%s'", value)
		}
		if f <= 0 {
			return fmt.Errorf("Invalid timeout value: %s. Timeout must be a positive number.", value)
		}
		o.Timeout = f
	case "printall":
		o.PrintAll = true
	case "printfound":
		o.PrintFound = true
	case "nocolor":
		o.NoColor = true
	case "browse":
		o.Browse = true
	case "local":
		o.Local = true
	case "nsfw":
		o.NSFW = true
	case "txt":
		o.TXT = true
	case "ignore":
		o.IgnoreExclusions = true
	default:
		return fmt.Errorf("unknown field %s", field)
	}
	return nil
}

func usageError(stderr io.Writer, message string) int {
	fmt.Fprint(stderr, usageText)
	fmt.Fprintf(stderr, "sherlock: error: %s\n", message)
	return exitUsage
}
