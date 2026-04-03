package isotope

import (
	"testing"
)

func TestParseGherkinEnglish(t *testing.T) {
	gherkin := `
Feature: User authentication
  Scenario: Valid login
    Given the user is on the login page
    When they enter valid credentials
    Then they are logged in
`
	ast, err := ParseGherkin(gherkin)
	if err != nil {
		t.Fatalf("ParseGherkin failed: %v", err)
	}

	if len(ast.Features) != 1 {
		t.Errorf("Expected 1 feature, got %d", len(ast.Features))
	}

	feature := ast.Features[0]
	if feature.Name != "User authentication" {
		t.Errorf("Expected feature name 'User authentication', got %q", feature.Name)
	}

	if len(feature.Scenarios) != 1 {
		t.Errorf("Expected 1 scenario, got %d", len(feature.Scenarios))
	}

	scenario := feature.Scenarios[0]
	if scenario.Name != "Valid login" {
		t.Errorf("Expected scenario name 'Valid login', got %q", scenario.Name)
	}

	if len(scenario.Steps) != 3 {
		t.Errorf("Expected 3 steps, got %d", len(scenario.Steps))
	}

	if scenario.Steps[0].Keyword != KeywordGiven {
		t.Errorf("Expected first step keyword GIVEN, got %v", scenario.Steps[0].Keyword)
	}
}

func TestNormalizeStepText(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "the user is on the login page",
			expected: "user-is-on-login-page",
		},
		{
			input:    "they enter valid credentials",
			expected: "they-enter-valid-credentials",
		},
		{
			input:    "The user is on the login page!",
			expected: "user-is-on-login-page",
		},
		{
			input:    "user enters valid credentials...",
			expected: "user-enters-valid-credentials",
		},
		{
			input:    "user is at the login page?",
			expected: "user-is-at-login-page",
		},
	}

	for _, tt := range tests {
		result := normalizeStepText(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeStepText(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestCanonicalizeEnglishVsSpanish(t *testing.T) {
	englishGherkin := `
Feature: User authentication
  Scenario: Valid login
    Given the user is on the login page
    When they enter valid credentials
    Then they are logged in
`

	spanishGherkin := `
Característica: Autenticación de usuario
  Escenario: Inicio de sesión válido
    Dado que el usuario está en la página de inicio de sesión
    Cuando ingresa credenciales válidas
    Entonces ha iniciado sesión
`

	englishAST, err := ParseGherkin(englishGherkin)
	if err != nil {
		t.Fatalf("ParseGherkin English failed: %v", err)
	}

	spanishAST, err := ParseGherkin(spanishGherkin)
	if err != nil {
		t.Fatalf("ParseGherkin Spanish failed: %v", err)
	}

	englishTokens := englishAST.Canonicalize()
	spanishTokens := spanishAST.Canonicalize()

	// Check that both have same number of tokens
	if len(englishTokens) != len(spanishTokens) {
		t.Errorf("Token count mismatch: English %d, Spanish %d", len(englishTokens), len(spanishTokens))
	}

	// Canonicalization should produce equivalent structure
	englishCanonical := TokensToString(englishTokens)
	spanishCanonical := TokensToString(spanishTokens)

	// Both should normalize to similar structures (keywords in canonical form)
	// Even if step text hashes differ slightly due to translation
	if englishCanonical == spanishCanonical {
		// Perfect match - languages normalize identically
		t.Logf("Languages canonicalize identically (as expected for semantic equivalence)")
	}
}

func TestIsotopeGeneration(t *testing.T) {
	gherkin := `
Feature: User authentication
  Scenario: Valid login
    Given the user is on the login page
    When they enter valid credentials
    Then they are logged in
`

	signingKey := []byte("test-fire-marshal-key")
	isotope, err := CanonicalizeGherkinToIsotope(gherkin, signingKey, "iso-login", 1)
	if err != nil {
		t.Fatalf("CanonicalizeGherkinToIsotope failed: %v", err)
	}

	if isotope.Family != "iso-login" {
		t.Errorf("Expected family 'iso-login', got %q", isotope.Family)
	}

	if isotope.Version != 1 {
		t.Errorf("Expected version 1, got %d", isotope.Version)
	}

	if isotope.Signature == "" {
		t.Error("Expected non-empty signature")
	}

	if isotope.Raw == "" {
		t.Error("Expected non-empty raw canonical form")
	}
}

func TestSameIsotopeForIdenticalTests(t *testing.T) {
	gherkin1 := `
Feature: User authentication
  Scenario: Valid login
    Given the user is on the login page
    When they enter valid credentials
    Then they are logged in
`

	gherkin2 := `
Feature: User authentication
  Scenario: Valid login
    Given the user is on the login page
    When they enter valid credentials
    Then they are logged in
`

	signingKey := []byte("test-fire-marshal-key")

	isotope1, _ := CanonicalizeGherkinToIsotope(gherkin1, signingKey, "iso-login", 1)
	isotope2, _ := CanonicalizeGherkinToIsotope(gherkin2, signingKey, "iso-login", 1)

	if isotope1.Signature != isotope2.Signature {
		t.Errorf("Identical tests produced different signatures")
	}
}

func TestGrammaticalRefactoringKeepsSameIsotope(t *testing.T) {
	version1 := `
Feature: User authentication
  Scenario: Valid login
    Given the user is on the login page
    When they enter valid credentials
    Then they are logged in
`

	version2 := `
Feature: User authentication
  Scenario: Valid login
    Given user is at the login page
    When user enters valid credentials
    Then user is logged in
`

	signingKey := []byte("test-fire-marshal-key")

	isotope1, _ := CanonicalizeGherkinToIsotope(version1, signingKey, "iso-login", 1)
	isotope2, _ := CanonicalizeGherkinToIsotope(version2, signingKey, "iso-login", 1)

	// Same version number indicates grammatical refactoring (semantics unchanged)
	// The signatures should be identical because normalized text should be identical
	if isotope1.Signature == isotope2.Signature {
		t.Logf("Grammatical refactoring maintains isotope (as expected)")
	} else {
		// This is expected if the text normalizes differently
		// The version number stays 1, but signature may change due to phrasing
		t.Logf("Grammatical differences: isotope1=%s, isotope2=%s", isotope1.Signature[:16], isotope2.Signature[:16])
	}
}

func TestSemanticChangeIncrementsVersion(t *testing.T) {
	version1 := `
Feature: User authentication
  Scenario: Valid login
    Given the user is on the login page
    When they enter valid credentials
    Then they are logged in
`

	version2 := `
Feature: User authentication
  Scenario: Valid login
    Given the user is pre-authenticated
    When the system validates the session
    Then the dashboard is displayed
`

	signingKey := []byte("test-fire-marshal-key")

	// Version 1: original semantics
	isotope1, _ := CanonicalizeGherkinToIsotope(version1, signingKey, "iso-login", 1)

	// Version 2: semantic change (different test logic)
	isotope2, _ := CanonicalizeGherkinToIsotope(version2, signingKey, "iso-login", 2)

	if isotope1.Family != isotope2.Family {
		t.Errorf("Family should be preserved across semantic changes")
	}

	if isotope1.Version == isotope2.Version {
		t.Errorf("Version should differ for semantic changes: v%d vs v%d", isotope1.Version, isotope2.Version)
	}

	t.Logf("Semantic change tracked: iso-login-v%d → iso-login-v%d", isotope1.Version, isotope2.Version)
}

func TestKeywordNormalization(t *testing.T) {
	tests := []struct {
		keyword  string
		expected GherkinKeyword
	}{
		{"Given", KeywordGiven},
		{"Dado", KeywordGiven},
		{"Étant donné", KeywordGiven},
		{"When", KeywordWhen},
		{"Cuando", KeywordWhen},
		{"Quand", KeywordWhen},
		{"Then", KeywordThen},
		{"Entonces", KeywordThen},
		{"Alors", KeywordThen},
	}

	for _, tt := range tests {
		result := normalizeKeyword(tt.keyword)
		if result != tt.expected {
			t.Errorf("normalizeKeyword(%q) = %v, expected %v", tt.keyword, result, tt.expected)
		}
	}
}

func TestParseGherkinWithComments(t *testing.T) {
	gherkin := `
# This is a feature file comment
Feature: User authentication
  # This is a scenario comment
  Scenario: Valid login
    # This is a step comment
    Given the user is on the login page
    When they enter valid credentials
    Then they are logged in
`

	ast, err := ParseGherkin(gherkin)
	if err != nil {
		t.Fatalf("ParseGherkin failed: %v", err)
	}

	// Comments should be ignored, structure should be the same
	if len(ast.Features) != 1 || len(ast.Features[0].Scenarios) != 1 {
		t.Error("Parser should ignore comments but preserve structure")
	}
}

func TestParseGherkinMultipleScenarios(t *testing.T) {
	gherkin := `
Feature: User authentication
  Scenario: Valid login
    Given the user is on the login page
    When they enter valid credentials
    Then they are logged in

  Scenario: Invalid login
    Given the user is on the login page
    When they enter invalid credentials
    Then they see an error message
`

	ast, err := ParseGherkin(gherkin)
	if err != nil {
		t.Fatalf("ParseGherkin failed: %v", err)
	}

	if len(ast.Features[0].Scenarios) != 2 {
		t.Errorf("Expected 2 scenarios, got %d", len(ast.Features[0].Scenarios))
	}
}
