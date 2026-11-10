package output

import (
	"archive/zip"
	"encoding/csv"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nurdich/sherlock/internal/result"
	"github.com/Nurdich/sherlock/internal/search"
)

func sampleHits() []search.Hit {
	secs := 0.5
	return []search.Hit{
		{
			Name: "GitHub", URLMain: "https://github.com/", URLUser: "https://github.com/blue",
			HTTPStatus: "200", Result: result.Result{Status: result.Claimed, QueryTime: &secs},
		},
		{
			Name: "GitLab", URLMain: "https://gitlab.com/", URLUser: "https://gitlab.com/missing",
			HTTPStatus: "404", Result: result.Result{Status: result.Available},
		},
	}
}

func TestTXTAndCSVFilter(t *testing.T) {
	dir := t.TempDir()
	hits := sampleHits()
	txt := filepath.Join(dir, "blue.txt")
	if err := WriteTXT(txt, hits); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(txt)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "https://github.com/blue\nTotal Websites Username Detected On : 1\n" {
		t.Fatalf("txt = %q", body)
	}

	csvPath := filepath.Join(dir, "blue.csv")
	if err := WriteCSV(csvPath, "blue", hits, true, false); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %#v", rows)
	}
	if rows[0][0] != "username" || rows[1][1] != "GitHub" || rows[1][4] != "Claimed" || rows[1][6] != "0.5" {
		t.Fatalf("csv = %#v", rows)
	}

	allPath := filepath.Join(dir, "all.csv")
	if err := WriteCSV(allPath, "blue", hits, true, true); err != nil {
		t.Fatal(err)
	}
	f2, err := os.Open(allPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()
	all, err := csv.NewReader(f2).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[2][4] != "Available" || all[2][6] != "" {
		t.Fatalf("all csv = %#v", all)
	}
}

func TestXLSX(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blue.xlsx")
	if err := WriteXLSX(path, "blue", sampleHits(), true, false); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var sheet string
	for _, file := range zr.File {
		if file.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		sheet = string(body)
	}
	if sheet == "" {
		t.Fatal("missing sheet")
	}
	for _, want := range []string{`name="sheet1"`, "username", "GitHub", `=HYPERLINK("https://github.com/blue")`, "Claimed"} {
		if want == `name="sheet1"` {
			continue
		}
		if !strings.Contains(sheet, want) {
			t.Fatalf("sheet missing %s\n%s", want, sheet)
		}
	}
	wb := mustZip(t, path, "xl/workbook.xml")
	if !strings.Contains(wb, `name="sheet1"`) {
		t.Fatalf("workbook = %s", wb)
	}
	if strings.Contains(sheet, "GitLab") {
		t.Fatal("xlsx included a non-claimed row")
	}
}

func mustZip(t *testing.T, path, name string) string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, file := range zr.File {
		if file.Name != name {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(rc)
		rc.Close()
		return string(body)
	}
	t.Fatalf("missing %s", name)
	return ""
}
