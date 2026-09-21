# Casino Architecture

## Services

| Service | Responsibility | Port |
|---|---|---|
| api-gateway | Public REST/WebSocket entrypoint | 8080 |
| auth-service | Register, login, JWT, sessions | 8081 |
| wallet-service | Balances, deposits, withdrawals, bets/wins | 8082 |
| game-service | Game rounds, spin results, session state | 8083 |
| bonus-service | Promotions, free spins, wagering | 8084 |
| risk-service | Fraud rules, bonus abuse, velocity checks | 8085 |

## Communication

- External: REST/JSON and WebSocket via api-gateway.
- Internal: gRPC preferred; HTTP acceptable during early development.
- Async: RabbitMQ events, e.g. `wallet.bet.placed`, `wallet.win.credited`.

## Data ownership

- Each service owns its schema.
- No service reads another service's tables directly.
- Cross-service data goes through APIs or events.

## Money

- Stored as `int64` minor units.
- Example: $10.50 = `1050`.

## Database layout

Single database `casino`, one schema per service:

| Schema | Owner | Notes |
|---|---|---|
| auth | auth-service | users, sessions, refresh tokens |
| wallet | wallet-service | accounts, transactions |
| game | game-service | rounds, spins |
| bonus | bonus-service | promotions, claims |
| risk | risk-service | fraud events |

Rules:
- A service only touches its own schema.
- Migrations live under `migrations/<service>/`.
- golang-migrate tracks versions in `public.<service>_migrations`.