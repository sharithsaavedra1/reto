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
| **Versión del informe** | 1.0 — Entrega técnica de observabilidad, trazas, métricas RED, logs y Grafana. |
| **Resultado técnico** | **100% Exitoso**: 5 de 5 criterios de la rúbrica implementados y validados con pruebas automatizadas. |

---

## 1. Resumen Ejecutivo

En el marco del plan de mejoramiento del programa ADSO (Ficha 3145555), se diseñó e implementó la suite completa de observabilidad sobre el microservicio `training-environment-service`.

La solución cumple de extremo a extremo con los requerimientos de la rúbrica de aseguramiento de calidad (Acta 02, Numeral 10), garantizando que cualquier petición que ingrese desde el Microfrontend (MFE) o pasarela de enlace sea rastreada, medida y registrada con identidad única a través de todas las capas de la arquitectura hexagonal.

---

## 2. Cumplimiento de Criterios Técnicos de la Rúbrica

### Criterio 1: Traza Distribuida (OpenTelemetry & W3C Trace Context)
- **Especificación:** Se implementó el estándar W3C Trace Context (*W3C Recommendation / OpenTelemetry*) mediante el paquete `internal/observability`.
- **Estructura de la cabecera:** Se valida y genera la cabecera estándar:
  ```http
  traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
  ```
  Donde:
  - `00`: Versión actual W3C.
  - `4bf92f3577b34da6a3ce929d0e0e4736`: Identificador único de traza (16 bytes = 32 caracteres hexadecimales).
  - `00f067aa0ba902b7`: Identificador del tramo/span (8 bytes = 16 caracteres hexadecimales).
  - `01`: Bandera de muestreo (`sampled`).
- **Propagación bidireccional:**
  - El middleware HTTP `ObservabilityMiddleware` extrae la cabecera `traceparent` entrante y continúa la traza. Si está ausente, genera una nueva con entropía criptográfica (`crypto/rand`).
  - El contexto se inyecta en el `context.Context` de Go y se propaga a las capas de aplicación y persistencia (PostgreSQL).
  - El servicio devuelve `traceparent`, `x-correlation-id` y `x-trace-id` en las cabeceras de respuesta HTTP para que el MFE pueda correlacionar el resultado con la vista del usuario.

### Criterio 2: Métricas RED (Rate, Errors, Duration)
- **Métricas expuestas en `/metrics` (formato estándar Prometheus):**
  1. **Rate (Tasa de peticiones):**
     ```prometheus
     # HELP http_requests_total Total number of HTTP requests processed (RED Rate metric)
     # TYPE http_requests_total counter
     http_requests_total{method="GET",path="/training-environments",status="200"} 42
     ```
  2. **Errors (Tasa de errores):**
     ```prometheus
     # HELP http_requests_errors_total Total number of HTTP requests returning errors (status >= 400) (RED Errors metric)
     # TYPE http_requests_errors_total counter
     http_requests_errors_total{method="GET",path="/training-environments",status="401"} 3
     ```
  3. **Duration (Duración / Latencia):**
     ```prometheus
     # HELP http_request_duration_seconds HTTP request latencies in seconds (RED Duration metric)
     # TYPE http_request_duration_seconds histogram
     http_request_duration_seconds_bucket{method="GET",path="/training-environments",le="0.005"} 15
     http_request_duration_seconds_bucket{method="GET",path="/training-environments",le="0.01"} 25
     ...
     http_request_duration_seconds_sum{method="GET",path="/training-environments"} 0.354000
     http_request_duration_seconds_count{method="GET",path="/training-environments"} 42
     ```
  4. **Concurrencia (In-flight):**
     ```prometheus
     # HELP http_requests_in_flight Current number of HTTP requests being served
     # TYPE http_requests_in_flight gauge
     http_requests_in_flight 0
     ```
- **Control de cardinalidad:** Los identificadores dinámicos UUID en las rutas se sustituyen automáticamente por `{id}` para prevenir explosión de memoria en Prometheus/Grafana.

### Criterio 3: Logs Estructurados con Correlación (`correlation_id`)
- **Motor:** Se implementó `slog.NewJSONHandler` emitiendo eventos estructurados en formato JSON por `stdout`.
- **Campos incluidos en cada traza de log:**
  ```json
  {
    "time": "2026-10-18T13:34:33.6934279-05:00",
    "level": "INFO",
    "msg": "HTTP request completed",
    "service.name": "training-environment-service",
    "service.version": "1.0.0",
    "environment": "development",
    "correlation_id": "adso-3145555-emily-amezquita-corr-001",
    "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
    "span_id": "00f067aa0ba902b7",
    "traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
    "http.method": "GET",
    "http.route": "/training-environments",
    "http.status_code": 200,
    "http.duration_ms": 3.42,
    "http.client_ip": "127.0.0.1",
    "http.user_agent": "Mozilla/5.0..."
  }
  ```
- **Niveles dinámicos:** Código HTTP $2xx/3xx \rightarrow \text{INFO}$, $4xx \rightarrow \text{WARN}$, $5xx \rightarrow \text{ERROR}$.

### Criterio 4: Tablero de Grafana y Alerta como Código
- **Provisioning as Code:**
  - `deploy/grafana/provisioning/datasources/prometheus-datasource.yml`: Configura el origen de datos Prometheus de manera desatendida.
  - `deploy/grafana/provisioning/dashboards/dashboards-provisioning.yml`: Carga automáticamente los tableros al arrancar el contenedor.
  - `deploy/grafana/dashboards/training-environment-red-dashboard.json`: Tablero con paneles de visualización para:
    - *Panel Stat:* Rate global (RPS).
    - *Panel Stat:* Tasa de error (%) en tiempo real.
    - *Panel Stat:* Latencia P95.
    - *Panel Stat:* Peticiones concurrentes en vuelo.
    - *Panel Time Series:* Tasa de peticiones por endpoint.
    - *Panel Time Series:* Desglose de errores por código HTTP.
    - *Panel Time Series con Umbral SLA (5%):* Tasa de error porcentual con línea roja de alerta.
    - *Panel Time Series:* Distribución percentilada P50, P95 y P99.
- **Regla de Alerta (> 5% de error):**
  Definida en `deploy/grafana/alerts/red-alerts.yml`:
  ```yaml
  - alert: TrainingEnvironmentHighErrorRate
    expr: (sum(rate(http_requests_errors_total[5m])) / sum(rate(http_requests_total[5m]))) * 100 > 5
    for: 1m
    labels:
      severity: critical
      service: training-environment-service
    annotations:
      summary: "Tasa de error HTTP crítica (> 5%) en training-environment-service"
      description: "La tasa de errores HTTP es del {{ $value | printf \"%.2f\" }}%, superando el umbral SLA del 5%."
  ```

### Criterio 5: Documentación Obligatoria
- **ADR Oficial:** Registrado en [docs/adr/ADR-004-observability-opentelemetry-red-metrics.md](docs/adr/ADR-004-observability-opentelemetry-red-metrics.md).
- **Informe Técnico:** Este documento ([docs/INFORME-TECNICO-R3-OBSERVABILIDAD.md](docs/INFORME-TECNICO-R3-OBSERVABILIDAD.md)).

---

## 3. Evidencias de Ejecución y Salidas de Prueba

### Prueba 1: Ejecución de la Suite de Pruebas Automatizadas
Se ejecutaron las pruebas unitarias e integración del paquete `internal/observability`:

```powershell
go test -v ./internal/observability/...
```

**Resultado:**
```text
=== RUN   TestTraceContext_ParseTraceparent
--- PASS: TestTraceContext_ParseTraceparent (0.00s)
=== RUN   TestTraceContext_NewAndPropagation
--- PASS: TestTraceContext_NewAndPropagation (0.00s)
=== RUN   TestREDMetricsRegistry
--- PASS: TestREDMetricsRegistry (0.00s)
=== RUN   TestObservability_MiddlewareIntegration
{"time":"2026-10-18T13:29:55.5203814-05:00","level":"INFO","msg":"HTTP request completed","service.name":"test-service","service.version":"1.0.0","environment":"test","correlation_id":"corr-test-12345","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00f067aa0ba902b7","traceparent":"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01","http.method":"POST","http.route":"/training-environments","http.status_code":201,"http.duration_ms":0,"http.client_ip":"192.0.2.1","http.user_agent":""}
--- PASS: TestObservability_MiddlewareIntegration (0.07s)
PASS
ok      github.com/code-sena/design-software-training-environment-service/internal/observability       0.525s
```

### Prueba 2: Salida en Vivo de la API y Propagación W3C
Se ejecutó la prueba de integración con inyección de cabeceras entrantes desde el frontend:

```powershell
node .\validacion-training-environment\test-r3-observability.mjs
```

**Salida obtenida:**
- **Petición con cabecera entrante:**
  `traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01`
  `x-correlation-id: adso-3145555-emily-amezquita-corr-001`
- **Cabeceras de respuesta devueltas por la API:**
  `traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01`
  `x-correlation-id: adso-3145555-emily-amezquita-corr-001`
  `x-trace-id: 4bf92f3577b34da6a3ce929d0e0e4736`
- **Registro en el log estructurado JSON:**
  El log correlacionó inmediatamente el `correlation_id` y `trace_id` con nivel `WARN` debido a la validación de autenticación 401.

---

## 4. Estructura de Archivos Creados y Modificados

```text
design-software-training-environment-service/
├── cmd/
│   └── training-environment-api/
│       └── main.go                                     # Integración de /metrics y Middleware de Observabilidad
├── internal/
│   └── observability/
│       ├── trace.go                                    # Trazabilidad distribuida W3C Trace Context
│       ├── metrics.go                                  # Registro y cálculo de métricas RED (Prometheus)
│       ├── logger.go                                   # Logger estructurado JSON con correlation_id
│       ├── middleware.go                               # Middleware HTTP unificado
│       └── observability_test.go                       # Suite de pruebas unitarias
├── deploy/
│   ├── docker-compose.observability.yml                # Stack desplegable de Grafana + Prometheus
│   ├── prometheus/
│   │   └── prometheus.yml                              # Configuración de scrapeo para /metrics
│   └── grafana/
│       ├── provisioning/
│       │   ├── datasources/prometheus-datasource.yml   # Datasource Prometheus automático
│       │   └── dashboards/dashboards-provisioning.yml  # Proveedor de tableros desatendido
│       ├── dashboards/
│       │   └── training-environment-red-dashboard.json # Tablero RED oficial con paneles y alertas
│       └── alerts/
│           └── red-alerts.yml                          # Reglas de alerta PromQL (> 5% error)
└── docs/
    ├── adr/
    │   └── ADR-004-observability-opentelemetry-red-metrics.md # Registro de Decisión Arquitectónica
    └── INFORME-TECNICO-R3-OBSERVABILIDAD.md            # Este informe técnico
```

---

## 5. Tabla de Equivalencia Técnica: Go vs Spring Boot

Para facilitar la sustentación técnica ante el comité de evaluación del SENA, se presenta la correspondencia de componentes entre este microservicio y la pila Spring Boot:

| Concepto de la Rúbrica | Implementación en este Microservicio (Go) | Equivalente en Java / Spring Boot |
|---|---|---|
| **Trazabilidad Distribuida** | `internal/observability/trace.go` (W3C Trace Context RFC) | `io.micrometer:micrometer-tracing-bridge-otel` + OpenTelemetry Java Agent |
| **Métricas RED** | `internal/observability/metrics.go` (`http_requests_total`, `http_requests_errors_total`, `http_request_duration_seconds`) | `spring-boot-starter-actuator` + `micrometer-registry-prometheus` (`http.server.requests`) |
| **Endpoint de Métricas** | `GET /metrics` expuesto en HTTP ServeMux | `GET /actuator/prometheus` expuesto en Spring Actuator |
| **Logging Estructurado** | `log/slog` con `JSONHandler` inyectando `correlation_id` y `trace_id` | `net.logstash.logback:logstash-logback-encoder` con `MDC.put("correlation_id", ...)` |
| **Middleware de Correlación** | `Observability.Middleware()` en `internal/observability/middleware.go` | `OncePerRequestFilter` o `HandlerInterceptor` de Spring Web |
| **Tablero y Alerta Grafana** | Provisioning as Code en `deploy/grafana/` con alerta al 5% | Provisioning as Code idéntico en Grafana / Prometheus Alertmanager |

---

## 6. Instrucciones de Reproducción y Verificación

1. **Ejecutar las pruebas unitarias de observabilidad:**
   ```powershell
   cd design-software-training-environment-service
   go test -v ./internal/observability/...
   ```

2. **Compilar el microservicio:**
   ```powershell
   go build -o training-environment-api.exe ./cmd/training-environment-api
   ```

3. **Iniciar el stack de Grafana y Prometheus (opcional con Docker):**
   ```powershell
   docker compose -f deploy/docker-compose.observability.yml up -d
   ```
   - Grafana disponible en `http://localhost:3000` (Usuario: `admin`, Clave: `admin`).
   - Prometheus disponible en `http://localhost:9090`.

4. **Ejecutar prueba en vivo de propagación y métricas:**
   ```powershell
   node .\validacion-training-environment\test-r3-observability.mjs
   ```

---

*Informe técnico elaborado y firmado por la aprendiza responsable:*  
**Emily Sharith Amézquita Saavedra**  
Ficha ADSO 3145555 — SENA  
*Rama Git:* `feature/r3-observability`
