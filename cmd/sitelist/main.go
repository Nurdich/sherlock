// Copyright (c) 2019 Sherlock Project
// SPDX-License-Identifier: MIT

// Command sitelist regenerates output/sites.mdx from resources/data.json
// and rewrites that manifest with sorted keys.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode/utf16"
)

const dataPath = "resources/data.json"

func main() {
	raw, err := os.ReadFile(dataPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read manifest: %v\n", err)
		os.Exit(1)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		fmt.Fprintf(os.Stderr, "parse manifest: %v\n", err)
		os.Exit(1)
	}
	delete(data, "$schema")
	names := make([]string, 0, len(data))
	for name := range data {
		names = append(names, name)
	}
	sort.Strings(names)

	if err := os.MkdirAll("output", 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}
	var page strings.Builder
	page.WriteString("---\n")
	page.WriteString("title: 'List of supported sites'\n")
	page.WriteString("sidebarTitle: 'Supported sites'\n")
	page.WriteString("icon: 'globe'\n")
	page.WriteString("description: 'Sherlock currently supports **400+** sites'\n")
	page.WriteString("---\n\n")
	for _, name := range names {
		info, _ := data[name].(map[string]any)
		urlMain, _ := info["urlMain"].(string)
		nsfw := ""
		if v, _ := info["isNSFW"].(bool); v {
			nsfw = "**(NSFW)**"
		}
		fmt.Fprintf(&page, "1. [%s](%s) %s\n", name, urlMain, nsfw)
	}
	if err := os.WriteFile("output/sites.mdx", []byte(page.String()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write site list: %v\n", err)
		os.Exit(1)
	}

	original := map[string]any{}
	if err := json.Unmarshal(raw, &original); err != nil {
		fmt.Fprintf(os.Stderr, "parse manifest: %v\n", err)
		os.Exit(1)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(original); err != nil {
		fmt.Fprintf(os.Stderr, "encode manifest: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(dataPath, []byte(escapeNonASCII(buf.String())), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write manifest: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Finished updating supported site listing!")
}

func escapeNonASCII(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 128:
			b.WriteRune(r)
		case r <= 0xFFFF:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			u := utf16.Encode([]rune{r})
			for _, unit := range u {
				fmt.Fprintf(&b, `\u%04x`, unit)
			}
		}
	}
	return b.String()
}
