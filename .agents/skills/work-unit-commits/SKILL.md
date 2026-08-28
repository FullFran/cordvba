---
name: work-unit-commits
description: "Estrategia para partir el trabajo de eye en commits revisables (test+impl+docs juntos) y PRs encadenadas cuando el cambio supera 400 líneas. Trigger: al planificar un cambio grande o al preparar commits para revisión."
risk: safe
date_added: "2026-06-23"
---

# Hagalink — Work-Unit Commits (Go)

Skill para estructurar el trabajo en unidades atómicas, revisables y coherentes.

## Cuándo usar

- Al planificar un cambio que afecta más de un archivo o capa.
- Al preparar commits antes de abrir una PR.
- Cuando el cambio estimado supera 400 líneas (o se acerca al límite).
- Al dividir un cambio grande en PRs encadenadas.

## Reglas obligatorias

1. Cada commit es una **unidad de trabajo completa**: test + implementación + docs relevantes, juntos.
2. Nunca commitear implementación sin test, ni test sin implementación (excepto spikes documentados).
3. Cada commit debe pasar el CI de forma independiente (`make ci-local` en verde en cada punto).
4. Usar Conventional Commits: `type(scope): descripción imperativa`.
5. El presupuesto por PR es **400 líneas**. Si se supera, dividir en PRs encadenadas.
6. Las PRs encadenadas se abren de forma secuencial: la siguiente depende del merge de la anterior.

## Anatomía de un buen commit (Go)

```
feat(api): add JWT refresh endpoint

- Add RefreshTokenUseCase with expiry validation (application layer)
- Add POST /auth/refresh route handler (infrastructure layer)
- Add table-driven unit tests for use case (expired, valid, malformed)
- Add integration test for the route using httptest

Closes #42 (parcial — ver PR #78 para la parte de dominio)
```

Un commit de trabajo unitario incluye:
- El test (RED que ahora está GREEN), con `go test -race`
- La implementación mínima que lo pasa
- Actualizaciones de interfaces de dominio o DTOs afectados
- Comentarios GoDoc si el código no es autoexplicativo

## Flujo para cambios grandes (> 400 líneas)

### 1. Planificar slices antes de codear

Dividir el cambio en slices alineados con la arquitectura hexagonal:

```
Slice 1 — Dominio: interfaces, tipos, errores de dominio
Slice 2 — Aplicación: use cases, lógica de negocio
Slice 3 — Infraestructura: clientes HTTP, store SQLite, adaptadores de salida
Slice 4 — Integración: tests de integración, migraciones si aplica
```

Cada slice es una PR. El orden es importante: las capas superiores dependen de las inferiores.

### 2. Crear ramas encadenadas

```bash
# Slice 1 — base: dominio
git checkout -b feat/auth-refresh-domain main

# Slice 2 — sobre slice 1: application
git checkout -b feat/auth-refresh-app feat/auth-refresh-domain

# Slice 3 — sobre slice 2: infra
git checkout -b feat/auth-refresh-infra feat/auth-refresh-app
```

### 3. Abrir PRs en orden

```bash
# PR 1: apunta a main
gh pr create --base main --head feat/auth-refresh-domain \
  --title "feat(api): add auth refresh domain interfaces"

# PR 2: apunta a la rama de la PR 1
gh pr create --base feat/auth-refresh-domain --head feat/auth-refresh-app \
  --title "feat(api): add auth refresh use case"
```

Cuando la PR 1 se mergea a `main`, la PR 2 actualiza su base automáticamente.

### 4. Actualizar base cuando el slice anterior se mergea

```bash
git checkout feat/auth-refresh-app
git rebase main
git push --force-with-lease
```

## Convención de títulos de commit por capa

| Capa | Ejemplo de commit |
|---|---|
| Dominio (interfaces/tipos) | `feat(api): define RefreshTokenRepository interface` |
| Tests primero | `test(api): add failing tests for RefreshTokenUseCase` |
| Application (use case) | `feat(api): implement RefreshTokenUseCase` |
| Infrastructure (handler) | `feat(cli): add `eye events` command` |
| Infrastructure (repo) | `feat(store): implement SQLite RecordStore` |
| Migración | `chore(infra): add refresh_tokens table migration` |
| Docs | `docs(api): document refresh token flow` |

## Verificación antes de abrir la PR

```bash
# Verificar que cada commit compila y pasa tests de forma independiente
git log --oneline origin/main..HEAD   # Ver commits a incluir
make ci-local                          # Puerta de calidad completa
```

## Anti-patrones

- Un solo commit gigante "feat: implement everything".
- Commits de implementación sin tests ("will add tests later").
- PR que mezcla refactor + feature + bugfix en el mismo diff.
- Encadenar PRs sin documentar la dependencia en el body.
- Usar `git push --force` en lugar de `--force-with-lease`.
- Violar capas dentro de un commit (e.g., importar `net/http` en `domain/`).
