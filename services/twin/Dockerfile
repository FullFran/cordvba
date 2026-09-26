# Build context is the repository root, so the uv workspace (eye-client +
# twin) resolves. Build with:
#   docker build -f services/twin/Dockerfile -t cordvba-twin .
FROM python:3.12-slim AS base

RUN groupadd --system twin && useradd --system --gid twin --create-home twin

COPY --from=ghcr.io/astral-sh/uv:0.8.15 /uv /uvx /usr/local/bin/

WORKDIR /app

# Only the files uv needs to resolve the workspace and this member's deps,
# so dependency layers stay cached across unrelated source changes.
COPY pyproject.toml uv.lock ./
COPY packages/eye-client/pyproject.toml packages/eye-client/pyproject.toml
COPY services/twin/pyproject.toml services/twin/pyproject.toml

RUN uv sync --locked --no-dev --package twin --no-install-workspace

COPY packages/eye-client packages/eye-client
COPY services/twin services/twin

RUN uv sync --locked --no-dev --package twin && chown -R twin:twin /app

USER twin

EXPOSE 8100

# Run the venv's own uvicorn directly (not `uv run`): the runtime user has
# no write access to re-sync, and none is needed since the image is built.
CMD ["/app/.venv/bin/uvicorn", "twin.main:app", "--host", "0.0.0.0", "--port", "8100"]
