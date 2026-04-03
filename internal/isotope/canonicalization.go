package isotope

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
)

// GherkinKeyword normalizes Gherkin keywords across languages to canonical form.
type GherkinKeyword string

const (
	KeywordGiven GherkinKeyword = "GIVEN"
	KeywordWhen  GherkinKeyword = "WHEN"
	KeywordThen  GherkinKeyword = "THEN"
	KeywordAnd   GherkinKeyword = "AND"
	KeywordBut   GherkinKeyword = "BUT"
)

// keywordMap converts localized keywords to canonical English.
var keywordMap = map[string]GherkinKeyword{
	// English
	"Given": KeywordGiven,
	"When":  KeywordWhen,
	"Then":  KeywordThen,
	"And":   KeywordAnd,
	"But":   KeywordBut,
	// Spanish
	"Dado":     KeywordGiven,
	"Cuando":   KeywordWhen,
	"Entonces": KeywordThen,
	"Y":        KeywordAnd,
	"Pero":     KeywordBut,
	// French
	"Étant donné":  KeywordGiven,
	"Étant donnée": KeywordGiven,
	"Quand":        KeywordWhen,
	"Alors":        KeywordThen,
	"Et":           KeywordAnd,
	"Mais":         KeywordBut,
	// German
	"Gegeben sei": KeywordGiven,
	"Wenn":        KeywordWhen,
	"Dann":        KeywordThen,
	// Japanese
	"与えられた": KeywordGiven,
	"もし":    KeywordWhen,
	"ならば":   KeywordThen,
	"そして":   KeywordAnd,
	"しかし":   KeywordBut,
}

// Step represents a single Gherkin step.
type Step struct {
	Keyword GherkinKeyword
	Text    string
}

// Scenario represents a Gherkin scenario.
type Scenario struct {
	Name  string
	Steps []Step
}

// Feature represents a Gherkin feature.
type Feature struct {
	Name      string
	Scenarios []Scenario
}

// GherkinAST represents parsed Gherkin abstract syntax tree.
type GherkinAST struct {
	Features []Feature
}

// ParseGherkin parses Gherkin text (language-agnostic via keyword normalization).
func ParseGherkin(text string) (*GherkinAST, error) {
	lines := strings.Split(text, "\n")
	ast := &GherkinAST{}
	var currentFeature *Feature
	var currentScenario *Scenario

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Feature line
		if strings.HasPrefix(line, "Feature:") || strings.HasPrefix(line, "Característica:") ||
			strings.HasPrefix(line, "Fonctionnalité:") || strings.HasPrefix(line, "フィーチャー:") {
			if currentFeature != nil {
				ast.Features = append(ast.Features, *currentFeature)
			}
			name := extractName(line)
			currentFeature = &Feature{Name: name}
			currentScenario = nil
			continue
		}

		// Scenario line
		if strings.HasPrefix(line, "Scenario:") || strings.HasPrefix(line, "Escenario:") ||
			strings.HasPrefix(line, "Scénario:") || strings.HasPrefix(line, "シナリオ:") {
			if currentScenario != nil && currentFeature != nil {
				currentFeature.Scenarios = append(currentFeature.Scenarios, *currentScenario)
			}
			name := extractName(line)
			currentScenario = &Scenario{Name: name}
			continue
		}

		// Step lines (Given/When/Then/And/But)
		if currentScenario != nil {
			keyword, text := extractStep(line)
			if keyword != "" {
				step := Step{
					Keyword: normalizeKeyword(keyword),
					Text:    text,
				}
				currentScenario.Steps = append(currentScenario.Steps, step)
			}
		}
	}

	// Don't forget last feature and scenario
	if currentScenario != nil && currentFeature != nil {
		currentFeature.Scenarios = append(currentFeature.Scenarios, *currentScenario)
	}
	if currentFeature != nil {
		ast.Features = append(ast.Features, *currentFeature)
	}

	return ast, nil
}

// extractName extracts the name part from a feature/scenario line.
func extractName(line string) string {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// extractStep extracts keyword and text from a step line.
func extractStep(line string) (keyword, text string) {
	for kw := range keywordMap {
		if strings.HasPrefix(line, kw) {
			remaining := strings.TrimPrefix(line, kw)
			remaining = strings.TrimSpace(remaining)
			return kw, remaining
		}
	}
	return "", ""
}

// normalizeKeyword converts a keyword to canonical form.
func normalizeKeyword(kw string) GherkinKeyword {
	if canonical, ok := keywordMap[kw]; ok {
		return canonical
	}
	return ""
}

// normalizeStepText normalizes step text for hashing.
// Removes articles, punctuation, lowercases, and tokenizes with hyphens.
func normalizeStepText(text string) string {
	// List of articles to remove across languages
	articles := []string{
		"the", "a", "an", // English
		"un", "une", "des", // French
		"el", "la", "los", "las", "un", "unos", "unas", // Spanish
		"le", "la", "l'", "les", // French variants
		"ein", "eine", "einen", // German
		"der", "die", "das", // German
		"の", "を", "に", "は", // Japanese particles (simplified)
	}

	text = strings.ToLower(text)

	// Remove articles
	for _, article := range articles {
		// Word boundary removal: \bword\b pattern
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(article) + `\b`)
		text = re.ReplaceAllString(text, "")
	}

	// Remove punctuation
	re := regexp.MustCompile(`[^\w\s-]`)
	text = re.ReplaceAllString(text, "")

	// Collapse whitespace
	text = strings.Join(strings.Fields(text), "-")

	return text
}

// CanonicalToken represents a single token in canonical form.
type CanonicalToken struct {
	Type  string // "FEATURE", "SCENARIO", "GIVEN", "WHEN", "THEN", "AND", "BUT"
	Key   string // e.g., "name", "hash"
	Value string // the canonical value
}

// Canonicalize converts a GherkinAST to canonical token sequence.
func (ast *GherkinAST) Canonicalize() []CanonicalToken {
	var tokens []CanonicalToken

	for _, feature := range ast.Features {
		tokens = append(tokens, CanonicalToken{
			Type:  "FEATURE",
			Key:   "name",
			Value: normalizeStepText(feature.Name),
		})

		for _, scenario := range feature.Scenarios {
			tokens = append(tokens, CanonicalToken{
				Type:  "SCENARIO",
				Key:   "name",
				Value: normalizeStepText(scenario.Name),
			})

			for _, step := range scenario.Steps {
				tokens = append(tokens, CanonicalToken{
					Type:  string(step.Keyword),
					Key:   "hash",
					Value: hashText(normalizeStepText(step.Text)),
				})
			}
		}
	}

	return tokens
}

// hashText produces a short hash of text for inclusion in canonical form.
// Uses SHA256 truncated to 16 hex chars.
func hashText(text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", h[:8]) // First 64 bits as hex
}

// TokensToString serializes canonical tokens to newline-delimited string.
func TokensToString(tokens []CanonicalToken) string {
	var lines []string
	for _, t := range tokens {
		line := fmt.Sprintf("%s:%s:%s", t.Type, t.Key, t.Value)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// Isotope represents a cryptographically signed test identifier.
type Isotope struct {
	Family    string // e.g., "iso-login"
	Version   int    // v1, v2, v3 for semantic changes
	Signature string // HMAC-SHA256 signature
	Raw       string // The canonical token string that was signed
}

// String returns the isotope in string form.
func (i Isotope) String() string {
	return fmt.Sprintf("%s-v%d:%s", i.Family, i.Version, i.Signature[:16])
}

// GenerateIsotope computes an isotope from canonical tokens using a signing key.
func GenerateIsotope(tokens []CanonicalToken, signingKey []byte, family string, version int) Isotope {
	canonical := TokensToString(tokens)

	h := hmac.New(sha256.New, signingKey)
	h.Write([]byte(canonical))
	signature := fmt.Sprintf("%x", h.Sum(nil))

	return Isotope{
		Family:    family,
		Version:   version,
		Signature: signature,
		Raw:       canonical,
	}
}

// CanonicalizeGherkinToIsotope is a convenience function that:
// 1. Parses Gherkin
// 2. Canonicalizes to tokens
// 3. Generates isotope
func CanonicalizeGherkinToIsotope(gherkinText string, signingKey []byte, family string, version int) (Isotope, error) {
	ast, err := ParseGherkin(gherkinText)
	if err != nil {
		return Isotope{}, err
	}

	tokens := ast.Canonicalize()
	isotope := GenerateIsotope(tokens, signingKey, family, version)

	return isotope, nil
}
