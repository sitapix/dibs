# dibs

[![CI](https://github.com/sitapix/dibs/actions/workflows/ci.yml/badge.svg)](https://github.com/sitapix/dibs/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

**Check domain availability across every TLD, right from your terminal.**

dibs checks whether a domain name is available across all 1400+ ICANN top-level domains. It uses DNS over HTTPS to keep your lookups off plaintext DNS, and you can optionally turn on RDAP verification to double-check results against the actual registry data. No registrar search bars, no WHOIS scraping. Just a single Go binary with zero dependencies.

```
$ dibs --verify columns

  ✓  columns.dev                     available
  ✗  columns.com                     taken
  ✓  columns.space                   available
  ✓  columns.tech                    available
  ✗  columns.org                     taken
  ━━━━━━━━━━━━━━━━ 25/25 TLDs  100%

Verifying 8 available domains via RDAP...
Found 8 available domains out of 25 checked (32%)
Verified 6 of 8 via RDAP (2 TLDs have no RDAP server)
```

## Features

- **DNS over HTTPS by default.** Queries go over HTTPS using [RFC 8484](https://datatracker.ietf.org/doc/html/rfc8484) wire format. You can fall back to system DNS with `--no-doh` if you prefer.
- **RDAP verification.** Use `--verify` to check results against the actual registry data. Catches domains that are registered but have no DNS set up.
- **Fast.** Runs 100 parallel lookups by default, and you can crank it up to 500.
- **Smart defaults.** Checks the top 25 popular TLDs out of the box (com, org, net, io, dev, app, ai, etc.)
- **All TLDs.** Or just search all 1400+ ICANN TLDs with `--all`.
- **Multiple output formats.** Terminal with colors and a progress bar, JSON, or CSV.
- **Batch processing.** Throw a list of domain names in a file and check them all at once.
- **Filtering.** Filter by TLD length, sort alphabetically or by length, limit how many you check.
- **Configurable.** Drop a config file at `~/.config/dibs/config` and your defaults are set.
- **Zero dependencies.** Pure Go standard library, single binary, nothing else to install.

## Installation

Pre-built binaries are available for macOS (arm64, amd64) and Linux (amd64, arm64).

### Homebrew (macOS and Linux)

```bash
brew install sitapix/dibs/dibs
```

### Shell installer

Downloads the latest binary, verifies its checksum, and installs it:

```bash
curl -fsSL https://raw.githubusercontent.com/sitapix/dibs/main/install.sh | bash
```

Or review the script first:

```bash
curl -fsSL https://raw.githubusercontent.com/sitapix/dibs/main/install.sh -o install.sh
less install.sh
bash install.sh
```

### From source

Requires Go 1.26+.

```bash
git clone https://github.com/sitapix/dibs.git
cd dibs
make build
```

Or with `go install`:

```bash
go install github.com/sitapix/dibs@latest
```

### Download binary

Grab the latest binary for your platform from the [releases page](https://github.com/sitapix/dibs/releases), then:

```bash
chmod +x dibs-*
sudo mv dibs-* /usr/local/bin/dibs
```

## Usage

```bash
# check top 25 popular TLDs
dibs mybrand

# check ALL 1400+ TLDs
dibs --all mybrand

# verify available domains against registry data
dibs --verify mybrand

# both
dibs --all --verify mybrand

# only show available domains
dibs --quiet mybrand

# interactive mode (prompts for domain name)
dibs
```

### Output formats

```bash
# JSON (for scripting)
dibs --json mybrand

# CSV (for spreadsheets)
dibs --csv mybrand

# JSON with verification data
dibs --json --verify mybrand

# disable colors (also respects NO_COLOR env var)
dibs --no-color mybrand
```

### Filtering

```bash
# only short TLDs (2-3 characters)
dibs --max-length 3 mybrand

# only TLDs with 4+ characters
dibs --min-length 4 mybrand

# specific TLDs only
dibs --tlds com,io,dev,app mybrand

# sort results
dibs --sort alpha mybrand
dibs --sort length mybrand

# limit how many TLDs to check
dibs --limit 50 --all mybrand
```

### Batch mode

```bash
dibs --file domains.txt
```

`domains.txt` format (one per line, `#` comments supported):
```
mybrand
myproject
mycompany
```

### Performance

```bash
# increase parallel connections (default: 100, max: 500)
dibs --parallel 200 --all mybrand

# adjust timeout per query (default: 5s)
dibs --timeout 3 mybrand

# retry failed DNS queries (default: 1)
dibs --retries 3 --all mybrand

# force refresh cached TLD list
dibs --refresh --all mybrand
```

### DNS providers

```bash
# use Mullvad DoH instead of Quad9 (default)
dibs --provider mullvad mybrand

# use Cloudflare or Google
dibs --provider cloudflare mybrand
dibs --provider google mybrand

# rotate between all providers
dibs --rotate mybrand

# use your own DoH server
dibs --doh-url https://dns.example.com/dns-query mybrand

# use system DNS instead of DoH (faster, plaintext)
dibs --no-doh mybrand
```

## How it works

### DNS scan

1. Grabs the official TLD list from [IANA](https://data.iana.org/TLD/tlds-alpha-by-domain.txt)
2. Caches it locally for 24 hours (`~/.cache/dibs/tlds.txt`)
3. Fires off parallel DNS A-record lookups via DoH ([RFC 8484](https://datatracker.ietf.org/doc/html/rfc8484) wire format) or system DNS
4. NXDOMAIN means available, NOERROR means taken

DNS is fast but it's really just a first pass. A domain can be registered without having any DNS set up, so it would look available when it's actually not.

### RDAP verification (`--verify`)

With `--verify`, dibs goes back and double-checks the domains that DNS said were available:

1. Grabs the [IANA RDAP bootstrap](https://data.iana.org/rdap/dns.json) (also cached for 24 hours)
2. For each "available" domain, asks the TLD's registry directly over HTTPS ([RFC 7480](https://datatracker.ietf.org/doc/html/rfc7480))
3. If the registry says it exists (HTTP 200), it gets corrected to taken. If the registry says it doesn't (HTTP 404), it's confirmed available.

dibs talks to registries directly using the IANA bootstrap files, not through a redirect service. All gTLDs support RDAP, but some ccTLDs don't, so those results stay unverified. See [rdap.org](https://about.rdap.org) if you want to learn more about the protocol.

### Can a domain pass both checks and still be taken?

Yes. Sometimes a domain looks available in both DNS and RDAP but you still can't register it. Registries can hold names back for premium pricing, trademark protection (TMCH), or other policy reasons without ever creating DNS records or RDAP entries for them. When you go to actually buy one of these, the registrar will either reject it or hit you with a higher price. dibs tells you what's probably available, but the registrar always has the final say.

## DoH providers

| Provider | Default | Notes |
|----------|---------|-------|
| **Quad9** | Yes | Non-profit. [quad9.net](https://quad9.net) |
| **Mullvad** | | [mullvad.net](https://mullvad.net/en/help/dns-over-https-and-dns-over-tls) |
| **Cloudflare** | | [1.1.1.1](https://developers.cloudflare.com/1.1.1.1/) |
| **Google** | | [dns.google](https://developers.google.com/speed/public-dns) |

Use `--rotate` to spread queries across all four providers, or `--no-doh` to skip DoH entirely and use system DNS (faster, but plaintext).

## Configuration

You can create a config file at `~/.config/dibs/config` to set your defaults:

```
# max concurrent DNS queries (default: 100, max: 500)
parallel=100

# DNS query timeout in seconds (default: 5)
timeout=5

# retry count on error (default: 1)
retries=1

# DoH provider: quad9, mullvad, cloudflare, google (default: quad9)
provider=quad9
```

Only the keys shown above are supported — unknown keys will produce an error.
CLI flags override config file values.

## Contributing

Contributions are welcome! Check out [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License. See [LICENSE](LICENSE) for details.
