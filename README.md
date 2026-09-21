# Casino

A learning project: an online casino platform built with Go microservices.

## Status

Phase 1 complete — service skeletons, local infra verified.
Phase 2 in progress — Auth service foundation.

## Requirements

- Go 1.22+
- PostgreSQL 16
- Redis 7
- RabbitMQ 3

## Local setup

```bash
make check   # verify Postgres / Redis / RabbitMQ are running
make build   # build all services
make test    # run tests
