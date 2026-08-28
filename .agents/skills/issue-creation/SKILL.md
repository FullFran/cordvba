---
name: issue-creation
description: "Workflow issue-first para crear issues útiles y aprobables en eye (bug/feature) usando labels obligatorias y flujo de aprobación. Trigger: cuando el usuario quiere crear un issue, reportar un bug o solicitar una feature."
risk: safe
date_added: "2026-06-23"
---

# Hagalink — Issue Creation (issue-first)

Skill operativa para asegurar que todo cambio empiece por un issue aprobado.

## Cuándo usar

- Cuando se crea un issue nuevo (bug o feature).
- Cuando se guía a alguien a reportar un problema.
- Cuando se hace triage previo a la implementación.

## Reglas obligatorias

1. Buscar duplicados antes de crear: `gh issue list --search "palabras-clave"`.
2. El issue nace con `status:needs-review` — no habilita implementación todavía.
3. Solo se implementa cuando el issue tenga `status:approved`.
4. Usar formato de título: `type(scope): descripción` (Conventional Commits).
5. Añadir siempre label de tipo (`type:feat`, `type:fix`, etc.) y `status:needs-review`.
6. No abrir rama ni PR hasta que el issue tenga `status:approved`.

## Scopes del proyecto

`api` | `infra` | `docs` | `skills` | `deps` | `ci` | `build` | `audit` | `governance` | `lint` | `release`

## Flujo

1. Buscar duplicados.
2. Crear issue con título claro y descripción completa.
3. Añadir labels: `status:needs-review` + tipo (`type:feat` / `type:fix` / `type:refactor` / `type:docs` / `type:chore`).
4. Esperar aprobación (`status:approved`).
5. Solo entonces: crear rama y PR.

## Comandos útiles

```bash
# Buscar duplicados
gh issue list --search "palabra-clave" --repo eye

# Crear issue
gh issue create \
  --title "feat(api): descripción corta" \
  --label "type:feat,status:needs-review" \
  --body "$(cat <<'EOF'
## Descripción

Explica qué problema resuelve este issue y por qué es necesario.

## Criterios de aceptación

- [ ] AC-1: ...
- [ ] AC-2: ...

## Scope

**IN**: qué entra exactamente.
**OUT**: qué no entra.
EOF
)"

# Ver issue
gh issue view <numero>

# Aprobar (cuando está listo para implementar)
gh issue edit <numero> --add-label "status:approved" --remove-label "status:needs-review"
```

## Labels del proyecto

| Label | Uso |
|---|---|
| `type:feat` | Nueva funcionalidad |
| `type:fix` | Comportamiento incorrecto |
| `type:refactor` | Refactor sin cambio de comportamiento |
| `type:docs` | Documentación |
| `type:chore` | Mantenimiento, deps, CI |
| `status:needs-review` | Issue pendiente de aprobación |
| `status:approved` | Aprobado para implementar |
| `priority:high` | Urgente |
| `priority:medium` | Normal |
| `priority:low` | Cuando haya tiempo |

## Anti-patrones

- Abrir una PR sin issue previo.
- Implementar con `status:needs-review` (sin aprobación).
- Títulos ambiguos como "fix stuff" o "update handler".
- Issue sin criterios de aceptación verificables.
