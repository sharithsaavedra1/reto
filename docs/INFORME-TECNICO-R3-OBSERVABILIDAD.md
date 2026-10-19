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

### Prueba 1: Suite Completa de Tests de Observabilidad (`go test -v`)

Se ejecutó la suite de pruebas unitarias e integradas en el paquete `internal/observability`:

```text
=== RUN   TestTraceContext_ParseTraceparent
--- PASS: TestTraceContext_ParseTraceparent (0.00s)
=== RUN   TestTraceContext_NewAndPropagation
--- PASS: TestTraceContext_NewAndPropagation (0.00s)
=== RUN   TestREDMetricsRegistry
--- PASS: TestREDMetricsRegistry (0.00s)
=== RUN   TestObservability_MiddlewareIntegration
{"time":"2026-10-19T00:19:36.741193854Z","level":"INFO","msg":"HTTP request completed","service.name":"test-service","service.version":"1.0.0","environment":"test","correlation_id":"corr-test-12345","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"38cc43463d7fb539","traceparent":"00-4bf92f3577b34da6a3ce929d0e0e4736-38cc43463d7fb539-01","http.method":"POST","http.route":"/training-environments","http.status_code":201,"http.duration_ms":0.097,"http.client_ip":"192.0.2.1","http.user_agent":""}
--- PASS: TestObservability_MiddlewareIntegration (0.01s)
=== RUN   TestDistributedTracing_MFE_To_Service_To_Database
{"time":"2026-10-19T00:19:36.752786061Z","level":"INFO","msg":"HTTP request completed","service.name":"training-environment-service","service.version":"1.0.0","environment":"test","correlation_id":"4bf92f3577b34da6a3ce929d0e0e4736","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"30255d55861dd138","traceparent":"00-4bf92f3577b34da6a3ce929d0e0e4736-30255d55861dd138-01","http.method":"GET","http.route":"/training-environments","http.status_code":200,"http.duration_ms":5.605,"http.client_ip":"192.0.2.1","http.user_agent":""}
    observability_test.go:292: Distributed Trace Verified Successfully:
    observability_test.go:293:   MFE Span ID:      00f067aa0ba902b7
    observability_test.go:294:   HTTP Server Span: 30255d55861dd138 (Parent: 00f067aa0ba902b7)
    observability_test.go:295:   DB Client Span:   58b1f9abdd21fb00 (Parent: 30255d55861dd138)
    observability_test.go:296:   Shared Trace ID:  4bf92f3577b34da6a3ce929d0e0e4736
--- PASS: TestDistributedTracing_MFE_To_Service_To_Database (0.01s)
PASS
ok  	github.com/code-sena/design-software-training-environment-service/internal/observability	0.038s
```

### Prueba 2: Verificación de Todo el Repositorio (`go test ./...`)

```text
ok  	github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http	0.053s
ok  	github.com/code-sena/design-software-training-environment-service/internal/adapter/in/http/middleware	0.284s
ok  	github.com/code-sena/design-software-training-environment-service/internal/adapter/out/persistence	0.024s
ok  	github.com/code-sena/design-software-training-environment-service/internal/application/usecase	0.020s
ok  	github.com/code-sena/design-software-training-environment-service/internal/domain/model	0.020s
ok  	github.com/code-sena/design-software-training-environment-service/internal/observability	0.031s
```
**Resultado:** 100% de los tests del repositorio pasando satisfactoriamente.

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
    └── INFORME-TECNICO-R3-OBSERVABILIDAD.md            # Este informe técnico
```

---

*Informe técnico de revalidación elaborado y firmado por la aprendiza responsable:*  
**Emily Sharith Amézquita Saavedra**  
Ficha ADSO 3145555 — SENA  
*Rama Git:* `feature/r3-observability`
