# eye — a static, CGO-free binary in a distroless image.
#
# The build context is apps/eye (this component's own root), not the
# monorepo root — the COPY instructions below are relative to it.
#
# From the repository root:
#   docker build -f apps/eye/deploy/Dockerfile -t eye:latest apps/eye
#
# Or, from apps/eye itself:
#   docker build -f deploy/Dockerfile -t eye:latest .
#
# Nothing secret is baked in. The token, the API keys and the data directory
# are all supplied at run time; an image that carried them would be a secret
# published to every registry it is pushed to.

# ---------------------------------------------------------------------------
# Build stage
# ---------------------------------------------------------------------------
FROM golang:1.26-bookworm AS build

WORKDIR /src

# Dependencies first, so an edit to the source does not re-resolve the module
# graph on every build.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

# CGO stays off. It is what keeps this a static binary and lets the final
# stage be distroless — see ADR-0003. The SQLite driver is pure Go.
ENV CGO_ENABLED=0 GOOS=linux

RUN go build -trimpath \
      -ldflags="-s -w \
        -X github.com/FullFran/cordvba/apps/eye/internal/version.Version=${VERSION} \
        -X github.com/FullFran/cordvba/apps/eye/internal/version.Commit=${COMMIT} \
        -X github.com/FullFran/cordvba/apps/eye/internal/version.Date=${DATE}" \
      -o /out/eye ./cmd/eye

# The state directory has to exist in the image and be owned by the user that
# runs. Distroless has no shell, so it cannot be created in the final stage,
# and a directory Docker invents for a volume is owned by root — which a
# non-root process cannot write to. Making it here is the fix.
RUN mkdir -p /out/state

# ---------------------------------------------------------------------------
# Runtime stage
# ---------------------------------------------------------------------------
# Distroless: no shell, no package manager, nothing to pivot to. eye needs
# none of it — it is one static binary reading public HTTP.
FROM gcr.io/distroless/static-debian12:nonroot

# The registry is data, not code: it is what eye is allowed to read, and an
# operator can replace it with --registry without rebuilding.
COPY --from=build /src/configs/sources.yaml /etc/eye/sources.yaml
COPY --from=build /out/eye /usr/local/bin/eye

# 65532 is the uid of "nonroot" in distroless. Numeric on purpose: the name is
# resolved against the target image, and a numeric id cannot be misresolved.
COPY --from=build --chown=65532:65532 /out/state /var/lib/eye

# nonroot is uid 65532 in this image. eye needs no privileges and must never
# ask for any, so it runs as a user that has none.
USER nonroot:nonroot

ENV EYE_DATA_DIR=/var/lib/eye \
    EYE_CONFIG_DIR=/etc/eye \
    EYE_LOG_LEVEL=info

VOLUME ["/var/lib/eye"]
EXPOSE 8787

# Distroless has no shell and no curl, so eye probes itself: `serve --probe`
# performs one request and exits. It hits /health, which is open by design —
# a health check that needs a credential reports the deployment down the
# moment the token rotates.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/usr/local/bin/eye", "serve", "--probe", "http://127.0.0.1:8787/health"]

ENTRYPOINT ["/usr/local/bin/eye"]
CMD ["serve", "--public", "--addr", "0.0.0.0:8787"]
