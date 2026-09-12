# ADR-002: Estrategia de Validación de `training_center_id` (Validación de Formato Síncrona y Existencia Diferida)

## Estado
**Aprobado / Accepted**

## Contexto y Declaración del Problema
En el microservicio `training-environment-service` (HU-ENV-002: Registro de ambientes de formación), cada ambiente físico registrado debe pertenecer a un centro de formación identificado mediante el atributo `training_center_id`.

El catálogo y ciclo de vida de los centros de formación pertenece a un **Bounded Context externo** (`training-center-service` o microservicio de sedes/centros), no gestionado directamente dentro de la base de datos de ambientes de formación.

El Definition of Ready (DoR) de la HU exige definir explícitamente la estrategia de validación para `training_center_id`: **Validación Síncrona (RPC/HTTP directo) vs Validación Diferida / Asíncrona (Consistencia Eventual / Saga / EDA)**.

---

## Decisión de Arquitectura

Se adopta una estrategia **híbrida desacoplada**:

### 1. Validación de Formato Síncrona en Ingesta (Edge / Domain Validation)
- El microservicio valida de forma síncrona y obligatoria que `training_center_id` cumpla estrictamente con la sintaxis **UUID RFC 4122** (36 caracteres hexadecimales con formato `8-4-4-4-12`).
- Cualquier solicitud con un identificador vacío, nulo o malformado (por ejemplo, cadenas de longitud arbitraria o caracteres no hexadecimales como `"xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"`) es rechazada inmediatamente con **HTTP `422 Unprocessable Entity`** y envelope de error corporativo.

### 2. Validación de Existencia Diferida / Desacoplada (Eventual Consistency & Bounded Contexts)
- **Desacoplamiento Temporal:** No se realiza una llamada HTTP/REST síncrona bloqueante hacia el microservicio de centros durante el `POST /training-environments`.
- **Justificación Técnica:**
  - Evitar el patrón antipatrón de *Distributed Monolith* y llamadas en cascada (cascading failures) donde una indisponibilidad o latencia en la red de centros bloquee el alta de ambientes físicos.
  - Mantener latencias sub-10ms en la API de ambientes de formación.
- **Mecanismo de Integración Inter-Servicio:** La consistencia referencial y la verificación de existencia del centro se delegan a la arquitectura orientada a eventos (EDA) / Eventual Consistency (ej. eventos de dominio `TrainingCenterCreated`, `TrainingCenterDeactivated` o sincronización por outbox/CDC).

---

## Consecuencias

### Positivas
- **Alta Disponibilidad y Resiliencia:** El microservicio de ambientes opera de forma autónoma sin depender de la disponibilidad en tiempo real del servicio de centros.
- **Rendimiento:** Cero sobrecarga de red síncrona en la ruta crítica del `POST /training-environments`.
- **Alineación con Gobernanza:** Cumple con los estándares de microservicios desacoplados y principios DDD (Domain-Driven Design).

### Negativas / Mitigaciones
- **Mitigación:** Si un centro es eliminado o inexistente en el sistema central, el proceso asíncrono o la vista materializada alertará o deshabilitará el ambiente (`is_active = false`) mediante eventos de compensación.

---

## Referencias
- **Historia de Usuario:** HU-ENV-002 (#153) — Record of training environments.
- **Reglas de Negocio:** RF-ENV-01, RN-ENV-01.
- **Estándar:** RFC 4122 (UUID Specifications).
