# features/open-the-pickle-jar.feature
# Canon record — last audited: 2026-03-26
# Exercises: open-the-pickle-jar skill — Gherkin authoring, drift audit, coverage reporting
# Code: .opencode/skills/open-the-pickle-jar/SKILL.md
# Step definitions: features/step_definitions/open_the_pickle_jar_steps.go
# see: features/skill-system.feature (ValidateSkillFile, FindSkills)
# see: features/federated-skills.feature (federated context invocation scenarios)
#
# SKILL CONTRACT:
#   This skill has two modes:
#     Direct (no scope)  → scan features/, internal/, skills/ — report coverage gaps
#     Targeted (scope)   → read domain source, write canonical .feature for that domain
#
#   Every feature file it produces MUST conform to the 10 Cucumber step authoring rules
#   documented in SKILL.md. This feature file is the self-description of those rules.
#
# NAMESPACING NOTE:
#   In a federated mesh, this skill is invoked as "[instance-id] open-the-pickle-jar".
#   Local invocation retains the unqualified name. See: features/federated-skills.feature.

@open-the-pickle-jar @skill
Feature: Open the Pickle Jar — Gherkin Authoring and Drift Audit Skill
  As a developer or agent working on ocd-smoke-alarm
  I want to invoke open-the-pickle-jar to produce and audit canonical Gherkin specs
  So that every domain slice has an executable specification and drift is surfaced early

  Background:
    Given a Claude Code session is active in this repository
    And the skill "open-the-pickle-jar" is installed at ".opencode/skills/open-the-pickle-jar/SKILL.md"

  # ── SKILL.md validity ─────────────────────────────────────────────────────

  Scenario: the open-the-pickle-jar SKILL.md file is valid and passes skill validation
    When ValidateSkillFile is called on ".opencode/skills/open-the-pickle-jar/SKILL.md"
    Then the result is valid with no errors
    And the skill name matches the directory name "open-the-pickle-jar"

  Scenario: the SKILL.md description contains the words "Gherkin" and "drift"
    Given the skill "open-the-pickle-jar" SKILL.md is read
    Then the description field contains the word "Gherkin"
    And the description field contains the word "drift"

  # ── direct invocation (no scope) ─────────────────────────────────────────

  Scenario: direct invocation with no scope produces a coverage summary
    When the agent invokes the skill "open-the-pickle-jar" with no scope argument
    Then the output lists all existing feature files in "features/"
    And the output identifies any internal packages without Gherkin coverage
    And the output proposes the highest-value next feature to write

  Scenario: direct invocation identifies a skill with no corresponding feature file
    Given a skill "some-new-skill" is installed at ".opencode/skills/some-new-skill/SKILL.md"
    And no "features/some-new-skill.feature" exists
    When the agent invokes the skill "open-the-pickle-jar" with no scope argument
    Then the output flags "some-new-skill" as lacking a feature document

  Scenario: direct invocation identifies an internal package with no feature coverage
    Given a package "internal/somepackage/" exists with Go source files
    And no feature file references "somepackage" or its exported types
    When the agent invokes the skill "open-the-pickle-jar" with no scope argument
    Then the output includes "somepackage" in the uncovered packages list

  Scenario: direct invocation output is stable across repeated calls with identical state
    Given the agent has invoked "open-the-pickle-jar" once and the output is recorded
    When the agent invokes "open-the-pickle-jar" again with no scope argument
    Then the second output is equivalent to the first
    And no new files are written to "features/" or "state/"

  # ── targeted invocation (with scope) ─────────────────────────────────────

  Scenario: targeted invocation with scope "somedomain" writes a feature file for that domain
    Given no "features/somedomain.feature" exists
    When the agent invokes "open-the-pickle-jar" with scope argument "somedomain"
    Then a file is written at "features/somedomain.feature"
    And the file begins with a canon record comment containing today's date
    And the file contains exactly one "Feature:" block

  Scenario: the written feature file uses the scope name as its primary tag
    When the agent writes a feature file for scope "somedomain"
    Then the feature file contains "@somedomain" as a tag

  Scenario: targeted invocation reads source files from "internal/<scope>/" before writing
    Given source files exist in "internal/somedomain/"
    When the agent invokes "open-the-pickle-jar" with scope "somedomain"
    Then the agent reads the Go source files in that directory
    And the written scenarios reference exported types from those source files

  Scenario: targeted invocation cross-references overlapping feature files
    Given "features/related.feature" contains scenarios that touch the target domain
    When the agent writes a feature file for scope "somedomain"
    Then the new feature file header includes a "see:" reference to "features/related.feature"

  # ── step authoring rules enforcement ─────────────────────────────────────

  Scenario: every When step in a produced feature file begins with a verb in imperative form
    When the agent produces a feature file for any scope
    Then every "When" step starts with a verb in imperative form
    And no "When" step begins with "the" or "a" as the first word

  Scenario: every variable value in a produced feature file uses a Cucumber expression type
    When the agent produces a feature file for any scope
    Then no step text contains a hardcoded string literal where a placeholder could be used
    And no step text contains a hardcoded integer where a placeholder could be used

  Scenario: no two steps in the same feature file are synonymous paraphrases
    When the agent produces a feature file for any scope
    Then no two step texts express the same action with different wording
    And no step text uses "run" where another uses "execute" for the same concept

  Scenario: every Scenario Outline in a produced feature file has an Examples table with a header row
    When the agent produces a feature file containing a Scenario Outline
    Then an "Examples:" block immediately follows each Scenario Outline
    And the Examples table has a header row with column names

  # ── drift audit ───────────────────────────────────────────────────────────

  Scenario: drift audit compares each scenario against source code and flags mismatches
    Given a feature file "features/somedomain.feature" and source code in "internal/somedomain/"
    When the agent performs a drift audit on scope "somedomain"
    Then the audit output contains a drift table with columns: Scenario, Feature, Code, Tests, Verdict

  Scenario: drift audit flags a scenario that references a non-existent exported symbol
    Given "features/somedomain.feature" references a function "NonExistentFunc" that does not exist in source
    When the agent performs a drift audit on scope "somedomain"
    Then the Verdict for that scenario is "DRIFT"
    And the audit suggests updating the feature or the source to reconcile

  Scenario: drift audit flags a test file scenario missing from the feature spec
    Given "tests/somedomain_test.go" tests a behaviour not covered by any scenario
    When the agent performs a drift audit on scope "somedomain"
    Then the audit flags the uncovered behaviour as a gap

  Scenario: drift audit produces a THESIS-FINDING entry for each unresolved drift
    Given a drift audit finds 2 mismatches
    When the agent writes the audit output
    Then the output contains 2 "THESIS-FINDING:" entries suitable for appending to TASKS.md

  # ── step definition implementation audit ─────────────────────────────────

  Scenario: direct invocation checks step definition files for pending markers
    Given step definition files exist in "features/step_definitions/"
    When the agent invokes "open-the-pickle-jar" with no scope argument
    Then the output includes a step-definition implementation summary
    And each step definition file is classified as "implemented", "partial", or "stub"

  Scenario: a fully-stubbed step definition file is recorded as a THESIS-FINDING
    Given a step definition file "features/step_definitions/somedomain_steps.go" exists
    And every step function in that file returns godog.ErrPending
    When the agent performs an audit pass
    Then the output contains a THESIS-FINDING entry for "somedomain_steps.go"
    And the THESIS-FINDING notes that Cucumber coverage is nominal, not executable

  Scenario: a partially-stubbed step definition file is recorded as a THESIS-FINDING
    Given a step definition file "features/step_definitions/somedomain_steps.go" exists
    And some step functions are implemented and some return godog.ErrPending
    When the agent performs an audit pass
    Then the THESIS-FINDING entry classifies the file as "partial"

  Scenario: a fully-implemented step definition file produces no THESIS-FINDING
    Given a step definition file "features/step_definitions/somedomain_steps.go" exists
    And no step function returns godog.ErrPending
    When the agent performs an audit pass
    Then no THESIS-FINDING is recorded for "somedomain_steps.go"

  Scenario: THESIS-FINDING entries for stubbed step definitions are appended to TASKS.md
    Given the audit finds 3 stubbed step definition files
    When the agent writes the audit output
    Then "TASKS.md" contains a "THESIS-FINDING:" section
    And each stubbed file has a corresponding entry in that section

  Scenario: absence of a step_definitions directory is noted but not treated as a failure
    Given no "features/step_definitions/" directory exists
    When the agent invokes "open-the-pickle-jar" with no scope argument
    Then the output notes that no step definition stubs exist
    And the output proposes scaffolding stubs for the highest-priority domain
    And the audit does not fail or exit with an error

  # ── output location rules ─────────────────────────────────────────────────

  Scenario: the skill writes feature files to "features/<scope>.feature" only
    When the agent writes output for scope "somedomain"
    Then the output file path is "features/somedomain.feature"
    And no files are written outside the "features/" directory

  Scenario: step definition stubs are written only when explicitly requested
    When the agent invokes "open-the-pickle-jar" with scope "somedomain" and no stub request
    Then no file is written to "features/step_definitions/"

  Scenario: step definition stubs are written to "features/step_definitions/<scope>_steps.go" when requested
    When the agent invokes "open-the-pickle-jar" with scope "somedomain" and stub generation requested
    Then a stub file is written at "features/step_definitions/somedomain_steps.go"
    And each step in the feature file has a corresponding stub function

  # ── federated context ─────────────────────────────────────────────────────
  # see: features/federated-skills.feature for routing and cycle-rejection scenarios

  Scenario: open-the-pickle-jar invoked directly on a federated instance reports that instance's coverage
    Given the local instance is operating within a federation mesh
    When the agent invokes "open-the-pickle-jar" with no scope argument
    Then the output identifies the local instance by its instance ID and role
    And the output notes which feature files exist locally on this instance

  Scenario: open-the-pickle-jar targeted with a federated scope routes the audit to the named instance
    Given instance "inst-b" (id "bbbb000000000002") is a registered peer
    When the agent invokes "open-the-pickle-jar" with scope "[bbbb000000000002]"
    Then the invocation is routed to inst-b
    And the resulting audit output covers inst-b's local feature files and skill domain
