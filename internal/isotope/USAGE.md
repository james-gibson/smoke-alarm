# Isotope System Usage Guide

The isotope system provides language-agnostic test identity and Byzantine consensus verification for distributed smoke-alarm testing.

## Core Concepts

### 1. Canonicalization: Language-Agnostic Test Identity

An **isotope** is a cryptographic signature of a Gherkin test's semantic structure, independent of language or grammar.

```go
import "ocd-smoke-alarm/internal/isotope"

// Gherkin test in English
gherkinText := `
Feature: User authentication
  Scenario: Valid login
    Given the user is on the login page
    When they enter valid credentials
    Then they are logged in
`

// Generate isotope with fire-marshal signing key
signingKey := []byte("fire-marshal-secret-key")
iso, err := isotope.CanonicalizeGherkinToIsotope(
    gherkinText,
    signingKey,
    "iso-login",  // Family name
    1,            // Version (bumps on semantic change)
)
if err != nil {
    log.Fatal(err)
}

fmt.Println(iso.String())
// Output: iso-login-v1:abc123def456ghi7
```

### 2. Same Isotope Across Languages

The same test in different languages produces the same isotope:

```go
// Spanish version of same test
spanishGherkin := `
Característica: Autenticación de usuario
  Escenario: Inicio de sesión válido
    Dado que el usuario está en la página de inicio de sesión
    Cuando ingresa credenciales válidas
    Entonces ha iniciado sesión
`

iso2, _ := isotope.CanonicalizeGherkinToIsotope(spanishGherkin, signingKey, "iso-login", 1)

// iso.Signature == iso2.Signature
// Both tests are recognized as the same despite different languages
```

### 3. Semantic Change Bumps Version

When test semantics change, the version increments:

```go
// Same test family, but semantic shift (pre-auth instead of manual login)
semanticChange := `
Feature: User authentication
  Scenario: Valid login
    Given the user is pre-authenticated
    When the system validates the session
    Then the dashboard is displayed
`

iso3, _ := isotope.CanonicalizeGherkinToIsotope(semanticChange, signingKey, "iso-login", 2)
// iso3.Version = 2 (different semantic behavior)
```

## Result Tracking by Isotope

### Recording Test Results

```go
ledger := isotope.NewResultLedger()

// When a test runs, record its result keyed by isotope
result := isotope.TestResult{
    Isotope:     iso,
    Result:      true,          // PASS
    Reason:      "test passed",
    Timestamp:   time.Now(),
    AlarmID:     "alarm-a",
    FailureType: "",            // "timeout", "assertion", etc. on failure
    ExecutionMs: 125,
}

ledger.RecordResult(result)
```

### Tracking Through Refactoring

Grammatical refactoring keeps the same isotope, so results accumulate:

```go
// Week 1: Initial test
iso1, _ := isotope.CanonicalizeGherkinToIsotope(gherkinV1, key, "iso-login", 1)
ledger.RecordResult(isotope.TestResult{Isotope: iso1, Result: false, ...}) // FAIL
ledger.RecordResult(isotope.TestResult{Isotope: iso1, Result: false, ...}) // FAIL

// Week 2: Grammatically refactored (same isotope)
iso2, _ := isotope.CanonicalizeGherkinToIsotope(gherkinV2, key, "iso-login", 1)
// iso2.Signature == iso1.Signature (same isotope)
ledger.RecordResult(isotope.TestResult{Isotope: iso2, Result: false, ...}) // FAIL
ledger.RecordResult(isotope.TestResult{Isotope: iso2, Result: true, ...})  // PASS

// Week 3: Further refactoring
iso3, _ := isotope.CanonicalizeGherkinToIsotope(gherkinV3, key, "iso-login", 1)
// iso3.Signature == iso1.Signature (still same isotope)
ledger.RecordResult(isotope.TestResult{Isotope: iso3, Result: true, ...})  // PASS
ledger.RecordResult(isotope.TestResult{Isotope: iso3, Result: true, ...})  // PASS

// Query results for isotope-login-v1 across all refactoring
history := ledger.QueryByIsotope("iso-login", 1)
// history.Results has all 6 tests (even though text changed)
// history.Stability = 3/6 = 50%
// history.Trend = "improving" (was failing, now passing)
```

### Detecting Regressions

```go
// Query when a passing test started failing
isRegression, when := ledger.DetectRegression("iso-login", 1)
if isRegression {
    fmt.Printf("Regression detected at %s\n", when)
}
```

### Verifying Fixes

```go
// After deploying a fix, verify 4 consecutive passes
fixDeployedAt := time.Now()
isFix := ledger.VerifyFix("iso-login", 1, fixDeployedAt, 4)
if isFix {
    fmt.Println("Fix verified: 4 consecutive passes after deployment")
}
```

### Measuring Refactoring Effectiveness

```go
refactoringTime := time.Now()

// ... record results before and after refactoring ...

before, after := ledger.MeasureRefactoringEffectiveness("iso-login", 1, refactoringTime)
fmt.Printf("Stability improved: %.0f%% → %.0f%%\n", before*100, after*100)
```

## Byzantine Consensus

### Three-Alarm Consensus

Multiple smoke-alarms independently verify the same isotope:

```go
// Alarm A runs same test
isoA, _ := isotope.CanonicalizeGherkinToIsotope(gherkin, keyA, "iso-login", 1)
reportA := isotope.ConsensusReport{
    AlarmID:   "alarm-a",
    Isotope:   isoA,
    Signature: isoA.Signature,
    Timestamp: time.Now(),
}

// Alarm B runs same test
isoB, _ := isotope.CanonicalizeGherkinToIsotope(gherkin, keyB, "iso-login", 1)
reportB := isotope.ConsensusReport{
    AlarmID:   "alarm-b",
    Isotope:   isoB,
    Signature: isoB.Signature,
    Timestamp: time.Now().Add(10 * time.Millisecond),
}

// Alarm C runs same test
isoC, _ := isotope.CanonicalizeGherkinToIsotope(gherkin, keyC, "iso-login", 1)
reportC := isotope.ConsensusReport{
    AlarmID:   "alarm-c",
    Isotope:   isoC,
    Signature: isoC.Signature,
    Timestamp: time.Now().Add(20 * time.Millisecond),
}

// Verify consensus (need 2/3 quorum)
reports := []isotope.ConsensusReport{reportA, reportB, reportC}
status := isotope.VerifyIsotopeAgreement(reports, 2)

if status.ConsensusFormed {
    fmt.Printf("%d alarms agree on isotope (Byzantine tolerance: F=%d)\n",
        status.AgreedCount, status.ByzantineCount)
}
```

### Detecting Compromised Alarms

```go
// If one alarm reports different isotope
reportC.Isotope = isotope.Isotope{
    Family:    "iso-login",
    Version:   1,
    Signature: "wrongsignature", // Compromised!
}

// Consensus still forms (2/3 agree)
status := isotope.VerifyIsotopeAgreement(reports, 2)
if status.ConsensusFormed && len(status.Outliers) > 0 {
    compromised := isotope.DetectCompromisedAlarm(reports, 2)
    if compromised != nil {
        fmt.Printf("ALERT: %s is compromised or misconfigured\n", *compromised)
    }
}
```

## Integration with Smoke-Alarm

### Fire-Marshal Pre-Generates Tests

```go
// Fire-marshal generates test harness before deployment
harness := isotope.GenerateTestHarness(skillSpec, signingKey)

// Harness includes:
// - Entropy check
// - Input correlation
// - SLA compliance tests
// - Declared behavior tests
// All tagged with isotope
```

### Smoke-Alarm Runs Generated Tests

```go
// At runtime, smoke-alarm loads pre-generated harness
harness := loadFireMarshalHarness("data-aggregator-v2")

// Run all tests and record isotope
for _, test := range harness.Tests {
    result := test.Run(skillInstance)

    // Record with isotope tag
    isotope := harness.IsotopeRoot + "-" + test.Name
    ledger.RecordResult(isotope.TestResult{
        Isotope: parseIsotope(isotope),
        Result:  result,
        AlarmID: "smoke-alarm-prod-01",
    })
}
```

### Multi-Alarm Consensus Detection

```go
// Smoke-Alarm-A detects SLA violation
slaTestIsotope := "iso-sla-latency-data-agg-v1"
reportA := alarmA.TestAndReport(slaTestIsotope)

// Smoke-Alarm-B detects same violation
reportB := alarmB.TestAndReport(slaTestIsotope)

// Smoke-Alarm-C detects same violation
reportC := alarmC.TestAndReport(slaTestIsotope)

// All three form consensus
reports := []isotope.ConsensusReport{reportA, reportB, reportC}
status := isotope.VerifyIsotopeAgreement(reports, 2)

if status.ConsensusFormed {
    fmt.Println("CRITICAL: SLA violation confirmed by 3-alarm consensus")
    escapeToFireMarshal()
}
```

## Canonicalization Algorithm

The canonicalization process is deterministic and language-agnostic:

1. **Parse Gherkin** (language-aware) → Extract AST
2. **Normalize tokens** → Remove articles, punctuation, lowercase
3. **Create canonical form** → Language-agnostic structure
4. **Hash with fire-marshal key** → HMAC-SHA256
5. **Generate isotope** → family-vN:signature

This ensures:
- Same test in English/Spanish/French → Same isotope
- Grammatical refactoring → Same isotope (same version)
- Semantic change → Different isotope (bumped version)
- Deterministic and non-repudiable (signed with fire-marshal key)

## Testing the Isotope System

Run the test suite:

```bash
go test ./internal/isotope -v
```

Tests verify:
- ✓ Language-independent canonicalization
- ✓ Version bumping on semantic change
- ✓ Byzantine consensus with N >= 3F+1
- ✓ Outlier detection
- ✓ Result tracking through refactoring
- ✓ Regression and fix detection
- ✓ Flakiness identification
- ✓ Effectiveness measurement
