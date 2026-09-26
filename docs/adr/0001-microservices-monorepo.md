# ADR 0001: Microservices in a Monorepo

## Status

Accepted — Phase 1.

## Context

We are building an online casino platform with distinct domains: auth,
wallet, game, bonus, and risk. Each domain has different scaling needs,
different data models, and different change cadences. A single-process
monolith would couple them together, making it hard to scale one domain
without scaling the others, and hard to enforce data ownership.

At the same time, splitting into multiple repositories from day one adds
friction: cross-repo changes require coordinated PRs, shared code needs a
private module registry or `replace` directives, and there is no single
place to see the whole system.

The team is currently one developer. Optimizing for coordination overhead
between teams is premature.

## Decision

Use a monorepo containing all services, each as its own Go module,
tied together by a `go.work` file.

- Each service under `services/<name>/` is its own module with its own
  `go.mod`, dependency list, and build target.
- Shared code lives under `pkg/` as its own module.
- `go.work` binds them together for local development only.
- Services communicate over HTTP (public) and, later, gRPC/events
  (internal). No direct function calls across service boundaries.
- Each service owns its schema. No service reads another service's tables.

## Consequences

### Easier

- **One `git clone` gets everything.** No cross-repo synchronization.
- **Shared code is trivial.** `pkg/` is importable from any service with
  no publishing step. Refactors that touch `pkg/` and multiple services
  land in one commit.
- **Local development is simple.** `make build` builds all modules;
  `make run-auth` runs one.
- **Atomic changes.** A breaking change to `pkg/` can be fixed in every
  caller in the same commit.

### Harder

- **Access control is all-or-nothing.** A contractor cannot be given
  access to only the wallet service.
- **Repo grows over time.** Cloning gets slower as history accumulates.
- **CI cannot easily run only the parts that changed** without extra
  tooling (path filters, per-module jobs).
- **Versioning is implicit.** Services are deployed from `main`, not
  from tagged releases. This is fine for now but will need revisiting
  when we have external consumers.

### Revisit when

- A second team joins and needs independent release cadence.
- The repo clone time exceeds a tolerable threshold (roughly >1 GB).
- Services need to be deployed on independent version schedules.

Until then, the monorepo provides more value than it costs.