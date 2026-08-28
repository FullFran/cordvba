---
name: pr-review
description: "Checklist de revisión de PR para eye: intención, tamaño, CI, arquitectura hexagonal, TDD, seguridad y documentación. Trigger: al revisar una PR o cuando el usuario pide code review."
risk: safe
date_added: "2026-06-23"
---

# Hagalink — PR Review Checklist (Go)

Skill para hacer revisiones de PR consistentes, accionables y con criterio técnico real.

## Cuándo usar

- Al revisar una PR de cualquier colaborador.
- Al auto-revisar una PR antes de pedir review.
- Al validar que una PR está lista para merge.

## Reglas obligatorias

1. Leer el issue referenciado antes de revisar el código.
2. Verificar que el CI está en verde antes de hacer cualquier comentario de código.
3. Emitir un veredicto explícito al final: **APPROVE / REQUEST CHANGES / COMMENT**.
4. Todo feedback debe ser accionable — nunca vago.

## Flujo de revisión

### 1. Intención y contexto

- [ ] ¿La PR referencia un issue con `status:approved`?
- [ ] ¿El título sigue Conventional Commits (`type(scope): descripción`)?
- [ ] ¿El body explica el "por qué", no solo el "qué"?
- [ ] ¿El scope del cambio coincide con el issue? (ni más ni menos)

### 2. Tamaño y presupuesto

- [ ] ¿El diff es menor de 400 líneas (excluyendo archivos generados)?
- [ ] Si supera 400 líneas, ¿tiene la label `size:exception` con justificación?
- [ ] ¿Los commits son atómicos y revisables por separado?

### 3. CI y calidad

- [ ] ¿Todos los checks de CI están en verde?
- [ ] ¿Hay evidencia de `make ci-local` en el body de la PR?
- [ ] ¿`golangci-lint` pasa sin advertencias nuevas?
- [ ] ¿`go vet` pasa limpio?

### 4. Arquitectura hexagonal y capas

- [ ] ¿El cambio respeta las capas (`domain` / `application` / `infrastructure`)?
- [ ] ¿El paquete `domain` importa algún framework externo (`net/http`, storage drivers)? → debe ser `REQUEST CHANGES`.
- [ ] ¿Los contratos (interfaces de dominio, DTOs, errores) están bien definidos?
- [ ] ¿Se usa `os.Getenv` fuera de `internal/config/config.go`? → debe ser `REQUEST CHANGES`.
- [ ] ¿Se introduce deuda técnica no documentada?

### 5. TDD y cobertura

- [ ] ¿Los tests se escribieron ANTES de la implementación (RED → GREEN → REFACTOR)?
- [ ] ¿Hay tests para cada criterio de aceptación del issue?
- [ ] ¿Los tests usan el race detector (`go test -race`)?
- [ ] ¿Los casos límite y escenarios de error están cubiertos?
- [ ] ¿Los errores están correctamente envueltos con `fmt.Errorf("context: %w", err)`?

### 6. Seguridad

- [ ] ¿Hay secretos o valores sensibles en el diff? (revisar `.env`, tokens, claves)
- [ ] ¿Las entradas del usuario están validadas antes de procesarse?
- [ ] ¿Las rutas nuevas tienen autenticación y autorización correctas?
- [ ] ¿Hay consultas SQL sin parametrizar?
- [ ] ¿Se propaga el `context.Context` en todas las funciones I/O?

### 7. Documentación y gobernanza

- [ ] ¿Se actualizó la documentación relevante (README, comentarios GoDoc, ADR)?
- [ ] ¿Los archivos `.agents/` reflejan cualquier convención nueva?

## Comandos útiles

```bash
# Ver el diff completo de la PR
gh pr diff <numero>

# Ver checks de CI
gh pr checks <numero>

# Ver detalles del issue referenciado
gh issue view <numero-issue> --json title,body,labels

# Aprobar PR
gh pr review <numero> --approve --body "LGTM. [razón breve]"

# Solicitar cambios
gh pr review <numero> --request-changes --body "Ver comentarios inline."

# Comentar sin veredicto
gh pr review <numero> --comment --body "Preguntas/sugerencias sin bloquear."
```

## Formato del veredicto

Al terminar la revisión, emitir uno de los tres veredictos con justificación breve:

```
APPROVE — El cambio es correcto, los tests cubren los AC del issue,
el CI está verde y la arquitectura hexagonal está respetada.

REQUEST CHANGES — [Razones concretas y accionables]
  - [Hallazgo 1]: [Qué cambiar y por qué]
  - [Hallazgo 2]: [Qué cambiar y por qué]

COMMENT — [Preguntas o sugerencias que no bloquean el merge]
```

## Anti-patrones de review

- Aprobar sin haber leído el issue referenciado.
- Comentar "looks good" sin verificar los tests.
- Feedback vago: "esto podría ser mejor" sin decir cómo.
- Ignorar violaciones de capas (domain importando net/http o drivers de almacenamiento).
- No verificar el CI antes de aprobar.
- Bloquear por estilo cuando golangci-lint ya lo cubre.
