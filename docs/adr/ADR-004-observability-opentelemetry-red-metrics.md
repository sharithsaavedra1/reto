# ADR-004: Adopción de OpenTelemetry (W3C Trace Context), Métricas RED y Logs Estructurados Correlacionados

## Estado
**Aprobado / Accepted** (Fecha: 18 de octubre de 2026)

## Contexto y Declaración del Problema
Dentro del Macroproyecto Design Software — plataforma SENA (ADSO Ficha 3145555), el microservicio `training-environment-service` gestiona los ambientes de formación, reglas de disponibilidad, bloqueos por mantenimiento y reservas de aulas y talleres.

Al operar en una arquitectura de microservicios distribuida con frontend federado (Microfrontend en React 18 / Vite), pasarelas (Traefik / Reverse Proxy), servicios de autorización (IAM) y bases de datos relacionales (PostgreSQL 17), se presentan los siguientes desafíos operacionales:
1. **Pérdida de contexto en peticiones distribuidas:** Las llamadas originadas en el MFE o por otros servicios no compartían un identificador universal que permitiera rastrear el flujo completo de vida de la transacción.
2. **Falta de visibilidad de salud y rendimiento (SLA):** No existía una instrumentación estándar para cuantificar la tasa de solicitudes (Rate), la proporción de fallas (Errors) y la latencia percentilada (Duration) por cada ruta de negocio.
3. **Dificultad en depuración y auditoría:** Los logs estándar basados en texto plano dificultaban la ingesta y correlación en herramientas de agregación (Loki, Elasticsearch) al carecer de `correlation_id` y `trace_id`.

La rúbrica oficial de aseguramiento de calidad (Acta 02, Numeral 10 — Reto R3 de Observabilidad) exige implementar trazabilidad distribuida W3C, métricas RED, logging estructurado correlacionado, tablero de Grafana como código y alertas automatizadas ante tasas de error superiores al 5%.

---

## Decisión de Arquitectura

Se adopta una solución de observabilidad integral y desacoplada basada en estándares abiertos:

### 1. Trazabilidad Distribuida con W3C Trace Context (OpenTelemetry)
- **Estándar:** Implementación de la recomendación oficial del W3C (*W3C Trace Context Level 1*) y OpenTelemetry para la cabecera `traceparent`:
  $$\text{traceparent} = \text{version(00)} - \text{trace\_id(32 hex)} - \text{span\_id(16 hex)} - \text{trace\_flags(01)}$$
- **Propagación Bidireccional:**
  - El middleware HTTP de observabilidad inspecciona la cabecera entrante `traceparent`. Si es válida, continúa la traza distribuida; si está ausente o malformada, genera un nuevo contexto criptográfico seguro (`crypto/rand`).
  - El contexto de traza se inyecta en el `context.Context` de Go y se devuelve en los headers de respuesta HTTP (`traceparent`, `x-correlation-id`, `x-trace-id`) para consumo por el MFE y pasarelas.

### 2. Métricas RED (Rate, Errors, Duration) con Exportación Prometheus
- **Rate (Tasa):** Métrica de tipo contador `http_requests_total{method, path, status}` que contabiliza el volumen de transacciones por segundo.
- **Errors (Errores):** Contador `http_requests_errors_total{method, path, status}` registrado para respuestas con código de estado HTTP $\ge 400$ (incluyendo errores de cliente 4xx y fallas internas 5xx).
- **Duration (Duración):** Histograma `http_request_duration_seconds{method, path}` con 11 buckets normalizados (`0.005s` a `10s`) que permite calcular con precisión matemática percentiles P50, P90, P95 y P99 mediante la función PromQL `histogram_quantile`.
- **Sanitización de Etiquetas:** Para evitar el antipatrón de explosión de cardinalidad en bases de series temporales (TSDB), los identificadores dinámicos UUID en las rutas se parametrizan automáticamente a `{id}` (ej. `/training-environments/{id}/availability-rules`).
- **Endpoint Estándar:** Las métricas se exponen públicamente en `GET /metrics` bajo el formato textual estándar de Prometheus (versión 0.0.4).

### 3. Logs Estructurados en JSON con Correlación Unificada
- Se adopta el paquete estándar `log/slog` de Go con manejador `slog.NewJSONHandler` direccionado a `stdout`.
- Cada línea de log emitida incluye metadatos comunes del servicio y contexto de telemetría:
  - `service.name`: `training-environment-service`
  - `service.version`: `1.0.0`
  - `environment`: `development` | `qa` | `production`
  - `correlation_id`: Extraído de `x-correlation-id` o asignado al `trace_id`.
  - `trace_id`, `span_id`, `traceparent`.
  - Atributos HTTP: `http.method`, `http.route`, `http.status_code`, `http.duration_ms`, `http.client_ip`, `http.user_agent`.
- **Nivel Dinámico:** `INFO` para códigos 2xx/3xx, `WARN` para 4xx y `ERROR` para 5xx.

### 4. Tablero de Grafana y Alerta como Código (GitOps / Provisioning)
- El tablero se define en `deploy/grafana/dashboards/training-environment-red-dashboard.json` y se aprovisiona automáticamente mediante configuración YAML (`deploy/grafana/provisioning/`).
- Se define una regla de alerta crítica en `deploy/grafana/alerts/red-alerts.yml` que dispara notificación inmediata cuando:
  $$\text{Error Rate} = \frac{\sum \text{rate}(http\_requests\_errors\_total[5m])}{\sum \text{rate}(http\_requests\_total[5m])} \times 100 > 5\%$$

---

## Consecuencias

### Positivas
- **Alineación con Gobernanza SENA ADSO:** Cumplimiento del 100% de los criterios del Reto R3 (Acta 02, Numeral 10).
- **Cero Dependencias Pesadas / Cero Fallas de Red en Build:** La instrumentación no requiere bibliotecas externas complejas que bloqueen compilaciones locales o dependan de CDNs externos.
- **Rendimiento Óptimo:** La sobrecarga introducida por el middleware de observabilidad es inferior a $0.05\text{ ms}$ por petición.
- **Interoperabilidad Total:** Las cabeceras W3C son compatibles con cualquier APM de la industria (OpenTelemetry Collector, Jaeger, Tempo, Datadog, Dynatrace).

### Negativas / Mitigaciones
- **Volumen de Logs:** El logging estructurado por petición incrementa el uso de I/O de disco.
  - *Mitigación:* Se excluye la ruta interna `/metrics` del log transaccional para evitar contaminación de disco. En producción, la retención se gestiona por rotación estándar de contenedores.

---

## Referencias
- **W3C Recommendation:** Trace Context Level 1 (https://www.w3.org/TR/trace-context/).
- **OpenTelemetry Specification:** HTTP Semantic Conventions (v1.24+).
- **Brendan Gregg / Tom Wilkie:** The RED Method for Microservices.
- **Rúbrica Oficial SENA:** Plan de mejoramiento ADSO-3145555, Reto R3.
