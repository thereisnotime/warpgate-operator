# Connection Management

## ADDED Requirements

### Requirement: Shared API Client per Connection

The operator SHALL reuse one Warpgate API client, and its HTTP connection pool, per `WarpgateConnection` across reconciles, and SHALL bound every API request with a timeout.

#### Scenario: Repeated reconciles reuse connections

- **WHEN** resources referencing the same connection are reconciled many times
- **THEN** the operator reuses keep-alive connections instead of opening a new one per reconcile

#### Scenario: Connection or Secret changes

- **WHEN** the connection's host, TLS settings, CA bundle or credentials change
- **THEN** the next reconcile builds a new client and closes the old client's idle connections

#### Scenario: Hung Warpgate

- **WHEN** Warpgate does not answer a request
- **THEN** the request fails after the client timeout (30s) or when the reconcile context ends
