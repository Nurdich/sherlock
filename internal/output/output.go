// Copyright (c) 2019 Sherlock Project
// SPDX-License-Identifier: MIT

// Package output writes Sherlock results to text, CSV, and spreadsheet files.
package output

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Nurdich/sherlock/internal/result"
	"github.com/Nurdich/sherlock/internal/search"
)

var headers = []string{"username", "name", "url_main", "url_user", "exists", "http_status", "response_time_s"}

// Include reports whether a hit belongs in CSV/XLSX output.
// This matches the Python CLI: --print-found defaults to true and cannot be
// turned off, so non-claimed rows are skipped unless --print-all is set.
func Include(printFound, printAll bool, status result.Status) bool {
	if printFound && !printAll && status != result.Claimed {
		return false
	}
	return true
}

// WriteTXT writes claimed profile URLs and a total, matching --txt.
func WriteTXT(path string, hits []search.Hit) error {
	var b strings.Builder
	count := 0
	for _, hit := range hits {
		if hit.Result.Status != result.Claimed {
			continue
		}
		count++
		b.WriteString(hit.URLUser)
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "Total Websites Username Detected On : %d\n", count)
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// WriteCSV writes a CSV report for one username.
func WriteCSV(path, username string, hits []search.Hit, printFound, printAll bool) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	w := csv.NewWriter(file)
	if err := w.Write(headers); err != nil {
		return err
	}
	for _, hit := range hits {
		if !Include(printFound, printAll, hit.Result.Status) {
			continue
		}
		rt := ""
		if hit.Result.QueryTime != nil {
			rt = strconv.FormatFloat(*hit.Result.QueryTime, 'f', -1, 64)
		}
		if err := w.Write([]string{
			username,
			hit.Name,
			hit.URLMain,
			hit.URLUser,
			hit.Result.Status.String(),
			hit.HTTPStatus,
			rt,
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// WriteXLSX writes a single-sheet workbook named sheet1.
// Profile URLs are stored as =HYPERLINK("...") text, matching the Python exporter.
func WriteXLSX(path, username string, hits []search.Hit, printFound, printAll bool) error {
	var rows [][]cell
	header := make([]cell, len(headers))
	for i, name := range headers {
		header[i] = textCell(name)
	}
	rows = append(rows, header)
	for _, hit := range hits {
		if !Include(printFound, printAll, hit.Result.Status) {
			continue
		}
		rt := textCell("")
		if hit.Result.QueryTime != nil {
			rt = numberCell(strconv.FormatFloat(*hit.Result.QueryTime, 'f', -1, 64))
		}
		status := textCell(hit.HTTPStatus)
		if _, err := strconv.Atoi(hit.HTTPStatus); err == nil {
			status = numberCell(hit.HTTPStatus)
		}
		rows = append(rows, []cell{
			textCell(username),
			textCell(hit.Name),
			textCell(hyperlink(hit.URLMain)),
			textCell(hyperlink(hit.URLUser)),
			textCell(hit.Result.Status.String()),
			status,
			rt,
		})
	}
	return writeXLSX(path, rows)
}

func hyperlink(raw string) string {
	return `=HYPERLINK("` + raw + `")`
}

type cell struct {
	value  string
	number bool
}

func textCell(v string) cell   { return cell{value: v} }
func numberCell(v string) cell { return cell{value: v, number: true} }

func writeXLSX(path string, rows [][]cell) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	zw := zip.NewWriter(file)
	files := []struct {
		name string
		body string
	}{
		{"[Content_Types].xml", contentTypesXML},
		{"_rels/.rels", relsXML},
		{"xl/workbook.xml", workbookXML},
		{"xl/_rels/workbook.xml.rels", workbookRelsXML},
		{"xl/worksheets/sheet1.xml", sheetXML(rows)},
	}
	for _, item := range files {
		w, err := zw.Create(item.name)
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte(item.body)); err != nil {
			return err
		}
	}
	return zw.Close()
}

func sheetXML(rows [][]cell) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for r, row := range rows {
		fmt.Fprintf(&b, `<row r="%d">`, r+1)
		for c, item := range row {
			ref := colName(c+1) + strconv.Itoa(r+1)
			if item.number {
				fmt.Fprintf(&b, `<c r="%s"><v>%s</v></c>`, ref, xmlEscape(item.value))
				continue
			}
			fmt.Fprintf(&b, `<c r="%s" t="inlineStr"><is><t>%s</t></is></c>`, ref, xmlEscape(item.value))
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData></worksheet>`)
	return b.String()
}

func colName(n int) string {
	s := ""
	for n > 0 {
		n--
		s = string(rune('A'+n%26)) + s
		n /= 26
	}
	return s
}

func xmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return replacer.Replace(s)
}

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
	`<Default Extension="xml" ContentType="application/xml"/>` +
	`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
	`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
	`</Types>`

const relsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
	`</Relationships>`

const workbookXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
	`<sheets><sheet name="sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>`

const workbookRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>` +
	`</Relationships>`
