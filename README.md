# design-software-training-environment-service (Go)

Capa de aplicación de `training-environment-service`. Esqueleto **hexagonal** (ADR-006): `internal/{domain,application,adapter}` + `cmd/` por deployable.
Regla: `cmd → adapter → application → domain`. Contract-first (SDD, shared-contracts) · TDD · OTel (pendiente).

## Verificación de QA para HU-ENV-003

Las pruebas unitarias y HTTP no requieren PostgreSQL. Las pruebas de integración deben ejecutarse
contra una base real usando `TEST_DATABASE_URL` (o `DATABASE_URL`). Para evitar que una ejecución
de revisión se marque como verde por omisión, se puede activar el modo obligatorio:

```powershell
$env:TEST_DATABASE_URL = "postgres://usuario:contraseña@localhost:5432/env_db?sslmode=disable"
$env:REQUIRE_POSTGRES_INTEGRATION = "1"
go test -count=1 -v ./internal/adapter/out/persistence
```

Con `REQUIRE_POSTGRES_INTEGRATION=1`, la ausencia de la URL, un fallo de conexión o un error al
preparar las tablas hace fallar la suite en lugar de omitirla. La base debe estar disponible para
la ejecución y los datos de prueba se limpian al finalizar.

La medición de E4 requiere una base PostgreSQL poblada y se ejecuta explícitamente con el tag
`performance`; no se declara ningún resultado hasta obtener una medición real:

```powershell
$env:PERF_DATE = "2026-09-15"
$env:PERF_SAMPLES = "100"
go test -tags performance -count=1 -v ./internal/adapter/out/persistence -run TestPostgresEnvironmentRepository_AvailabilityP95
```

La prueba registra el número de ambientes, las muestras, la fecha evaluada y el p95 observado.
Falla si la consulta de disponibilidad medida alcanza o supera 300 ms. El resultado de ese comando
es la evidencia que debe adjuntarse a la revisión de QA; una ejecución sin PostgreSQL poblado no
puede considerarse evidencia de cumplimiento.
