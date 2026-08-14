# design-software-training-environment-service (Go)

Capa de aplicación de `training-environment-service`. Esqueleto **hexagonal** (ADR-006): `internal/{domain,application,adapter}` + `cmd/` por deployable.
Regla: `cmd → adapter → application → domain`. Contract-first (SDD, shared-contracts) · TDD · OTel (pendiente).
