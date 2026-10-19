# Informe Técnico: Implementación del Reto R3 de Observabilidad

## Datos de identificación y control

| Dato | Información |
|---|---|
| **Aprendiz responsable** | Emily Sharith Amézquita Saavedra. |
| **Número de ficha** | 3145555. |
| **Programa de formación** | ADSO (Análisis y Desarrollo de Software) — SENA. |
| **Proyecto formativo** | Macroproyecto Design Software — plataforma SENA. |
| **Microservicio instrumentado** | `training-environment-service` (Gestión de ambientes de formación y reservas). |
| **Tipo de entrega** | Plan de mejoramiento individual — Reto R3: Observabilidad integral. |
| **Rúbrica oficial** | Acta 02, Numeral 10. |
| **Rama de trabajo** | `feature/r3-observability`. |
| **Fecha de ejecución** | 18 de octubre de 2026. |
| **Versión del informe** | 2.0 — Revalidación QA: SDK oficial OpenTelemetry, trazabilidad MFE → API → PostgreSQL y métricas RED. |
| **Resultado técnico** | **100% Exitoso / PASS**: Todos los criterios de la rúbrica implementados, bloqueadores de QA subsanados y validados con pruebas automatizadas. |

---

## 1. Resumen Ejecutivo y Resolución de Hallazgos QA

En el marco del plan de mejoramiento del programa ADSO (Ficha 3145555) y en respuesta a la revalidación emitida por el equipo de Aseguramiento de Calidad (QA) sobre el PR #19, se completó la integración total del **SDK oficial de OpenTelemetry**, eliminando la implementación manual y extendiendo la trazabilidad distribuida desde el Microfrontend (MFE) a través del servicio hasta las operaciones de persistencia en la base de datos PostgreSQL.

### Matriz de Resolución de Observaciones QA

| Componente Observado por QA | Estado Previo | Solución Implementada en esta Entrega | Estado Actual |
|---|---|---|:---:|
| **Dependencias OpenTelemetry** | `go.mod` contenía únicamente `github.com/lib/pq`. | Incorporadas dependencias oficiales: `go.opentelemetry.io/otel`, `otel/trace`, `otel/sdk`, `otel/exporters/otlp` y `otel/exporters/stdout`. | **PASS** |
| **TracerProvider & OTel SDK** | Sin TracerProvider oficial ni exportadores de spans. | Implementado `internal/observability/tracer.go` con `sdktrace.NewTracerProvider`, exportadores OTLP gRPC y Stdout, muestreo `ParentBased(AlwaysSample)` y apagado ordenado (`Shutdown`). | **PASS** |
| **Trazabilidad W3C hacia PostgreSQL** | Spans no exportables; sin instrumentación en BD. | Integrado `github.com/XSAM/otelsql`. Las conexiones a PostgreSQL (`observability.OpenTracedPostgresDB`) crean spans de tipo Client hijos del span HTTP para cada consulta SQL. | **PASS** |
| **Continuidad de Traza Distribuida** | No demostraba continuidad MFE → Servicio → DB. | Prueba automatizada `TestDistributedTracing_MFE_To_Service_To_Database` valida matemáticamente que MFE, HTTP y DB comparten el mismo `trace_id` con jerarquía padre-hijo estricta. | **PASS** |
| **Limpieza de Binarios Generados** | `training-environment-api.exe` estaba en el índice de Git. | Ejecutado `git rm --cached training-environment-api.exe` e ignorado permanentemente en `.gitignore`. | **PASS** |

---

## 2. Arquitectura de Observabilidad Implementada

### 2.1. Trazabilidad Distribuida de Extremo a Extremo (MFE → Servicio → PostgreSQL)

La traza distribuida implementa rigurosamente la recomendación del W3C (*W3C Trace Context Level 1*) y los estándares semánticos de OpenTelemetry:

```text
[ Microfrontend (MFE React 18 / Vite) ]
       │
       │ HTTP Request con Header:
       │ traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
       ▼
┌────────────────────────────────────────────────────────────────────────┐
│ training-environment-service (HTTP Server Span)                        │
│ Trace ID:  4bf92f3577b34da6a3ce929d0e0e4736                           │
│ Span ID:   30255d55861dd138                                            │
│ Parent ID: 00f067aa0ba902b7 (Span del MFE)                             │
│ Kind:      SERVER                                                      │
│ Atributos: http.method=GET, http.route=/training-environments          │
│                                                                        │
│   ├── Log Estructurado JSON:                                           │
│   │   {"msg":"HTTP request completed","trace_id":"...","span_id":"..."}│
│   │                                                                    │
│   └── Repositorio PostgreSQL instrumentado vía otelsql (DB Client Span)│
│       Trace ID:  4bf92f3577b34da6a3ce929d0e0e4736 (¡MISMO TRACE ID!)  │
│       Span ID:   58b1f9abdd21fb00                                      │
│       Parent ID: 30255d55861dd138 (Span HTTP del Servicio)             │
│       Kind:      CLIENT                                                │
│       Atributos: db.system=postgresql, db.statement=SELECT ...         │
└────────────────────────────────────────────────────────────────────────┘
       │
       │ HTTP/1.1 200 OK
       │ Traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-30255d55861dd138-01
       │ X-Correlation-Id: 4bf92f3577b34da6a3ce929d0e0e4736
       ▼
[ Cliente MFE / Pasarela ]
```

### 2.2. Métricas RED (Rate, Errors, Duration) en `/metrics`

Las métricas se exponen bajo el estándar textual de Prometheus 0.0.4:
1. **Rate:** `http_requests_total{method, path, status}` (Contador de volumen por endpoint).
2. **Errors:** `http_requests_errors_total{method, path, status}` (Contador de fallas HTTP $\ge 400$).
3. **Duration:** `http_request_duration_seconds_bucket{method, path, le="..."}` (Histograma con 11 buckets de latencia para cálculo de P50, P90, P95, P99).
4. **In-Flight:** `http_requests_in_flight` (Gauge de concurrencia activa).

### 2.3. Logs Estructurados JSON Correlacionados

Implementados mediante `log/slog` direccionado a `stdout`. Cada registro incluye:
- `service.name`: `training-environment-service`
- `environment`: `development` | `qa` | `production`
- `correlation_id`: Extraído de `x-correlation-id` o asignado al `trace_id`.
- `trace_id`, `span_id`, `traceparent`.
- `http.method`, `http.route`, `http.status_code`, `http.duration_ms`, `http.client_ip`.

---

## 3. Evidencias de Validación y Pruebas Automatizadas

### Evidencia 1: Ejecución de Tests con Docker Go Runtime (`go test -v ./internal/observability/...`)

Prueba automatizada dentro de contenedor Docker aislando el entorno de compilación de Go con el SDK OpenTelemetry cargado:

![Ejecución de Pruebas Unitarias Docker](img/Captura%20de%20pantalla%202026-10-18%20200900.png)

```text
=== RUN   TestTraceContext_ParseTraceparent
--- PASS: TestTraceContext_ParseTraceparent (0.00s)
=== RUN   TestTraceContext_NewAndPropagation
--- PASS: TestTraceContext_NewAndPropagation (0.00s)
```

### Evidencia 2: Verificación de Trazabilidad Distribuida Completa (MFE → HTTP → DB con otelsql)

Prueba automatizada de integración (`TestDistributedTracing_MFE_To_Service_To_Database`) que ejecuta una consulta SQL real sobre PostgreSQL utilizando una conexión instrumentada con `OpenTracedPostgresDB()` (`otelsql`). Se valida que el span de base de datos es generado automáticamente por `otelsql` sin instrumentación manual, compartiendo el `trace_id`, enlazando la jerarquía padre-hijo e incluyendo los atributos semánticos de la consulta ejecutada:

![Verificación de Traza Distribuida MFE a DB](img/Captura%20de%20pantalla%202026-10-18%20200918.png)

```text
=== RUN   TestREDMetricsRegistry
--- PASS: TestREDMetricsRegistry (0.00s)
=== RUN   TestObservability_MiddlewareIntegration
{"time":"2026-10-19T20:29:06.844Z","level":"INFO","msg":"HTTP request completed","service.name":"test-service","correlation_id":"corr-test-12345","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"cfb49a82f46c25d1","traceparent":"00-4bf92f3577b34da6a3ce929d0e0e4736-cfb49a82f46c25d1-01","http.method":"POST","http.route":"/training-environments","http.status_code":201}
--- PASS: TestObservability_MiddlewareIntegration (0.01s)
=== RUN   TestDistributedTracing_MFE_To_Service_To_Database
{"time":"2026-10-19T20:29:07.083Z","level":"INFO","msg":"HTTP request completed","service.name":"training-environment-service","correlation_id":"4bf92f3577b34da6a3ce929d0e0e4736","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"535cb3ee50a7e6db","traceparent":"00-4bf92f3577b34da6a3ce929d0e0e4736-535cb3ee50a7e6db-01","http.method":"GET","http.route":"/training-environments","http.status_code":200}
    observability_test.go:277: Total spans recorded: 3
    observability_test.go:279: Span[0]: Name="sql.conn.query", Kind=client, TraceID=4bf92f3577b34da6a3ce929d0e0e4736, SpanID=4ff16ebaee0ad236, ParentSpanID=535cb3ee50a7e6db
    observability_test.go:282:    attr: db.system=postgresql
    observability_test.go:282:    attr: db.name=training_environment_db
    observability_test.go:282:    attr: db.query.text=SELECT 1 AS trace_check
    observability_test.go:279: Span[1]: Name="sql.rows", Kind=client, TraceID=4bf92f3577b34da6a3ce929d0e0e4736, SpanID=96b80354a2fe0c28, ParentSpanID=535cb3ee50a7e6db
    observability_test.go:282:    attr: db.system=postgresql
    observability_test.go:282:    attr: db.name=training_environment_db
    observability_test.go:279: Span[2]: Name="GET /training-environments", Kind=server, TraceID=4bf92f3577b34da6a3ce929d0e0e4736, SpanID=535cb3ee50a7e6db, ParentSpanID=00f067aa0ba902b7
    observability_test.go:282:    attr: http.method=GET
    observability_test.go:282:    attr: http.target=/training-environments
    observability_test.go:282:    attr: http.status_code=200
    observability_test.go:362: Distributed Trace Verified Successfully:
    observability_test.go:363:   MFE Span ID:      00f067aa0ba902b7
    observability_test.go:364:   HTTP Server Span: 535cb3ee50a7e6db (Parent: 00f067aa0ba902b7)
    observability_test.go:365:   DB Client Span:   4ff16ebaee0ad236 (Parent: 535cb3ee50a7e6db)
    observability_test.go:366:   Shared Trace ID:  4bf92f3577b34da6a3ce929d0e0e4736
--- PASS: TestDistributedTracing_MFE_To_Service_To_Database (0.24s)
PASS
```

### Evidencia 3: Validación del Endpoint `/metrics` (Métricas RED) y Propagación `traceparent` vía cURL

Respuesta HTTP emitida al consultar el servicio activo con la cabecera W3C `traceparent`:

![Respuesta cURL con Métricas RED y Headers W3C](img/Captura%20de%20pantalla%202026-10-18%20201524.png)

```text
# HELP http_requests_total Total number of HTTP requests processed (RED Rate metric)
# TYPE http_requests_total counter
http_requests_total{method="GET",path="/health",status="200"} 1

# HELP http_requests_errors_total Total number of HTTP requests returning errors (status >= 400) (RED Errors metric)
# TYPE http_requests_errors_total counter

# HELP http_request_duration_seconds HTTP request latencies in seconds (RED Duration metric)
# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{method="GET",path="/health",le="0.005"} 1
...
HTTP/1.1 200 OK
Content-Type: application/json
Traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-838d77dd5db8ebeb-01
X-Correlation-Id: 4bf92f3577b34da6a3ce929d0e0e4736
X-Trace-Id: 4bf92f3577b34da6a3ce929d0e0e4736
```

### Evidencia 4: Despliegue de la Infraestructura de Observabilidad con Docker Compose

Levantamiento de los contenedores de Grafana y Prometheus utilizando `docker compose`:

![Despliegue Docker Compose Grafana y Prometheus](img/Captura%20de%20pantalla%202026-10-18%20203115.png)

```text
[+] up 25/25
 ✔ Image grafana/grafana:10.1.0        Pulled
 ✔ Image prom/prometheus:v2.47.0       Pulled
 ✔ Container training-env-prometheus   Recreated
 ✔ Container training-env-grafana      Recreated
```

### Evidencia 5: Tablero Grafana de Métricas RED y Umbral de Alerta

Visualización gráfica en tiempo real en Grafana con los paneles de Tasa (Rate), Errores (Errors %), Latencia (P50, P95, P99) y Umbral de Alerta al 5%:

![Tablero Grafana RED y Alerta de Error](img/Captura%20de%20pantalla%202026-10-18%20204407.png)

---

## 4. Tabla de Equivalencia Técnica: Go vs Spring Boot

Para facilitar la sustentación técnica ante el comité de evaluación del SENA, se detalla la equivalencia entre las tecnologías implementadas en Go y su par en el stack Java / Spring Boot:

| Concepto de la Rúbrica | Implementación en este Microservicio (Go) | Equivalente en Java / Spring Boot |
|---|---|---|
| **SDK de OpenTelemetry** | `go.opentelemetry.io/otel` + `sdktrace.NewTracerProvider` | `io.opentelemetry:opentelemetry-sdk` + OpenTelemetry Java Agent |
| **Trazabilidad W3C HTTP** | `internal/observability/middleware.go` con `otel.GetTextMapPropagator` | `io.micrometer:micrometer-tracing-bridge-otel` + Spring Web Filter |
| **Trazabilidad en Base de Datos** | `github.com/XSAM/otelsql` (`OpenTracedPostgresDB`) | `io.opentelemetry.instrumentation:opentelemetry-jdbc` o Spring Data JPA OTel Driver |
| **Métricas RED** | `internal/observability/metrics.go` (`http_requests_total`, `http_requests_errors_total`, `http_request_duration_seconds`) | `spring-boot-starter-actuator` + `micrometer-registry-prometheus` (`http.server.requests`) |
| **Endpoint de Métricas** | `GET /metrics` expuesto en HTTP ServeMux | `GET /actuator/prometheus` en Spring Actuator |
| **Logging Estructurado** | `log/slog` con `JSONHandler` inyectando `correlation_id` y `trace_id` | `net.logstash.logback:logstash-logback-encoder` con `MDC.put("trace_id", ...)` |
| **Tablero y Alerta Grafana** | Provisioning as Code en `deploy/grafana/` con alerta al 5% | Provisioning as Code idéntico en Grafana / Prometheus Alertmanager |

---

## 5. Estructura de Archivos del Módulo de Observabilidad

```text
design-software-training-environment-service/
├── cmd/
│   └── training-environment-api/
│       └── main.go                                     # Inicialización de OTel y conexión PostgreSQL traced
├── internal/
│   └── observability/
│       ├── tracer.go                                   # OpenTelemetry TracerProvider, OTLP/Stdout y otelsql
│       ├── trace.go                                    # W3C Trace Context y puente con trace.SpanFromContext
│       ├── metrics.go                                  # Registro thread-safe de métricas RED y Prometheus
│       ├── logger.go                                   # Logger JSON estructurado correlacionado
│       ├── middleware.go                               # Middleware HTTP con Server Spans y cabeceras W3C
│       └── observability_test.go                       # Suite de pruebas unitarias y de integración distribuida
├── deploy/
│   ├── docker-compose.observability.yml                # Stack desplegable de Grafana + Prometheus
│   ├── prometheus/
│   │   └── prometheus.yml                              # Scrapeo automático de /metrics
│   └── grafana/
│       ├── provisioning/
│       │   ├── datasources/prometheus-datasource.yml   # Datasource Prometheus automático
│       │   └── dashboards/dashboards-provisioning.yml  # Provisioning desatendido de tableros
│       ├── dashboards/
│       │   └── training-environment-red-dashboard.json # Tablero RED oficial con paneles y alertas
│       └── alerts/
│           └── red-alerts.yml                          # Regla de alerta ante Error Rate > 5%
└── docs/
    ├── adr/
    │   └── ADR-004-observability-opentelemetry-red-metrics.md # Registro de Decisión Arquitectónica
    ├── img/                                            # Evidencias gráficas (capturas de pantalla)
    │   ├── Captura de pantalla 2026-10-18 200900.png
    │   ├── Captura de pantalla 2026-10-18 200918.png
    │   ├── Captura de pantalla 2026-10-18 201524.png
    │   ├── Captura de pantalla 2026-10-18 203115.png
    │   └── Captura de pantalla 2026-10-18 204407.png
    └── INFORME-TECNICO-R3-OBSERVABILIDAD.md            # Este informe técnico
```

---

*Informe técnico de revalidación elaborado y firmado por la aprendiza responsable:*  
**Emily Sharith Amézquita Saavedra**  
Ficha ADSO 3145555 — SENA  
*Rama Git:* `feature/r3-observability`