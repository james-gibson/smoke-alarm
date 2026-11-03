Feature: Isotope registry
  As a lab service that depends on another project
  I want a registered name to carry its project's route and the contract it implements
  So that a dependency can be resolved by project identity, not by a remembered URL

  # A NAME IS A HEARTBEAT. A registration lives for a TTL and is renewed by
  # re-registering; a node that dies without deregistering is swept. The name is
  # an instance; the project route is the project it belongs to (lezz
  # `seed.json` `project.route`); the contract is the signed spec it implements.
  #
  # Consumers resolve a dependency by route (where the project is) and pin the
  # contract id (what it was built against). Neither is evidence of trust: the
  # rung stays the registry's judgement.

  Background:
    Given an ocd-smoke-alarm with the isotope registry enabled
    And a heartbeat window of "5m"

  @isotope @registry
  Scenario: a registration is a heartbeat with a TTL
    When an instance registers with name "adhd-1a2b3c4d" at endpoint "127.0.0.1:9090"
    Then the registration reports status "live" and an expiry
    And re-registering the same name and endpoint renews the expiry
    And registered_at does not change

  @isotope @registry
  Scenario: a name that stops renewing stops resolving
    Given an instance registered with a "2s" heartbeat
    When the heartbeat is not renewed for "3s"
    Then the instance is absent from the registry list
    And resolving its name returns 404

  @isotope @registry
  Scenario: a node that shuts down cleanly removes its name
    Given a registered instance
    When the instance sends DELETE for its name
    Then resolving its name returns 404

  @isotope @identity
  Scenario: a registration carries the project's route and contract
    Given a registration for project route "01JABCDEF" implementing "the registry contract"
    Then the registry list shows that project route and contract id
    And resolving by route "01JABCDEF" returns the instance

  @isotope @identity
  Scenario: a dependency is referenced by route and pinned by contract
    Given a registration for "workspace-42" depending on project route "01JABCDEF"
    And the dependency pins contract "the registry contract"
    Then the registry list shows the dependency
    And resolving by contract "the registry contract" returns the registration

  @isotope @identity
  Scenario: a heartbeat does not erase identity
    Given an instance registered with a project route, contract, and dependency
    When it re-registers with only its name and endpoint
    Then the instance still carries its route, contract, and dependency

  @isotope @identity
  Scenario: an unknown route or contract resolves to nothing
    When a consumer resolves by route "no-such-project"
    Then the registry answers 404
