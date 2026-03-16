package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/sitapix/dibs/config"
	"github.com/sitapix/dibs/dns"
	"github.com/sitapix/dibs/output"
	"github.com/sitapix/dibs/rdap"
	"github.com/sitapix/dibs/tlds"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

const (
	appName    = "dibs"
	retryDelay = 500 * time.Millisecond
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Stdin))
}

// reorderArgs moves positional arguments to the end so flags can appear
// after the domain name (e.g. "dibs mybrand --json" works like "dibs --json mybrand").
func reorderArgs(args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			// Everything after -- is positional.
			positional = append(positional, args[i:]...)
			break
		}
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			// If this flag takes a value (e.g. -provider cloudflare), grab the next arg too.
			// Flags that take values have the form -name or --name without '='.
			if !strings.Contains(arg, "=") {
				// Check if this is a known value-taking flag.
				name := strings.TrimLeft(arg, "-")
				switch name {
				case "provider", "file", "sort", "tlds", "doh-url", "retries",
					"limit", "parallel", "timeout", "min-length", "max-length":
					if i+1 < len(args) {
						i++
						flags = append(flags, args[i])
					}
				}
			}
		} else {
			positional = append(positional, arg)
		}
	}
	return append(flags, positional...)
}

func run(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	args = reorderArgs(args)

	// ── 1. Flag setup ──────────────────────────────────────────────────
	fs := flag.NewFlagSet(appName, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: dibs [options] <domain-name>\n")
		fmt.Fprintf(stderr, "Check if a domain name is up for grabs across 1400+ TLDs.\n\n")
		fmt.Fprintf(stderr, "Options:\n")
		fs.PrintDefaults()
	}

	// ── 2. Define flags ────────────────────────────────────────────────
	flagAll := fs.Bool("all", false, "Check all IANA TLDs (not just Top 25)")
	flagLimit := fs.Int("limit", 0, "Limit number of TLDs checked")
	flagTLDs := fs.String("tlds", "", "Comma-separated list of TLDs to check")
	flagJSON := fs.Bool("json", false, "Output results as JSON")
	flagCSV := fs.Bool("csv", false, "Output results as CSV")
	flagQuiet := fs.Bool("quiet", false, "Only show available domains")
	flagProvider := fs.String("provider", "", "DoH provider: quad9, mullvad, cloudflare, google")
	flagRotate := fs.Bool("rotate", false, "Rotate between DoH providers")
	flagNoDOH := fs.Bool("no-doh", false, "Use system DNS resolver instead of DoH")
	flagNoColor := fs.Bool("no-color", false, "Disable ANSI color output")
	flagParallel := fs.Int("parallel", 0, "Number of parallel DNS queries")
	flagTimeout := fs.Int("timeout", 0, "DNS query timeout in seconds")
	flagFile := fs.String("file", "", "Read domain names from file (one per line)")
	flagMinLength := fs.Int("min-length", 0, "Minimum TLD length to check")
	flagMaxLength := fs.Int("max-length", 0, "Maximum TLD length to check")
	flagSort := fs.String("sort", "", "Sort TLDs: alpha, length")
	flagRetries := fs.Int("retries", 0, "Number of retries on DNS error")
	flagRefresh := fs.Bool("refresh", false, "Force refresh of cached TLD list")
	flagDohURL := fs.String("doh-url", "", "Custom DoH resolver URL")
	flagVerify := fs.Bool("verify", false, "Verify available domains via RDAP")
	flagVersion := fs.Bool("version", false, "Print version and exit")

	// ── 3. Parse ───────────────────────────────────────────────────────
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	// --version
	if *flagVersion {
		fmt.Fprintf(stdout, "dibs %s\n", version)
		return 0
	}

	// ── Load & merge config ────────────────────────────────────────────
	cfg := config.Default()

	fileCfg, err := config.ParseFile(configFilePath())
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	cfg = config.Merge(cfg, fileCfg)

	// Apply only explicitly-set CLI flags on top of merged config.
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "all":
			cfg.All = *flagAll
		case "limit":
			cfg.Limit = *flagLimit
		case "tlds":
			cfg.TLDs = *flagTLDs
		case "json":
			cfg.JSON = *flagJSON
		case "csv":
			cfg.CSV = *flagCSV
		case "quiet":
			cfg.Quiet = *flagQuiet
		case "provider":
			cfg.Provider = *flagProvider
		case "rotate":
			cfg.Rotate = *flagRotate
		case "no-doh":
			cfg.NoDOH = *flagNoDOH
		case "no-color":
			cfg.NoColor = *flagNoColor
		case "parallel":
			cfg.Parallel = *flagParallel
		case "timeout":
			cfg.Timeout = *flagTimeout
		case "file":
			cfg.File = *flagFile
		case "min-length":
			cfg.MinLength = *flagMinLength
		case "max-length":
			cfg.MaxLength = *flagMaxLength
		case "sort":
			cfg.Sort = *flagSort
		case "retries":
			cfg.Retries = *flagRetries
		case "refresh":
			cfg.Refresh = *flagRefresh
		case "doh-url":
			cfg.DohURL = *flagDohURL
		case "verify":
			cfg.Verify = *flagVerify
		}
	})
	if os.Getenv("NO_COLOR") != "" {
		cfg.NoColor = true
	}

	// Validate mutual exclusivity.
	if cfg.Rotate && *flagProvider != "" {
		fmt.Fprintf(stderr, "Error: --rotate and --provider are mutually exclusive\n")
		return 1
	}

	if cfg.JSON && cfg.CSV {
		fmt.Fprintf(stderr, "Error: --json and --csv are mutually exclusive\n")
		return 1
	}

	validProviders := make(map[string]bool)
	for _, name := range dns.ProviderNames() {
		validProviders[name] = true
	}
	if err := config.Validate(cfg, validProviders); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	// ── 4. Collect domain names ──────────────────────────────────────────
	var names []string
	if cfg.File != "" {
		var err error
		names, err = readDomainsFromFile(cfg.File)
		if err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
	} else {
		var name string
		if fs.NArg() > 0 {
			name = fs.Arg(0)
		} else {
			// Interactive mode.
			fmt.Fprint(stderr, "Enter domain name to check: ")
			scanner := bufio.NewScanner(stdin)
			if scanner.Scan() {
				name = strings.TrimSpace(scanner.Text())
			}
		}
		names = []string{name}
	}

	for _, name := range names {
		if err := validateDomain(name); err != nil {
			if cfg.File != "" {
				fmt.Fprintf(stderr, "Error: invalid domain %q: %v\n", name, err)
			} else {
				fmt.Fprintf(stderr, "Error: %v\n", err)
			}
			return 1
		}
	}

	// ── 5. Build domain list ────────────────────────────────────────────
	tldList := resolveTLDList(cfg, stderr)
	allDomains := make([]string, 0, len(names)*len(tldList))
	for _, name := range names {
		allDomains = append(allDomains, buildDomainList(name, tldList)...)
	}

	// ── 6. Set up resolver ─────────────────────────────────────────────
	timeout := time.Duration(cfg.Timeout) * time.Second
	var resolver dns.Resolver
	switch {
	case cfg.NoDOH:
		resolver = dns.NewSystemResolver(timeout)
	case cfg.DohURL != "":
		custom := dns.Provider{Name: "custom", URL: cfg.DohURL}
		resolver = dns.NewDoHResolver([]dns.Provider{custom}, false, timeout)
	default:
		providers := resolveProviders(cfg)
		resolver = dns.NewDoHResolver(providers, cfg.Rotate, timeout)
	}

	// ── 7. Set up renderer ─────────────────────────────────────────────
	var renderer output.Renderer
	switch {
	case cfg.JSON:
		renderer = output.NewJSONRenderer(stdout, stderr)
	case cfg.CSV:
		renderer = output.NewCSVRenderer(stdout, stderr)
	default:
		noColor := cfg.NoColor || !output.IsWriterTTY(stdout)
		renderer = output.NewTerminalRenderer(stdout, cfg.Quiet, noColor)
	}

	// ── 8. Signal handling ─────────────────────────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── 9. Run worker pool ─────────────────────────────────────────────
	query := strings.Join(names, ", ")
	renderer.Start(query, len(allDomains))

	results, partial := runWorkerPool(ctx, resolver, allDomains, cfg.Parallel, cfg.Retries, renderer)

	// ── 9.5. RDAP verification ──────────────────────────────────────
	if cfg.Verify && !partial {
		var available []dns.Result
		for _, r := range results {
			if r.Status == dns.StatusAvailable {
				available = append(available, r)
			}
		}

		if len(available) > 0 {
			corrections, stats := runRDAPVerify(ctx, available, cfg, stderr)
			renderer.ApplyVerification(corrections, stats)
		}
	}

	// ── 10. Finish ─────────────────────────────────────────────────────
	renderer.Finish(partial)
	if partial {
		return 130 // 128 + SIGINT
	}
	return 0
}

// runWorkerPool fans out DNS lookups across a pool of goroutines.
func runWorkerPool(ctx context.Context, resolver dns.Resolver, domains []string, parallel, retries int, renderer output.Renderer) ([]dns.Result, bool) {
	results, cancelled := fanOut(ctx, domains, parallel, func(ctx context.Context, domain string) dns.Result {
		var result dns.Result
		for attempt := 0; attempt <= retries; attempt++ {
			result = resolver.Lookup(ctx, domain)
			if result.Status != dns.StatusError {
				break
			}
			if attempt < retries {
				time.Sleep(retryDelay)
			}
		}
		renderer.Render(result)
		return result
	})
	return results, cancelled
}

// ── Helper functions ───────────────────────────────────────────────────

// validateDomain checks that name is a valid domain label.
func validateDomain(name string) error {
	if name == "" {
		return fmt.Errorf("domain name cannot be empty")
	}
	if len(name) > 63 {
		return fmt.Errorf("domain name cannot exceed 63 characters")
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("domain name cannot start with a hyphen")
	}
	if strings.HasSuffix(name, "-") {
		return fmt.Errorf("domain name cannot end with a hyphen")
	}
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' {
			return fmt.Errorf("domain name contains invalid character: %q", r)
		}
	}
	return nil
}

// buildDomainList creates fully-qualified domain names from a label and TLD list.
func buildDomainList(name string, tldList []string) []string {
	domains := make([]string, len(tldList))
	for i, t := range tldList {
		domains[i] = name + "." + t
	}
	return domains
}

// readDomainsFromFile reads domain names from a file, one per line.
// Empty lines and lines starting with '#' are skipped.
func readDomainsFromFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open file %q: %w", path, err)
	}
	defer f.Close()

	var names []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		names = append(names, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading file %q: %w", path, err)
	}
	return names, nil
}

// resolveTLDList determines the TLD list to use based on config flags.
func resolveTLDList(cfg config.Config, stderr io.Writer) []string {
	var list []string

	switch {
	case cfg.TLDs != "":
		list = tlds.ParseCustom(cfg.TLDs)
	case cfg.All:
		list = fetchAllTLDs(cfg.Refresh, stderr)
	default:
		list = tlds.Top25()
	}

	if cfg.MinLength > 0 || cfg.MaxLength > 0 {
		list = tlds.FilterByLength(list, cfg.MinLength, cfg.MaxLength)
	}
	if cfg.Sort != "" {
		list = tlds.Sort(list, cfg.Sort)
	}
	if cfg.Limit > 0 {
		list = tlds.Limit(list, cfg.Limit)
	}

	return list
}

// resolveProviders returns the DNS provider list based on config.
func resolveProviders(cfg config.Config) []dns.Provider {
	if cfg.Rotate {
		return dns.AllProviders()
	}
	p, ok := dns.GetProvider(cfg.Provider)
	if !ok {
		return dns.AllProviders()[:1]
	}
	return []dns.Provider{p}
}

// xdgPath returns a path under an XDG base directory, falling back to
// ~/.fallback/appName when the environment variable is unset.
func xdgPath(envVar, fallback string, parts ...string) string {
	base := os.Getenv(envVar)
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, fallback)
	}
	return filepath.Join(append([]string{base, appName}, parts...)...)
}

// configFilePath returns the path to the configuration file.
func configFilePath() string {
	return xdgPath("XDG_CONFIG_HOME", ".config", "config")
}

// cacheFilePath returns the path to the TLD cache file.
func cacheFilePath() string {
	return xdgPath("XDG_CACHE_HOME", ".cache", "tlds.txt")
}

// runRDAPVerify checks available domains via RDAP and returns corrections.
func runRDAPVerify(ctx context.Context, available []dns.Result, cfg config.Config, stderr io.Writer) ([]dns.Result, output.VerifyStats) {
	fmt.Fprintf(stderr, "Verifying %d available domains via RDAP...\n", len(available))

	bootstrap := fetchRDAPBootstrap(cfg.Refresh, stderr)
	if bootstrap == nil {
		fmt.Fprintf(stderr, "Warning: could not load RDAP bootstrap, skipping verification\n")
		return nil, output.VerifyStats{}
	}

	client := rdap.NewClient(bootstrap, time.Duration(cfg.Timeout)*time.Second)

	rdapResults, _ := fanOut(ctx, available, cfg.Parallel, func(ctx context.Context, r dns.Result) rdap.Result {
		return client.Lookup(ctx, r.Domain, r.TLD)
	})

	var corrections []dns.Result
	var stats output.VerifyStats
	for _, r := range rdapResults {
		switch r.Status {
		case rdap.Registered:
			stats.Verified++
			corrections = append(corrections, dns.Result{
				Domain: r.Domain,
				TLD:    r.TLD,
				Status: dns.StatusTaken,
			})
		case rdap.NotFound:
			stats.Verified++
		case rdap.Unavailable:
			stats.Unverified++
		default:
			stats.Unverified++
		}
	}
	stats.Checked = len(available)

	return corrections, stats
}

