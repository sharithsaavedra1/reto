# ADR-003 — Modelo temporal de mantenimientos

## Estado

Aprobado para HU-ENV-018.

## Contexto

`training_environment.maintenance` almacenaba únicamente `start_date` y `end_date` y mantenía la restricción `end_date > start_date`. Ese modelo no permite representar un mantenimiento válido dentro del mismo día ni aplicar la regla de negocio que compara `start_time` y `end_time` cuando ambas fechas son iguales.

HU-ENV-018 exige mantener como objeto oficial `training_environment.maintenance`, aceptar mantenimientos del mismo día cuando la hora final sea posterior, y aceptar rangos de varios días aunque la hora final sea menor que la inicial.

## Decisión

Se mantiene la tabla oficial `training_environment.maintenance` y se incorporan las columnas:

- `start_time TIME NOT NULL DEFAULT '00:00:00'`
- `end_time TIME NOT NULL DEFAULT '00:00:00'`

El intervalo se interpreta con los límites completos de fecha y hora. Se aplican las siguientes reglas:

```text
end_date >= start_date
```

```text
si start_date = end_date, entonces end_time > start_time
```

Cuando `start_date < end_date`, no se compara el orden de las horas. Por ejemplo, `2026-10-05 22:00` hasta `2026-10-06 06:00` es válido.

La restricción histórica `chk_maintenance_end_date` se elimina y se reemplaza por:

- `chk_maintenance_date_range`
- `chk_maintenance_same_day_time_range`

El servicio recibe y devuelve `start_time` y `end_time`, normalizados como `HH:MM:SS`. Las violaciones temporales se traducen a un error de validación controlado y la API responde `422`.

No se introduce una tabla alternativa; el objeto oficial continúa siendo `training_environment.maintenance`.

## Compatibilidad de datos existentes

Los registros históricos no tienen horas. Los defaults `00:00:00` permiten migrarlos sin modificar los changesets DML ya aplicados y preservan la semántica anterior para rangos de varios días.

No se modifica ningún changeset histórico para evitar cambios de checksum de Liquibase.

## Rollback

Antes de retirar las columnas horarias, los mantenimientos creados para un solo día se amplían hasta el día siguiente. Después se eliminan los checks nuevos y las columnas `start_time`/`end_time`, y se restaura `chk_maintenance_end_date` con `end_date > start_date`.

El rollback conserva las filas. Al regresar al modelo anterior se pierde, por definición, la precisión horaria de los mantenimientos creados con HU-ENV-018.

## Consecuencias

- Base de datos y servicio usan el mismo modelo temporal.
- Se pueden registrar mantenimientos válidos dentro del mismo día.
- Los rangos de varios días admiten una hora final menor que la inicial.
- Los eventos `environment.maintenance.started` incluyen `start_time` y `end_time` de forma aditiva.
