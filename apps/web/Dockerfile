# syntax=docker/dockerfile:1
#
# Build from the monorepo root so the contract fixtures
# (packages/contracts/environment/v1) are in the build context:
#   docker build -f apps/web/Dockerfile -t cordvba-web .

# --- build ---
FROM node:24-slim AS build
WORKDIR /repo

# Non-interactive: pnpm's pre-run dependency check must never try to prompt
# to purge node_modules (ERR_PNPM_ABORTED_REMOVE_MODULES_DIR_NO_TTY).
ENV CI=true

RUN corepack enable

COPY apps/web/package.json apps/web/pnpm-lock.yaml apps/web/pnpm-workspace.yaml apps/web/
WORKDIR /repo/apps/web
RUN pnpm install --frozen-lockfile

WORKDIR /repo
COPY packages/contracts/environment/v1 packages/contracts/environment/v1
COPY apps/web apps/web

WORKDIR /repo/apps/web
RUN pnpm build

# --- serve ---
FROM nginx:1.27-alpine AS serve

COPY apps/web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /repo/apps/web/dist /usr/share/nginx/html

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
