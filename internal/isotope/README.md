# Isotope: Language-Agnostic Test Identity & Byzantine Consensus

The `isotope` package provides stable cryptographic identifiers for tests and Byzantine fault-tolerant consensus verification for distributed smoke-alarm networks.

## What Problem Does This Solve?

### Problem 1: Test Identity Across Localization

Gherkin feature files can be localized to multiple languages, but tests need stable identifiers:

```gherkin
# English
Given the user is on the login page

# Spanish
Dado que el usuario está en la página de inicio de sesión

# French
Étant donné que l'utilisateur est sur la page de connexion
```

Without isotopes, each language produces a different identifier even though they're the same test. With isotopes, all three produce the same signature.

### Problem 2: Test Identity Through Refactoring

As tests are refactored, their text changes but their semantic meaning may be preserved:

```gherkin
# Week 1
Given the user is on the login page

# Week 2 (grammatical refactor)
Given user is at login page

# Week 3 (semantic change)
Given user is pre-authenticated
```

Without isotopes, these would be treated as three different tests, losing historical context. With isotopes:
- Weeks 1-2: Same isotope (grammatical variation)
- Week 3: Bumped version (semantic change)

Results accumulate under the same isotope through grammatical refactoring, but a new isotope version tracks semantic changes.

### Problem 3: Consensus Among Distributed Alarms

Multiple smoke-alarms running the same test need to agree on the test's identity and results:

- If all 3 alarms report the same isotope → consensus formed
- If 1 alarm reports a different isotope → outlier detected
- If signatures don't verify → alarm may be compromised

## Module Structure

### `canonicalization.go`

Converts Gherkin tests to language-agnostic canonical form:

- **`ParseGherkin()`** – Parses Gherkin in any supported language (English, Spanish, French, German, Japanese)
- **`Canonicalize()`** – Converts AST to language-agnostic tokens
- **`normalizeStepText()`** – Normalizes text (removes articles, punctuation, lowercases, tokenizes)
- **`GenerateIsotope()`** – Creates HMAC-SHA256 signed isotope
- **`CanonicalizeGherkinToIsotope()`** – Full pipeline (parse → canonicalize → sign)

Key behavior:
- Same semantic test in different languages → Same isotope signature
- Grammatical changes → Same version number
- Semantic changes → Bumped version number
- Deterministic hashing → Same input always produces same isotope

### `consensus.go`

Implements Byzantine fault-tolerant consensus verification:

- **`VerifyIsotopeAgreement()`** – Checks if N ≥ 3F+1 alarms agree on isotope
- **`VerifyConsensusSignatures()`** – Cryptographically verifies each alarm's signature
- **`TimestampOrdering()`** – Detects replay attacks via timestamp ordering
- **`DetectCompromisedAlarm()`** – Identifies outlier alarms
- **`TestEntropyCheck()`** – Verifies output randomness
- **`TestAgreementPattern()`** – Checks multi-alarm consensus

Key behavior:
- Requires quorum (e.g., 2/3 of 3 alarms, or 3/5 of 5 alarms)
- Byzantine tolerance: system tolerates F = ⌊(N-1)/3⌋ failures
- Signature verification prevents forged reports
- Timestamp ordering detects replay attacks

### `results.go`

Tracks test results keyed by isotope for analysis:

- **`ResultLedger`** – Stores all test results indexed by isotope
- **`RecordResult()`** – Adds a test result keyed by isotope
- **`QueryByIsotope()`** – Retrieves results for specific isotope version
- **`QueryByFamily()`** – Retrieves all versions of a test family
- **`DetectFlakiness()`** – Identifies tests with inconsistent results
- **`DetectRegression()`** – Finds when passing tests started failing
- **`VerifyFix()`** – Confirms consecutive passes after fix deployment
- **`MeasureRefactoringEffectiveness()`** – Compares stability before/after refactoring
- **`AggregateAcrossVariants()`** – Combines results from same isotope across different implementations

Key behavior:
- Results accumulate under same isotope across refactoring cycles
- Tracks stability trends (improving/stable/degrading)
- Detects flakiness through alternation patterns
- Measures refactoring effectiveness by comparing stability before/after
- Allows aggregating results across languages/implementations with same isotope

## Data Flow

### 1. Test Canonicalization (At Test Authoring / Fire-Marshal Time)

```
Gherkin File (any language)
    ↓
Parse with language awareness
    ↓
Extract AST (language-agnostic structure)
    ↓
Normalize tokens (remove articles, lowercase, etc.)
    ↓
Create canonical form (text structure)
    ↓
HMAC-SHA256 sign with fire-marshal key
    ↓
Isotope (family-vN:signature)
```

### 2. Test Execution & Result Recording (At Smoke-Alarm Runtime)

```
Run Test
    ↓
Canonicalize test → Get isotope
    ↓
Record: { isotope, result (pass/fail), timestamp, alarm-id }
    ↓
Store in ResultLedger
```

### 3. Multi-Alarm Consensus (When Alarms Detect Issues)

```
Alarm-A runs test → Reports: isotope-X, signature-A, timestamp-T1
Alarm-B runs test → Reports: isotope-X, signature-B, timestamp-T2
Alarm-C runs test → Reports: isotope-X, signature-C, timestamp-T3
    ↓
Verify isotope agreement: All three report isotope-X
    ↓
Verify signatures: All three signatures valid
    ↓
Verify timestamps: T1 < T2 < T3 (no replay)
    ↓
Consensus formed: 3/3 alarms agree
```

### 4. Analysis (For Reliability Engineering)

```
Query results for "iso-login-v1"
    ↓
Get all results across:
  - 3 grammatical refactorings (same isotope)
  - 3 different languages (same isotope)
  - 3 different test suites (same isotope)
    ↓
Calculate:
  - Stability: 42/45 = 93%
  - Trend: Improving (was 50%, now 100%)
  - Flakiness: 5% (low)
  - Regressions: 2 detected and fixed
    ↓
Conclusion: "Test is stable, refactoring helped, no open issues"
```

## Byzantine Fault Tolerance

The system implements Byzantine Fault Tolerance (BFT) consensus:

- **N alarms** can tolerate **F = ⌊(N-1)/3⌋** Byzantine failures
- Requires **quorum ≥ N - F** alarms in agreement
- Each alarm's report is cryptographically signed
- Timestamps prevent replay attacks
- Outliers are identified and investigated

Examples:
- 3 alarms: tolerate F=0 (all 3 must agree)
- 4 alarms: tolerate F=1 (need 3/4 agreement)
- 5 alarms: tolerate F=1 (need 4/5 agreement)
- 7 alarms: tolerate F=2 (need 5/7 agreement)

## Integration Points

### With Fire-Marshal

Fire-marshal generates test harnesses for skills. Each harness is tagged with isotope family and includes:
- Entropy check
- Input correlation test
- SLA latency test
- SLA error-rate test
- Declared behavior compliance tests

Example: `isotope-gen-data-aggregator-v2-v1`

### With Smoke-Alarm

Smoke-alarms load fire-marshal-generated harnesses and run tests. Each test result is tagged with isotope. When SLA violations or test failures occur:
1. Alarm records result with isotope
2. Compares isotope with other alarms' reports
3. If 3-alarm consensus forms → escalates to fire-marshal

### With Result Ledger

Operators query results by isotope to:
- Track test stability through refactoring
- Detect and respond to regressions
- Measure effectiveness of test improvements
- Aggregate results across multiple implementations/languages

## Security Properties

### Non-Repudiation

Isotopes are signed with fire-marshal's private key. An alarm cannot deny it reported a specific isotope.

### Tamper Detection

If an isotope is modified, the HMAC signature will not verify.

### Canonical Form is Authoritative

The canonical tokenized form (not the Gherkin text) is the definitive test specification. This prevents disputes about what a test "really" means.

## Example: Login Test Across 3 Languages

```
English:   "Given the user is on the login page"
Spanish:   "Dado que el usuario está en la página de inicio de sesión"
French:    "Étant donné que l'utilisateur est sur la page de connexion"

Canonical form (same for all):
  FEATURE:name:user-authentication
  SCENARIO:name:valid-login
  GIVEN:hash:user-on-login-page
  WHEN:hash:enter-valid-credentials
  THEN:hash:logged-in

Isotope: iso-login-v1:abc123def456ghi789
```

All three languages produce the same isotope because the canonical form is identical.

## Testing

Run the test suite:

```bash
go test ./internal/isotope -v
```

Tests cover:
- ✓ Gherkin parsing (all supported languages)
- ✓ Text normalization
- ✓ Canonical form generation
- ✓ HMAC signature generation
- ✓ Language-independent canonicalization
- ✓ Byzantine consensus (3, 4, 5+ alarms)
- ✓ Outlier detection
- ✓ Timestamp ordering
- ✓ Result recording and querying
- ✓ Regression detection
- ✓ Fix verification
- ✓ Refactoring effectiveness measurement
- ✓ Flakiness detection

See `USAGE.md` for detailed usage examples.
