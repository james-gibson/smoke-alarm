# features/simulation-validator.feature
# Canon record — last audited: 2026-03-26
# Exercises: simulation-validator skill
# Step definitions: features/step_definitions/simulation_validator_steps.go
# see: features/open-the-pickle-jar.feature (skill writing patterns)
# see: features/tui.feature (TUI integration)
# see: features/federation.feature (federation validation)

@simulation-validator @skill
Feature: Simulation Validator — Gherkin Feature Alignment and Secure Context Verification
  As a devops trainer running warroom simulations
  I want to validate that simulations align with Gherkin feature specs
  So that what we train matches what we spec matches what runs

  Background:
    Given the ocd-smoke-alarm binary is installed
    And the skill "simulation-validator" is installed at ".opencode/skills/simulation-validator"

  # ── Config Input ────────────────────────────────────────────────────────────

  Scenario: Validator accepts a config file path
    Given the config file "configs/samples/federation-star-hub.yaml" exists
    When the agent invokes "simulation-validator" with config "configs/samples/federation-star-hub.yaml"
    Then the validator reads the config file
    And extracts service parameters

  Scenario: Validator counts targets from config
    Given the config file "configs/samples/federation-pool.yaml" exists
    When the validator parses the config
    Then the config contains 5 targets

  Scenario: Validator detects federation configuration
    Given the config file "configs/samples/federation-mesh.yaml" exists
    Then the config has federation enabled with base_port 5100

  Scenario: Validator detects alert aggressiveness
    Given the config file "configs/samples/secrets-rotation.yaml" exists
    Then the config has alerts with aggressive set to false

  # ── Feature Mapping ─────────────────────────────────────────────────────────

  Scenario: Validator discovers relevant feature files for federation config
    Given the config file "configs/samples/federation-star-hub.yaml" exists
    When the validator discovers relevant feature files
    Then it finds features/federation.feature
    And it finds features/tui.feature
    And it finds features/alerts.feature

  Scenario: Validator discovers relevant feature files for deployment config
    Given the config file "configs/samples/deployment-canary.yaml" exists
    When the validator discovers relevant feature files
    Then it finds features/tui.feature
    And it finds features/ops.feature

  Scenario: Validator maps targets to federation feature steps
    Given the config file "configs/samples/federation-star-hub.yaml" exists
    When the validator maps targets to federation feature steps
    Then the simulation has participant for target "hub-coordinator"
    And the simulation has participant for target "spoke-files"
    And the validator finds 4 matching scenarios

  # ── Secure Context Verification ───────────────────────────────────────────

  Scenario: Validator checks OAuth configuration
    Given the config file "configs/samples/secure-context-handoff.yaml" exists
    When the validator checks OAuth configuration
    Then the OAuth provider "context-broker" is marked as OK
    And the OAuth provider "encrypted-store" is marked as OK

  Scenario: Validator verifies scope assignments
    Given the config file "configs/samples/multitenancy-isolation.yaml" exists
    When the validator checks OAuth configuration
    Then scope "tenant.corporate" is granted to 1 targets
    And scope "tenant.startup" is granted to 1 targets

  Scenario: Validator checks redaction is enabled
    Given the config file "configs/samples/oauth-mock-allow.yaml" exists
    When the validator checks redaction is enabled
    Then the redaction mask is "****"

  Scenario: Validator identifies weak scopes
    Given the config file "configs/samples/secure-context-handoff.yaml" exists
    When the validator checks OAuth configuration
    Then the OAuth provider "provider-engineering" is marked as WARNING
    And the scope analysis includes recommendations

  # ── Gap Analysis ───────────────────────────────────────────────────────────

  Scenario: Validator identifies gaps from simulation to feature
    Given the config file "configs/samples/federation-chain.yaml" exists
    When the validator identifies gaps from simulation to feature
    Then the validator identifies 1 gaps from simulation to feature
    And a gap exists: "malformed JSON error handling not simulated"

  Scenario: Validator identifies gaps from feature to simulation
    Given the config file "configs/samples/federation-star-hub.yaml" exists
    When the validator identifies gaps from feature to simulation
    Then the validator identifies 0 gaps from feature to simulation

  Scenario: Validator flags skills without feature documents
    Given the skill "warroom-builder" exists at ".opencode/skills/warroom-builder"
    And no feature file references "warroom-builder" or its exported types
    When the validator performs gap analysis
    Then a gap exists: "warroom-builder skill has no corresponding feature document"

  # ── TUI Integration ───────────────────────────────────────────────────────

  Scenario: Validator produces TUI integration guidance
    Given the config file "configs/samples/federation-star-hub.yaml" exists
    When the validator produces TUI integration guidance
    Then the TUI command is generated for the config
    And the keyboard shortcuts are documented
    And the TUI elements map to feature scenarios

  Scenario: TUI command is correctly formatted
    Given the config file "configs/samples/hosted-mcp-acp.yaml" exists
    When the validator produces TUI integration guidance
    Then the TUI command contains "--mode=foreground"
    And the TUI command contains the config path

  # ── Validation Report ─────────────────────────────────────────────────────

  Scenario: Validator generates complete report
    Given the config file "configs/samples/federation-star-hub.yaml" exists
    When the validator generates a validation report
    Then the report includes feature coverage section
    And the report includes security verification section
    And the report includes TUI observation section
    And the overall validation status is "PASSED WITH GAPS"

  Scenario: Report shows feature coverage metrics
    Given the config file "configs/samples/deployment-canary.yaml" exists
    When the validator generates a validation report
    Then the report shows 12 passed validations
    And the report shows 0 failed validations

  Scenario: Report includes security findings
    Given the config file "configs/samples/secrets-rotation.yaml" exists
    When the validator generates a validation report
    Then the report includes security verification section
    And the overall validation status is "PASSED"

  # ── Voice Modes ───────────────────────────────────────────────────────────

  Scenario: Validator supports warroom voice for urgent simulations
    Given the config file "configs/samples/federation-star-hub.yaml" exists
    When the simulation is requested in warroom voice
    Then the output uses urgent command language

  Scenario: Validator supports mentor voice for educational simulations
    Given the config file "configs/samples/federation-star-hub.yaml" exists
    When the simulation is requested in mentor voice
    Then the output uses explanatory language with pauses

  # ── Integration with config-to-simulation ────────────────────────────────

  Scenario: Validator integrates with config-to-simulation output
    Given a simulation was generated from config "configs/samples/federation-star-hub.yaml" in warroom mode
    When the validator runs on that simulation
    Then the validation result is "matched"

  # ── Edge Cases ───────────────────────────────────────────────────────────

  Scenario: Validator handles config with no targets
    Given a config with 0 targets exists
    When the validator generates a validation report
    Then the report shows 0 passed validations
    And the overall validation status is "NO TARGETS"

  Scenario: Validator handles config with no relevant features
    Given a config with no matching features exists
    When the validator discovers relevant feature files
    Then the validator identifies 0 gaps from feature to simulation
    And the report includes TUI observation section

  Scenario: Validator handles missing feature files gracefully
    Given a config "configs/samples/custom-unknown.yaml" exists
    When the validator discovers relevant feature files
    Then the validator identifies 1 gaps from feature to simulation
    And a gap exists: "no feature files found for this config type"
