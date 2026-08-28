---
name: release-flow
description: "Flujo de release para eye con release-please: Conventional Commits → PR de release automática → GitHub Release + tag. Trigger: al preparar un release, publicar una versión o revisar el flujo de versionado."
risk: safe
date_added: "2026-06-23"
---

# Hagalink — Release Flow (Go)

Skill para gestionar releases de servicios Go con release-please.

## Cuándo usar

- Al preparar la publicación de una versión del servicio.
- Al revisar qué cambios están pendientes de release.
- Al hacer troubleshooting de una PR de release bloqueada.

## Reglas obligatorias

1. Nunca crear tags ni editar versiones manualmente — release-please gestiona todo.
2. Usar Conventional Commits (`feat:`, `fix:`, `chore:`, etc.) para que release-please pueda calcular el bump de versión.
3. El flujo de release es: merge a `main` → release-please crea/actualiza la PR de release → merge de la PR → tag + GitHub Release automáticos.
4. No publicar binarios manualmente desde local — el CI gestiona el build y la release.

## Flujo de release

### Paso 1 — Durante el desarrollo

Usar Conventional Commits en todos los commits que llegan a `main`:

```
feat(api): add JWT refresh endpoint       → bump MINOR
fix(api): handle nil pointer in health    → bump PATCH
feat!: change auth header format          → bump MAJOR (breaking change)
chore(deps): upgrade modernc.org/sqlite            → sin bump de release
docs(api): update OpenAPI spec            → sin bump de release
```

### Paso 2 — release-please crea la PR de release

Cuando se mergea un commit con `feat:` o `fix:` a `main`, release-please crea o actualiza
automáticamente una PR llamada **"chore(release): vX.Y.Z"** que:
- Actualiza la versión en el código (si aplica).
- Genera o actualiza `CHANGELOG.md` con los cambios desde el último release.
- Acumula múltiples cambios en un solo bump.

```bash
# Ver la PR de release
gh pr list --search "chore(release)"
```

### Paso 3 — Revisar y mergear la PR de release

Revisar el `CHANGELOG.md` generado. Si todo es correcto:

```bash
gh pr view <numero-release-pr>
gh pr merge <numero-release-pr> --squash
```

### Paso 4 — Tag y GitHub Release automáticos

Al mergear la PR de release, release-please:
- Crea el tag `vX.Y.Z` en el repositorio.
- Publica un GitHub Release con las notas de `CHANGELOG.md`.
- El CI cross-compila los binarios y los adjunta a la release.

```bash
# Verificar el tag creado
gh release view vX.Y.Z

# Ver todas las releases
gh release list
```

## Configuración de release-please (ya incluida en la template)

```yaml
# .github/workflows/release.yml
# release-please se activa en cada push a main y gestiona el ciclo completo.
```

## Comandos de diagnóstico

```bash
# Ver el estado del repositorio
gh release list

# Ver el último tag
git describe --tags --abbrev=0

# Ver commits desde el último release
git log $(git describe --tags --abbrev=0)..HEAD --oneline

# Ver la PR de release activa
gh pr list --search "chore(release)" --state open
```

## Anti-patrones

- Crear tags manualmente con `git tag`.
- Editar `CHANGELOG.md` manualmente.
- Mergear la PR de release sin revisar los cambios listados.
- Usar mensajes de commit que no siguen Conventional Commits (release-please no los detecta).
- Publicar binarios a mano desde local.
