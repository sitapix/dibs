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
			// If this flag takes a value (e.g. -provider nextdns), grab the next arg too.
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
		fmt.Fprintf(stderr, "Usage:\n")
		fmt.Fprintf(stderr, "  dibs [options] <name>          Sweep ICANN TLDs for <name>\n")
		fmt.Fprintf(stderr, "  dibs [options] <name>.<tld>    Check a single full domain\n")
		fmt.Fprintf(stderr, "  dibs [options] --file <path>   Check domains listed in a file\n\n")
		fmt.Fprintf(stderr, "Examples:\n")
		fmt.Fprintf(stderr, "  dibs mybrand              # Top 25 TLDs\n")
		fmt.Fprintf(stderr, "  dibs mybrand --all        # every ICANN TLD (~1400)\n")
		fmt.Fprintf(stderr, "  dibs vi.be                # one specific domain\n")
		fmt.Fprintf(stderr, "  dibs foo.co.uk --verify   # confirm via RDAP\n\n")
		fmt.Fprintf(stderr, "Options:\n")
		fs.PrintDefaults()
	}

	// ── 2. Define flags ────────────────────────────────────────────────
	flags := defineFlags(fs)

	// ── 3. Parse ───────────────────────────────────────────────────────
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	// --version
	if *flags.version {
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

	applyCLIFlags(fs, flags, &cfg)
	if os.Getenv("NO_COLOR") != "" {
		cfg.NoColor = true
	}

	// Validate mutual exclusivity.
	if cfg.Rotate && *flags.provider != "" {
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
	names, allDomains, err := collectDomains(cfg, fs, stdin, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	// ── 6. Set up resolver and renderer ────────────────────────────────
	resolver := buildResolver(cfg)
	renderer := buildRenderer(cfg, stdout, stderr)

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
	retryLoop:
		for attempt := 0; attempt <= retries; attempt++ {
			result = resolver.Lookup(ctx, domain)
			if result.Status != dns.StatusError {
				break
			}
			if attempt < retries {
				// Interruptible sleep: without the ctx.Done() arm, Ctrl+C
				// would block up to retryDelay × remaining attempts before
				// the next Lookup sees the cancelled context.
				select {
				case <-time.After(retryDelay):
				case <-ctx.Done():
					break retryLoop
				}
			}
		}
		renderer.Render(result)
		return result
	})
	return results, cancelled
}

// ── Helper functions ───────────────────────────────────────────────────

// flagPointers holds the pointers returned by fs.X for every defined flag,
// so defineFlags and applyCLIFlags can be split without spilling 22 named
// arguments through the call stack.
type flagPointers struct {
	all       *bool
	limit     *int
	tlds      *string
	json      *bool
	csv       *bool
	quiet     *bool
	provider  *string
	rotate    *bool
	noDOH     *bool
	noColor   *bool
	parallel  *int
	timeout   *int
	file      *string
	minLength *int
	maxLength *int
	sort      *string
	retries   *int
	refresh   *bool
	dohURL    *string
	verify    *bool
	version   *bool
}

// defineFlags registers every dibs flag against fs and returns the pointer
// bundle for later inspection.
func defineFlags(fs *flag.FlagSet) *flagPointers {
	return &flagPointers{
		all:       fs.Bool("all", false, "Check all IANA TLDs (not just Top 25)"),
		limit:     fs.Int("limit", 0, "Limit number of TLDs checked"),
		tlds:      fs.String("tlds", "", "Comma-separated list of TLDs to check"),
		json:      fs.Bool("json", false, "Output results as JSON"),
		csv:       fs.Bool("csv", false, "Output results as CSV"),
		quiet:     fs.Bool("quiet", false, "Only show available domains"),
		provider:  fs.String("provider", "", "DoH provider: quad9, mullvad, nextdns, adguard"),
		rotate:    fs.Bool("rotate", false, "Rotate between DoH providers"),
		noDOH:     fs.Bool("no-doh", false, "Use system DNS resolver instead of DoH"),
		noColor:   fs.Bool("no-color", false, "Disable ANSI color output"),
		parallel:  fs.Int("parallel", 0, "Number of parallel DNS queries"),
		timeout:   fs.Int("timeout", 0, "DNS query timeout in seconds"),
		file:      fs.String("file", "", "Read domain names from file (one per line)"),
		minLength: fs.Int("min-length", 0, "Minimum TLD length to check"),
		maxLength: fs.Int("max-length", 0, "Maximum TLD length to check"),
		sort:      fs.String("sort", "", "Sort TLDs: alpha, length"),
		retries:   fs.Int("retries", 0, "Number of retries on DNS error"),
		refresh:   fs.Bool("refresh", false, "Force refresh of cached TLD list"),
		dohURL:    fs.String("doh-url", "", "Custom DoH resolver URL"),
		verify:    fs.Bool("verify", false, "Verify available domains via RDAP"),
		version:   fs.Bool("version", false, "Print version and exit"),
	}
}

// applyCLIFlags copies the explicitly-set flag values from p into cfg. Only
// CLI-explicit flags (via fs.Visit) overwrite cfg, so config-file defaults
// stay intact when the user doesn't pass a flag.
func applyCLIFlags(fs *flag.FlagSet, p *flagPointers, cfg *config.Config) {
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "all":
			cfg.All = *p.all
		case "limit":
			cfg.Limit = *p.limit
		case "tlds":
			cfg.TLDs = *p.tlds
		case "json":
			cfg.JSON = *p.json
		case "csv":
			cfg.CSV = *p.csv
		case "quiet":
			cfg.Quiet = *p.quiet
		case "provider":
			cfg.Provider = *p.provider
		case "rotate":
			cfg.Rotate = *p.rotate
		case "no-doh":
			cfg.NoDOH = *p.noDOH
		case "no-color":
			cfg.NoColor = *p.noColor
		case "parallel":
			cfg.Parallel = *p.parallel
		case "timeout":
			cfg.Timeout = *p.timeout
		case "file":
			cfg.File = *p.file
		case "min-length":
			cfg.MinLength = *p.minLength
		case "max-length":
			cfg.MaxLength = *p.maxLength
		case "sort":
			cfg.Sort = *p.sort
		case "retries":
			cfg.Retries = *p.retries
		case "refresh":
			cfg.Refresh = *p.refresh
		case "doh-url":
			cfg.DohURL = *p.dohURL
		case "verify":
			cfg.Verify = *p.verify
		}
	})
}

// collectDomains gathers the inputs to check from one of three sources (file,
// positional arg, interactive prompt) and decides whether single-domain mode
// applies. It returns the user-facing names slice (used for the renderer's
// query line) and the canonical allDomains slice (used for DNS lookup).
func collectDomains(cfg config.Config, fs *flag.FlagSet, stdin io.Reader, stderr io.Writer) (names, allDomains []string, err error) {
	if cfg.File != "" {
		names, err = readDomainsFromFile(cfg.File)
		if err != nil {
			return nil, nil, err
		}
	} else {
		var name string
		if fs.NArg() > 0 {
			name = strings.TrimSpace(fs.Arg(0))
		} else {
			fmt.Fprint(stderr, "Enter domain name to check: ")
			scanner := bufio.NewScanner(stdin)
			if scanner.Scan() {
				name = strings.TrimSpace(scanner.Text())
			}
		}
		names = []string{name}
	}

	// Single-domain mode: a single positional/interactive arg containing a
	// dot is treated as a full domain (e.g. "vi.be") and bypasses the sweep.
	singleDomain := cfg.File == "" && len(names) == 1 && strings.Contains(names[0], ".")
	if singleDomain {
		if err := rejectSingleDomainConflicts(fs); err != nil {
			return nil, nil, err
		}
		label, tld, err := parseFullDomain(names[0])
		if err != nil {
			return nil, nil, err
		}
		// `names` keeps the user's original input (for the renderer's query
		// line); `allDomains` carries the normalized lookup form.
		return names, []string{label + "." + tld}, nil
	}

	for _, name := range names {
		if err := validateDomain(name); err != nil {
			if cfg.File != "" {
				return nil, nil, fmt.Errorf("invalid domain %q: %w", name, err)
			}
			return nil, nil, err
		}
	}

	tldList := resolveTLDList(cfg, stderr)
	allDomains = make([]string, 0, len(names)*len(tldList))
	for _, name := range names {
		allDomains = append(allDomains, buildDomainList(name, tldList)...)
	}
	return names, allDomains, nil
}

// buildResolver constructs the DNS resolver from cfg.
func buildResolver(cfg config.Config) dns.Resolver {
	timeout := time.Duration(cfg.Timeout) * time.Second
	switch {
	case cfg.NoDOH:
		return dns.NewSystemResolver(timeout)
	case cfg.DohURL != "":
		custom := dns.Provider{Name: "custom", URL: cfg.DohURL}
		return dns.NewDoHResolver([]dns.Provider{custom}, false, timeout)
	default:
		return dns.NewDoHResolver(resolveProviders(cfg), cfg.Rotate, timeout)
	}
}

// buildRenderer constructs the output renderer for cfg.
func buildRenderer(cfg config.Config, stdout, stderr io.Writer) output.Renderer {
	switch {
	case cfg.JSON:
		return output.NewJSONRenderer(stdout, stderr)
	case cfg.CSV:
		return output.NewCSVRenderer(stdout, stderr)
	default:
		noColor := cfg.NoColor || !output.IsWriterTTY(stdout)
		return output.NewTerminalRenderer(stdout, cfg.Quiet, noColor)
	}
}

// rejectSingleDomainConflicts returns an error if any flag that controls TLD
// selection was explicitly set on the command line while single-domain mode is
// active. Only CLI-explicit flags are checked (via fs.Visit), so defaults from
// the config file never cause surprises. All conflicts are reported in a
// single error so the user can fix them in one pass.
//
// Note: --file is intentionally NOT in this set. The single-domain branch is
// only reachable when cfg.File == "", so --file can never reach this check.
func rejectSingleDomainConflicts(fs *flag.FlagSet) error {
	conflictingFlags := map[string]bool{
		"all":        true,
		"tlds":       true,
		"limit":      true,
		"min-length": true,
		"max-length": true,
		"sort":       true,
	}
	var conflicts []string
	fs.Visit(func(f *flag.Flag) {
		if conflictingFlags[f.Name] {
			conflicts = append(conflicts, "--"+f.Name)
		}
	})
	if len(conflicts) > 0 {
		return fmt.Errorf("%s cannot be combined with a full domain argument", strings.Join(conflicts, ", "))
	}
	return nil
}

// parseFullDomain splits a full domain like "vi.be" or "foo.co.uk" into its
// registrable label and TLD via the Public Suffix List. It normalizes input
// (trim whitespace, strip trailing FQDN dot, lowercase), rejects subdomain
// inputs ("mail.google.com"), non-registrable suffixes, and labels that
// fail validateDomain.
func parseFullDomain(input string) (label, tld string, err error) {
	input = strings.TrimSpace(input)
	input = strings.TrimSuffix(input, ".")
	input = strings.ToLower(input)
	if input == "" {
		return "", "", fmt.Errorf("domain name cannot be empty")
	}

	// tlds.Suffix uses PSL to handle multi-label TLDs like .co.uk; see its
	// docs for the icann semantics. Power users wanting non-ICANN suffixes
	// can fall back to the sweep form: `dibs <label> --tlds <tld>`.
	suffix, icann := tlds.Suffix(input)
	if !icann {
		return "", "", fmt.Errorf("%q is not a registrable ICANN TLD", suffix)
	}

	etld1, err := tlds.RegistrableDomain(input)
	if err != nil {
		return "", "", fmt.Errorf("invalid domain %q: %w", input, err)
	}
	if etld1 != input {
		return "", "", fmt.Errorf("%q has extra labels; did you mean %q?", input, etld1)
	}

	label = strings.TrimSuffix(input, "."+suffix)
	tld = suffix

	if err := validateDomain(label); err != nil {
		return "", "", fmt.Errorf("invalid domain label %q: %w", label, err)
	}
	return label, tld, nil
}

// validateDomain checks that name is a valid DNS label per RFC 1035:
// ASCII letters, digits, and hyphens only, no leading or trailing hyphen,
// length within dns.MaxLabelLength.
func validateDomain(name string) error {
	if name == "" {
		return fmt.Errorf("domain name cannot be empty")
	}
	if len(name) > dns.MaxLabelLength {
		return fmt.Errorf("domain name cannot exceed %d characters", dns.MaxLabelLength)
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("domain name cannot start with a hyphen")
	}
	if strings.HasSuffix(name, "-") {
		return fmt.Errorf("domain name cannot end with a hyphen")
	}
	for _, r := range name {
		if r > unicode.MaxASCII {
			return fmt.Errorf("non-ASCII character %q not supported in domain names (use punycode)", r)
		}
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
