# Build context is the repository root, so the uv workspace (eye-client +
# api) resolves. Build with:
#   docker build -f apps/api/Dockerfile -t cordvba-api .
FROM python:3.12-slim AS base

RUN groupadd --system api && useradd --system --gid api --create-home api

COPY --from=ghcr.io/astral-sh/uv:0.8.15 /uv /uvx /usr/local/bin/

WORKDIR /app

# Only the files uv needs to resolve the workspace and this member's deps,
# so dependency layers stay cached across unrelated source changes. uv
# resolves the whole workspace declared in the root pyproject.toml, so
# every member's pyproject.toml must be present even though only
# eye-client and api are actually installed below.
COPY pyproject.toml uv.lock ./
COPY packages/eye-client/pyproject.toml packages/eye-client/pyproject.toml
COPY services/twin/pyproject.toml services/twin/pyproject.toml
COPY apps/api/pyproject.toml apps/api/pyproject.toml

RUN uv sync --locked --no-dev --package api --no-install-workspace

COPY packages/eye-client packages/eye-client
COPY apps/api apps/api

RUN uv sync --locked --no-dev --package api && chown -R api:api /app

USER api

EXPOSE 8000

# Run the venv's own uvicorn directly (not `uv run`): the runtime user has
# no write access to re-sync, and none is needed since the image is built.
CMD ["/app/.venv/bin/uvicorn", "api.main:app", "--host", "0.0.0.0", "--port", "8000"]
