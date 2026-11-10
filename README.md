<p align="center">
  <br>
  <a href="https://sherlockproject.xyz/" target="_blank"><img src="docs/images/sherlock-logo.png" alt="sherlock"/></a>
  <br>
  <span>Hunt down social media accounts by username across <a href="https://sherlockproject.xyz/sites">400+ social networks</a></span>
  <br>
</p>

This repository is a **Go port** of the [Sherlock Project](https://github.com/sherlock-project/sherlock), originally created by [Siddharth Dushantha](https://github.com/sdushantha). The site list, probe rules, and command-line flags follow that project. The original work is MIT licensed; this port keeps the same license. See [LICENSE](LICENSE).

## Installation

Requires Go 1.22 or newer.

```bash
go install github.com/Nurdich/sherlock/cmd/sherlock@latest
```

Or build from a checkout:

```bash
go build -o sherlock ./cmd/sherlock
./sherlock --help
```

Docker:

```bash
docker build -t sherlock .
docker run --rm sherlock --help
docker run --rm sherlock user123
```

## Usage

Search one username:

```bash
sherlock user123
```

Search several usernames:

```bash
sherlock user1 user2 user3
```

`{?}` expands to `_`, `-`, and `.`:

```bash
sherlock "user{?}name"
```

Useful flags (run `sherlock --help` for the full list):

| Flag | Meaning |
| --- | --- |
| `--site SITE_NAME` | Check only the named sites. Repeat the flag for more than one. |
| `--local`, `-l` | Use the site list embedded in the binary instead of the upstream manifest. |
| `--json`, `-j` | Load `data.json` from a file, a URL, or an upstream pull-request number. |
| `--nsfw` | Include NSFW sites. Named `--site` values are kept even without this flag. |
| `--ignore-exclusions` | Do not apply the upstream false-positive exclusion list. |
| `--proxy`, `-p` | Send probes through an HTTP(S) or SOCKS proxy. |
| `--timeout` | Per-request timeout in seconds (default 60). |
| `--print-all` | Also print sites where the username was not found. |
| `--no-color` | Plain text output. |
| `--txt` | Write claimed profile URLs to a text file. |
| `--csv` | Write a CSV report. |
| `--xlsx` | Write an Excel workbook. |
| `--output`, `-o` | Text output path for a single username (requires `--txt`). |
| `--folderoutput`, `-fo` | Directory for per-username text and CSV reports. |
| `--verbose`, `-v`, `-d`, `--debug` | Include response times. |
| `--dump-response` | Print raw HTTP responses for debugging. |
| `--browse`, `-b` | Open claimed profiles in the default browser. |

By default the tool downloads the current manifest from `https://data.sherlockproject.xyz` and the exclusion list maintained by the upstream project. `--local` uses the copy of `resources/data.json` embedded at build time, which is the same manifest this repository ships.

Claimed URLs are printed as they are found. Text, CSV, and spreadsheet files are written only when `--txt`, `--csv`, or `--xlsx` is set.

## Development

```bash
go test ./...
go build -o sherlock ./cmd/sherlock
```

Live network checks against real sites are optional:

```bash
go test -tags online ./...
```

`cmd/sitelist` rewrites `output/sites.mdx` from `resources/data.json`.

## Intentional differences from the Python CLI

- The program is a Go module (`cmd/sherlock` plus `internal` packages) instead of a Python package.
- `--help` matches the Python argparse text for an 80-column terminal, including the same flag names.
- `--local` reads the embedded manifest, so the binary does not need the JSON file beside it.
- CSV files use LF line endings.
- Terminal color is ANSI. `--no-color` disables it.
- `.xlsx` files contain the same columns and `=HYPERLINK("...")` values as the Python exporter.
- `--json 123` still means upstream pull request 123 in `sherlock-project/sherlock`.

## Credits

Thank you to everyone who has contributed to the original Sherlock Project.

MIT © Sherlock Project  
Creator — [Siddharth Dushantha](https://github.com/sdushantha)
