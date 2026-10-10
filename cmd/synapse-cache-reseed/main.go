// Command synapse-cache-reseed rebuilds Synapse MCP's knowledge cache for the
// smoke-alarm feature knowledge base.
//
// The derived docs under docs/features are the source of truth for summaries:
// each catalog doc describes one Gherkin .feature file, so its title and
// description are written onto that feature's header chunk. Subsystem rollup
// docs have no .feature source, so their own heading chunk is used instead.
// Chunk ids are re-resolved from the live index on every run; nothing is
// persisted between runs.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	appName = "synapse-cache-reseed"
	version = "dev"

	// catalogSubdir and subsystemSubdir are relative to --docs-root.
	catalogSubdir   = "catalog"
	subsystemSubdir = "subsystems"

	// featureSymbolPrefix identifies the header chunk of a .feature file.
	featureSymbolPrefix = "feature_"
)

type options struct {
	synapseURL string
	repoID     string
	repoRoot   string
	docsRoot   string
	timeout    time.Duration
	weight     float64
	apply      bool
}

func main() {
	opts := parseFlags()
	if err := run(context.Background(), opts); err != nil {
		fmt.Fprintln(os.Stderr, appName+":", err)
		os.Exit(1)
	}
}

func parseFlags() options {
	var opts options
	flag.StringVar(&opts.synapseURL, "synapse-url", "http://127.0.0.1:8585/mcp", "Synapse MCP Streamable HTTP endpoint")
	flag.StringVar(&opts.repoID, "repo", "smoke-alarm", "Synapse repository id")
	flag.StringVar(&opts.repoRoot, "repo-root", ".", "path to the repository root")
	flag.StringVar(&opts.docsRoot, "docs-root", "docs/features", "feature docs directory relative to --repo-root")
	flag.DurationVar(&opts.timeout, "timeout", 30*time.Second, "per-request HTTP timeout")
	flag.Float64Var(&opts.weight, "weight", 1, "learning signal weight for learned queries")
	flag.BoolVar(&opts.apply, "apply", false, "write summaries and learning signals (default is a dry run)")
	flag.Parse()
	return opts
}

func run(ctx context.Context, opts options) error {
	set, err := loadDocs(opts.repoRoot, opts.docsRoot)
	if err != nil {
		return err
	}
	if len(set.catalogs) == 0 && len(set.subsystems) == 0 {
		return fmt.Errorf("no markdown docs under %s", opts.docsRoot)
	}

	client, err := dialMCP(ctx, opts.synapseURL, opts.timeout)
	if err != nil {
		return fmt.Errorf("connect to Synapse at %s: %w", opts.synapseURL, err)
	}

	plan := buildPlan(ctx, client, opts, set)
	if err := applyPlan(ctx, client, opts, plan); err != nil {
		return err
	}
	report(opts, set, plan)

	if len(plan.failures) > 0 {
		return fmt.Errorf("%d doc(s) could not be resolved", len(plan.failures))
	}
	return nil
}

// --- documents ---------------------------------------------------------------

type frontmatter struct {
	Title         string   `yaml:"title"`
	Description   string   `yaml:"description"`
	Source        string   `yaml:"source"`
	Subsystem     string   `yaml:"subsystem"`
	GherkinTags   []string `yaml:"gherkin_tags"`
	ScenarioCount int      `yaml:"scenario_count"`
	FeatureCount  int      `yaml:"feature_count"`
}

type doc struct {
	path string // repository-relative, slash-separated
	fm   frontmatter
}

type docSet struct {
	catalogs   []doc
	subsystems []doc
}

// summary renders the doc's frontmatter as the cache summary line.
func (d doc) summary() string {
	title := strings.TrimSpace(d.fm.Title)
	description := strings.TrimSpace(d.fm.Description)
	switch {
	case title == "":
		return description
	case description == "":
		return title
	default:
		return title + " — " + description
	}
}

func parseFrontmatter(data []byte) (*frontmatter, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	const delimiter = "---\n"
	if !strings.HasPrefix(text, delimiter) {
		return nil, errors.New("missing YAML frontmatter")
	}
	rest := text[len(delimiter):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, errors.New("unterminated YAML frontmatter")
	}
	var fm frontmatter
	if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err != nil {
		return nil, fmt.Errorf("parse YAML frontmatter: %w", err)
	}
	return &fm, nil
}

func loadDocs(repoRoot, docsRoot string) (docSet, error) {
	catalogs, err := readDocDir(repoRoot, filepath.Join(docsRoot, catalogSubdir))
	if err != nil {
		return docSet{}, err
	}
	subsystems, err := readDocDir(repoRoot, filepath.Join(docsRoot, subsystemSubdir))
	if err != nil {
		return docSet{}, err
	}
	return docSet{catalogs: catalogs, subsystems: subsystems}, nil
}

func readDocDir(repoRoot, dir string) ([]doc, error) {
	entries, err := os.ReadDir(filepath.Join(repoRoot, dir))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	docs := make([]doc, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(dir, entry.Name()))
		data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
		fm, err := parseFrontmatter(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		docs = append(docs, doc{path: rel, fm: *fm})
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].path < docs[j].path })
	return docs, nil
}

// featurePathFor turns a frontmatter source such as
// "smoke-alarm/features/alerts.feature" into the repository-relative path
// "features/alerts.feature".
func featurePathFor(repoID, source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", errors.New("missing source in frontmatter")
	}
	path := strings.TrimPrefix(strings.TrimPrefix(source, repoID+"/"), "./")
	if !strings.HasSuffix(path, ".feature") {
		return "", fmt.Errorf("source %q is not a .feature file", source)
	}
	return path, nil
}

// featureHeading reads the human-readable name from the Feature: line.
func featureHeading(repoRoot, featurePath string) (string, error) {
	file, err := os.Open(filepath.Join(repoRoot, filepath.FromSlash(featurePath)))
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if heading, ok := strings.CutPrefix(strings.TrimSpace(scanner.Text()), "Feature:"); ok {
			return strings.TrimSpace(heading), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no Feature: line in %s", featurePath)
}

// aliasFor derives a query alias from a file name, e.g. "chaos-window" ->
// "chaos window".
func aliasFor(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return strings.ReplaceAll(strings.ReplaceAll(base, "-", " "), "_", " ")
}

// gherkinTagFrequency counts how many catalog docs carry each Gherkin tag.
// Tags shared across docs are ambiguous as learned queries and are skipped.
func gherkinTagFrequency(catalogs []doc) map[string]int {
	frequency := make(map[string]int)
	for _, d := range catalogs {
		seen := make(map[string]bool)
		for _, tag := range d.fm.GherkinTags {
			name := strings.TrimPrefix(strings.TrimSpace(tag), "@")
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			frequency[name]++
		}
	}
	return frequency
}

func catalogQueries(d doc, heading string, tagFrequency map[string]int) []string {
	queries := []string{d.fm.Title, heading, aliasFor(d.path)}
	for _, tag := range d.fm.GherkinTags {
		name := strings.TrimPrefix(strings.TrimSpace(tag), "@")
		if name == "" || tagFrequency[name] != 1 {
			continue
		}
		queries = append(queries, name)
	}
	return queries
}

func subsystemQueries(d doc) []string {
	return []string{d.fm.Title, aliasFor(d.path)}
}

// --- chunk resolution --------------------------------------------------------

type chunkRef struct {
	ChunkID   string `json:"chunk_id"`
	FilePath  string `json:"file_path"`
	Symbol    string `json:"symbol"`
	StartLine int    `json:"start_line"`
}

type searchResults struct {
	Results []chunkRef `json:"results"`
}

func (c *mcpClient) searchChunks(ctx context.Context, repoID, path, query string) ([]chunkRef, error) {
	var results searchResults
	args := map[string]any{
		"action":           "regex",
		"query":            query,
		"path":             path,
		"repo_id":          repoID,
		"limit":            50,
		"compress_payload": false,
	}
	if err := c.callToolInto(ctx, "synapse_search_codebase", args, &results); err != nil {
		return nil, err
	}
	return results.Results, nil
}

// resolveFeatureChunk finds the header chunk of a .feature file.
func resolveFeatureChunk(ctx context.Context, client *mcpClient, repoID, featurePath string) (chunkRef, error) {
	results, err := client.searchChunks(ctx, repoID, featurePath, "^Feature:")
	if err != nil {
		return chunkRef{}, err
	}
	for _, ref := range results {
		if strings.HasPrefix(ref.Symbol, featureSymbolPrefix) {
			return ref, nil
		}
	}
	return chunkRef{}, fmt.Errorf("no %s* header chunk for %s", featureSymbolPrefix, featurePath)
}

// resolveDocChunk finds the level-1 heading chunk of a subsystem doc.
func resolveDocChunk(ctx context.Context, client *mcpClient, repoID, docPath string) (chunkRef, error) {
	results, err := client.searchChunks(ctx, repoID, docPath, "^# ")
	if err != nil {
		return chunkRef{}, err
	}
	for _, ref := range results {
		if ref.FilePath == docPath && ref.Symbol != "" {
			return ref, nil
		}
	}
	return chunkRef{}, fmt.Errorf("no heading chunk for %s", docPath)
}

// --- cache plan --------------------------------------------------------------

type summaryItem struct {
	ChunkID     string `json:"chunk_id"`
	Summary     string `json:"summary"`
	SummaryType string `json:"summary_type"`
}

type learnItem struct {
	query   string
	chunkID string
}

type plan struct {
	summaries  []summaryItem
	learns     []learnItem
	failures   []string
	warnings   []string
	learnIndex map[string]string
}

func (p *plan) addFailure(path string, err error) {
	p.failures = append(p.failures, fmt.Sprintf("%s: %v", path, err))
}

func (p *plan) addLearn(query, chunkID string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}
	if existing, ok := p.learnIndex[query]; ok {
		if existing != chunkID {
			p.warnings = append(p.warnings, fmt.Sprintf("query %q maps to multiple chunks; kept the first", query))
		}
		return
	}
	p.learnIndex[query] = chunkID
	p.learns = append(p.learns, learnItem{query: query, chunkID: chunkID})
}

func buildPlan(ctx context.Context, client *mcpClient, opts options, set docSet) *plan {
	p := &plan{learnIndex: make(map[string]string)}
	tagFrequency := gherkinTagFrequency(set.catalogs)

	for _, d := range set.catalogs {
		featurePath, err := featurePathFor(opts.repoID, d.fm.Source)
		if err != nil {
			p.addFailure(d.path, err)
			continue
		}
		ref, err := resolveFeatureChunk(ctx, client, opts.repoID, featurePath)
		if err != nil {
			p.addFailure(d.path, err)
			continue
		}
		p.summaries = append(p.summaries, summaryItem{ChunkID: ref.ChunkID, Summary: d.summary(), SummaryType: "user"})
		heading, err := featureHeading(opts.repoRoot, featurePath)
		if err != nil {
			p.warnings = append(p.warnings, fmt.Sprintf("%s: %v", d.path, err))
			heading = ""
		}
		for _, query := range catalogQueries(d, heading, tagFrequency) {
			p.addLearn(query, ref.ChunkID)
		}
	}

	for _, d := range set.subsystems {
		ref, err := resolveDocChunk(ctx, client, opts.repoID, d.path)
		if err != nil {
			p.addFailure(d.path, err)
			continue
		}
		p.summaries = append(p.summaries, summaryItem{ChunkID: ref.ChunkID, Summary: d.summary(), SummaryType: "user"})
		for _, query := range subsystemQueries(d) {
			p.addLearn(query, ref.ChunkID)
		}
	}
	return p
}

func applyPlan(ctx context.Context, client *mcpClient, opts options, p *plan) error {
	if !opts.apply {
		return nil
	}
	if err := saveSummaries(ctx, client, p.summaries); err != nil {
		return err
	}
	for _, item := range p.learns {
		args := map[string]any{
			"action":           "learn",
			"query":            item.query,
			"chunk_id":         item.chunkID,
			"weight":           opts.weight,
			"compress_payload": false,
		}
		if err := client.callToolNoResult(ctx, "synapse_knowledge_cache", args); err != nil {
			return fmt.Errorf("learn %q: %w", item.query, err)
		}
	}
	return nil
}

func saveSummaries(ctx context.Context, client *mcpClient, items []summaryItem) error {
	if len(items) == 0 {
		return nil
	}
	var result struct {
		Results []struct {
			ChunkID string `json:"chunk_id"`
			Result  string `json:"result"`
			Error   string `json:"error"`
		} `json:"results"`
	}
	args := map[string]any{
		"action":           "save_summary",
		"summaries":        items,
		"compress_payload": false,
	}
	if err := client.callToolInto(ctx, "synapse_knowledge_cache", args, &result); err != nil {
		return err
	}
	for _, item := range result.Results {
		switch {
		case item.Error != "":
			return fmt.Errorf("save_summary %s: %s", item.ChunkID, item.Error)
		case item.Result != "" && item.Result != "ok":
			return fmt.Errorf("save_summary %s: %s", item.ChunkID, item.Result)
		}
	}
	return nil
}

// --- reporting ---------------------------------------------------------------

func report(opts options, set docSet, p *plan) {
	mode := "dry run"
	if opts.apply {
		mode = "applied"
	}
	fmt.Printf("%s (%s)\n", appName, mode)
	fmt.Printf("  synapse:  %s\n", opts.synapseURL)
	fmt.Printf("  repo:     %s\n", opts.repoID)
	fmt.Printf("  docs:     %s\n\n", opts.docsRoot)
	fmt.Printf("  catalog docs:   %d\n", len(set.catalogs))
	fmt.Printf("  subsystem docs: %d\n", len(set.subsystems))
	fmt.Printf("  summaries:      %d\n", len(p.summaries))
	fmt.Printf("  learn queries:  %d\n", len(p.learns))
	if len(p.warnings) > 0 {
		fmt.Printf("\n  warnings (%d):\n", len(p.warnings))
		for _, warning := range p.warnings {
			fmt.Printf("    - %s\n", warning)
		}
	}
	if len(p.failures) > 0 {
		fmt.Printf("\n  unresolved (%d):\n", len(p.failures))
		for _, failure := range p.failures {
			fmt.Printf("    - %s\n", failure)
		}
	}
	if !opts.apply {
		fmt.Println("\n  dry run: pass --apply to write summaries and learning signals")
	}
}
