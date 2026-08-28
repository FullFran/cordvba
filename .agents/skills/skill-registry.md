# Skill Registry — eye (Go backend)

Registro canónico de skills de agente para este proyecto.
Los sub-agentes reciben las reglas de "Compact Rules" pre-digeridas — no leen este archivo directamente.

## User Skills

| Skill | Trigger | Cuándo usar |
|---|---|---|
| `issue-creation` | Al crear un issue, reportar un bug o solicitar una feature | Asegura issue-first: búsqueda de duplicados, labels obligatorias, flujo de aprobación |
| `branch-pr` | Al crear, abrir o preparar una PR para revisión | Enforcement de issue aprobado, label type:*, puerta make ci-local, presupuesto 400 líneas |
| `pr-review` | Al revisar una PR o cuando el usuario pide code review | Checklist completo: intención, tamaño, CI, arquitectura hexagonal, TDD, seguridad, docs |
| `work-unit-commits` | Al planificar un cambio grande o preparar commits para revisión | Commits atómicos (test+impl+docs), PRs encadenadas por capa hexagonal cuando supera 400 líneas |
| `release-flow` | Al preparar un release, publicar una versión o revisar el flujo de versionado | release-please para gestión automática de tags y GitHub Releases |

## Compact Rules

Reglas comprimidas para inyectar a sub-agentes. Copiar el bloque relevante en el prompt del sub-agente.

---

### issue-first

Todo cambio empieza por un issue aprobado.

```
ISSUE-FIRST ENFORCEMENT:
- Buscar duplicados: gh issue list --search "keywords"
- Issue nace con status:needs-review. No implementar hasta status:approved.
- Título del issue: type(scope): descripción (Conventional Commits)
- Labels obligatorias: type:* + status:needs-review
- Solo crear rama cuando el issue tiene status:approved
```

---

### conventional-commits + scope-enum

```
CONVENTIONAL COMMITS (obligatorio):
Formato: type(scope): descripción imperativa, lowercase, sin punto final, max 100 chars

Tipos permitidos: feat | fix | refactor | test | docs | chore | perf | ci | build

Scope-enum (Go backend):
  api | infra | docs | skills | deps | ci | build | audit | governance | lint | release

Ejemplos:
  feat(api): add JWT refresh endpoint
  fix(api): return 422 for missing required fields
  chore(deps): upgrade modernc.org/sqlite
  docs(governance): update PR budget rule
```

---

### ci-local gate (Go)

```
PUERTA DE CALIDAD PRE-PR (obligatoria):
  make ci-local

Equivale a: gofmt + go vet + golangci-lint run ./... + go test -race -cover ./... + go build.
Adjuntar el output en el body de la PR antes de pedir review.
No abrir PR sin pasar esta puerta.
```

---

### 400-line budget

```
PRESUPUESTO DE PR: 400 LÍNEAS MÁXIMO
- Contar líneas cambiadas excluyendo archivos generados.
- Si supera 400 líneas: dividir en PRs encadenadas (slices por capa hexagonal).
- Si se requiere excepción: añadir label size:exception con justificación en el body.
- Orden de slices: dominio (interfaces/tipos) → aplicación (use cases) → infraestructura (handlers/repos) → integración.
- Cada slice es una PR independiente; la siguiente apunta a la rama de la anterior.
```

---

### strict-tdd (Go)

```
STRICT TDD (non-negotiable):
  RED → GREEN → REFACTOR

1. Escribir el test que falla primero. Debe fallar por la razón correcta.
2. Escribir la implementación mínima que lo pasa. Nada más.
3. Refactorizar manteniendo el test en verde.

Runner: go test -race -cover ./...
Estructura: internal/<domain>/application/service_test.go + infrastructure/handler_test.go
Cada commit de trabajo unitario incluye: test + implementación + GoDoc si es necesario.
No existe "añadir tests después". Si el test no existe antes de la implementación, es inválido.
```

---

### hexagonal-layers

```
ARQUITECTURA HEXAGONAL — reglas de capa (non-negotiable):
- domain/: entidades, interfaces, errores de dominio. CERO imports de net/http ni drivers.
- application/: use cases. Depende SOLO de interfaces de domain/.
- infrastructure/: clientes HTTP, store SQLite, salida por terminal. Implementa interfaces de domain/.
- cmd/eye/main.go: composition root. Único punto de wiring.
- internal/config/config.go: único lugar donde se puede llamar os.Getenv.
- Todos los errores: fmt.Errorf("context: %w", err).
- Context propagation obligatorio en toda función I/O.
```

---

### pr-governance

```
PR GOVERNANCE — checklist mínimo antes de pedir review:
- [ ] Issue referenciado con status:approved (Closes #N)
- [ ] Título sigue Conventional Commits con scope válido
- [ ] Exactamente UNA label type:*
- [ ] make ci-local pasa — evidencia en el body
- [ ] Diff < 400 líneas (o size:exception justificada)
- [ ] Tests cubren los criterios de aceptación del issue
- [ ] Docs/.agents actualizados si se tomó una decisión de arquitectura
```
