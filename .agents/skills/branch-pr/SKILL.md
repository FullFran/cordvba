---
name: branch-pr
description: "Workflow para abrir PRs en eye con enforcement de issue aprobado, label type:*, puerta de calidad make ci-local y presupuesto de 400 líneas. Trigger: al crear, abrir o preparar una PR para revisión."
risk: safe
date_added: "2026-06-23"
---

# Hagalink — Branch & PR Enforcement (Go)

Skill para abrir PRs consistentes con la gobernanza issue-first de eye.

## Cuándo usar

- Al abrir una PR.
- Al preparar una rama para revisión.
- Al verificar si una PR cumple la gobernanza mínima.

## Reglas obligatorias

1. Toda PR debe referenciar un issue aprobado (`Closes #N` o `Fixes #N`).
2. El issue referenciado DEBE tener `status:approved` antes de abrir la rama.
3. La PR debe tener exactamente UNA label `type:*`.
4. Antes de pedir review, ejecutar `make ci-local` y adjuntar evidencia en el body.
5. Usar Conventional Commits en todos los commits de la rama.
6. El presupuesto máximo es **400 líneas** de cambio (excluyendo archivos generados). Si se supera, dividir en PRs encadenadas.

## Puerta de calidad pre-PR (obligatoria)

```bash
make ci-local
```

Equivale a: `gofmt` + `go vet` + `golangci-lint run ./...` + `go test -race -cover ./...` + `go build`. Adjuntar el output en el body de la PR.

## Flujo completo

1. Verificar que el issue tiene `status:approved`:
   ```bash
   gh issue view <N> --json labels
   ```
2. Crear rama: `git checkout -b type/descripcion-corta`
   (ejemplos: `feat/auth-refresh`, `fix/health-handler-nil`)
3. Implementar siguiendo TDD: RED → GREEN → REFACTOR.
4. Commitear con Conventional Commits.
5. Correr la puerta de calidad:
   ```bash
   make ci-local
   ```
6. Abrir PR:
   ```bash
   gh pr create \
     --title "feat(api): descripción corta" \
     --body "$(cat <<'EOF'
   ## ¿Por qué?

   Contexto: qué problema resuelve, motivación.

   ## ¿Qué cambia?

   Resumen técnico de los cambios.

   ## ¿Cómo lo pruebo?

   - [ ] `make ci-local` pasa
   - [ ] Pruebas manuales: <describe>

   ## Riesgo y blast radius

   ¿Qué puede romper? ¿Hay migraciones?

   ## Checklist

   - [ ] Tests añadidos o actualizados (TDD: RED → GREEN → REFACTOR)
   - [ ] Docs actualizadas si aplica
   - [ ] Conventional Commit en el título
   - [ ] PR < 400 líneas (o etiqueta `size:exception` justificada)

   Closes #<numero>
   EOF
   )"
   ```
7. Añadir exactamente UNA label `type:*`:
   ```bash
   gh pr edit <numero-pr> --add-label "type:feat"
   ```

## Tipos de rama y labels

| Tipo de cambio | Prefijo de rama | Label PR |
|---|---|---|
| Nueva feature | `feat/` | `type:feat` |
| Bug fix | `fix/` | `type:fix` |
| Refactor | `refactor/` | `type:refactor` |
| Documentación | `docs/` | `type:docs` |
| Mantenimiento | `chore/` | `type:chore` |

## PRs encadenadas (cuando supera 400 líneas)

Si el trabajo estimado supera 400 líneas:

1. Dividir en slices independientes y ordenados por dependencia.
2. La primera PR sienta la base (interfaces de dominio, contratos).
3. Cada PR siguiente depende de que la anterior esté mergeada.
4. Usar `git checkout -b feat/desc-parte-2 feat/desc-parte-1` para derivar.

## Anti-patrones

- PR sin referencia a issue (`Closes #N`).
- PR con issue que no tiene `status:approved`.
- PR con múltiples labels `type:*`.
- PR sin evidencia de `make ci-local`.
- Commits con mensajes como "fix stuff" o "wip".
- PR que supera 400 líneas sin etiqueta `size:exception` y justificación.
