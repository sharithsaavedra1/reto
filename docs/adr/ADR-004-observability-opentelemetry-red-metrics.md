# ADR-004: Adopción de OpenTelemetry (W3C Trace Context), Métricas RED, Logs Estructurados Correlacionados e Instrumentación de PostgreSQL

## Estado
**Aprobado / Accepted — Revalidación Oficial QA** (Fecha: 18 de octubre de 2026)

## Contexto y Declaración del Problema
Dentro del Macroproyecto Design Software — plataforma SENA (ADSO Ficha 3145555), el microservicio `training-environment-service` gestiona los ambientes de formación, reglas de disponibilidad, bloqueos por mantenimiento y reservas de aulas y talleres.

Al operar en una arquitectura de microservicios distribuida con frontend federado (Microfrontend en React 18 / Vite), pasarelas (Traefik / Reverse Proxy), servicios de autorización (IAM) y bases de datos relacionales (PostgreSQL 17), se presentan los siguientes requerimientos de observabilidad:
1. **Trazabilidad Distribuida End-to-End (MFE → Servicio → Base de Datos):** La transacción originada en el cliente debe mantener un único identificador (`trace_id`) que se preserve al ingresar por el middleware HTTP y continúe de forma jerárquica hasta la capa de persistencia en PostgreSQL (`otelsql`).
2. **Adopción del SDK Oficial de OpenTelemetry:** Integrar el ecosistema estándar `go.opentelemetry.io/otel`, configurando `TracerProvider`, exportadores OTLP/Stdout, propagadores globales W3C (`propagation.TraceContext`) y registro de spans exportables.
3. **Métricas RED por Endpoint (Rate, Errors, Duration):** Exposición de contadores e histogramas percentilados en formato nativo de Prometheus en `/metrics`.
4. **Logs Estructurados en JSON:** Con correlación unificada mediante `correlation_id`, `trace_id` y `span_id`.
5. **Grafana y Alertas como Código:** Provisión declarativa del tablero de control y regla de alerta ante tasa de error $> 5\%$.

---

## Decisión de Arquitectura

Se implementa una solución de observabilidad integral y estandarizada basada en el SDK oficial de OpenTelemetry y buenas prácticas de ingeniería de software:

### 1. SDK Oficial de OpenTelemetry y TracerProvider
- **Dependencias Estándar:**
  - `go.opentelemetry.io/otel`: Interfaz y núcleo global.
  - `go.opentelemetry.io/otel/trace`: Creación y contexto de spans.
  - `go.opentelemetry.io/otel/sdk/trace`: `TracerProvider`, procesadores (`BatchSpanProcessor`, `SimpleSpanProcessor`) y muestreo (`ParentBased(AlwaysSample)`).
  - `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc`: Exportador gRPC nativo hacia Tempo o Colectores OpenTelemetry.
  - `go.opentelemetry.io/otel/exporters/stdout/stdouttrace`: Exportador de salida estándar para depuración y verificación.
  - `go.opentelemetry.io/otel/propagation`: Propagadores compuestos de `TraceContext` y `Baggage`.
- **Propagación W3C Bidireccional:**
  - El middleware HTTP extrae la cabecera `traceparent` entrante del MFE mediante `otel.GetTextMapPropagator().Extract()`.
  - Inicia un Server Span con atributos semánticos HTTP (`http.method`, `http.target`, `http.client_ip`, `http.user_agent`, `http.status_code`).
  - Inyecta la cabecera de traza en la respuesta HTTP mediante `otel.GetTextMapPropagator().Inject()`, devolviendo también `Traceparent`, `X-Correlation-Id` y `X-Trace-Id`.

### 2. Instrumentación de Base de Datos PostgreSQL (MFE → Servicio → PostgreSQL)
- **Librería de Instrumentación:** Se adopta `github.com/XSAM/otelsql`, el estándar oficial para instrumentar la interfaz `database/sql` de Go con OpenTelemetry.
- **Creación de Spans de Base de Datos:**
  - La conexión se abre mediante `observability.OpenTracedPostgresDB(dbURL)`, que encapsula el driver `postgres` (`github.com/lib/pq`) bajo un proxy instrumentado con atributos `db.system=postgresql`.
  - Todas las operaciones ejecutadas por los repositorios (`QueryContext`, `ExecContext`, transacciones) reciben el `ctx context.Context` enriquecido por el middleware HTTP.
  - `otelsql` genera automáticamente spans de tipo **Client** como hijos directos del span del servidor HTTP, conservando de forma estricta el mismo `trace_id` y registrando el `db.statement` y latencia de cada consulta SQL.
  - Se registra además la recolección automática de métricas de rendimiento del pool de conexiones con `otelsql.RegisterDBStatsMetrics`.

### 3. Métricas RED (Rate, Errors, Duration) con Exportación Prometheus
- **Rate (Tasa):** Métrica de tipo contador `http_requests_total{method, path, status}` que contabiliza el volumen de transacciones.
- **Errors (Errores):** Contador `http_requests_errors_total{method, path, status}` para respuestas con código de estado HTTP $\ge 400$.
- **Duration (Duración):** Histograma `http_request_duration_seconds{method, path}` con 11 buckets normalizados (`0.005s` a `10s`) para cálculo de percentiles P50, P90, P95 y P99.
- **Sanitización de Etiquetas:** Parametrización automática de identificadores dinámicos UUID a `{id}`.
- **Endpoint Estándar:** Expuesto públicamente en `GET /metrics` bajo formato estándar de Prometheus.

### 4. Logs Estructurados en JSON con Correlación Unificada
- Se adopta `log/slog` direccionado a `stdout`.
- Cada registro incluye: `service.name`, `service.version`, `environment`, `correlation_id`, `trace_id`, `span_id`, `traceparent`, `http.method`, `http.route`, `http.status_code`, `http.duration_ms`, `http.client_ip`.
- Nivel dinámico: `INFO` para 2xx/3xx, `WARN` para 4xx y `ERROR` para 5xx.

### 5. Grafana y Alertas como Código
- Tablero aprovisionado en `deploy/grafana/dashboards/training-environment-red-dashboard.json`.
- Regla de alerta en `deploy/grafana/alerts/red-alerts.yml` ante tasa de error $> 5\%$.

---

## Diagrama de Trazabilidad Distribuida Unificada

```
[ Microfrontend (MFE React 18) ]
       │
       │ traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
       ▼
┌────────────────────────────────────────────────────────────────────────┐
│ training-environment-service (HTTP Server Span)                        │
│ Trace ID: 4bf92f3577b34da6a3ce929d0e0e4736                            │
│ Span ID:  30255d55861dd138 (Parent: 00f067aa0ba902b7)                  │
│ Atributos: http.method=GET, http.route=/training-environments          │
│                                                                        │
│   ├── Structured Log: {"msg":"HTTP request completed","trace_id":"..."}│
│   │                                                                    │
│   └── PostgreSQL Repository Query via otelsql (DB Client Span)        │
│       Trace ID: 4bf92f3577b34da6a3ce929d0e0e4736 (¡MISMO TRACE ID!)   │
│       Span ID:  58b1f9abdd21fb00 (Parent: 30255d55861dd138)           │
│       Atributos: db.system=postgresql, db.statement=SELECT ...         │
└────────────────────────────────────────────────────────────────────────┘
       │
       │ HTTP/1.1 200 OK
       │ Traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-30255d55861dd138-01
       ▼
[ Cliente MFE / Auditoría ]
```

---

## Consecuencias

### Positivas
- **Aprobación de Revalidación QA:** Resuelve al 100% el bloqueo de OpenTelemetry y trazabilidad hasta la base de datos PostgreSQL.
- **Interoperabilidad Total con Ecosistema Cloud-Native:** Compatible con OTLP, Grafana Tempo, Prometheus, Jaeger y Grafana Loki.
- **Limpieza del Repositorio:** El binario ejecutable `training-environment-api.exe` fue desvinculado del árbol de Git y añadido a `.gitignore`.
