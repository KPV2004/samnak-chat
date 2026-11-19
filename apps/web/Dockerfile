# syntax=docker/dockerfile:1
# Build from the repository root (the npm workspace needs the root lock file):
#   docker buildx build -f apps/web/Dockerfile --platform linux/amd64,linux/arm64 .
# The build stage only produces static files, so it runs on the build machine's
# platform; only the Caddy stage is built per architecture.
FROM --platform=$BUILDPLATFORM node:22-alpine AS build
WORKDIR /src
COPY package.json package-lock.json ./
COPY apps/web/package.json apps/web/
COPY packages/core/package.json packages/core/
RUN npm ci
COPY apps/web/index.html apps/web/tsconfig.json apps/web/vite.config.ts apps/web/
COPY apps/web/src apps/web/src
COPY packages/core/src packages/core/src
RUN npm run build -w @samnak/web

FROM caddy:2-alpine
COPY deploy/Caddyfile /etc/caddy/Caddyfile
COPY --from=build /src/apps/web/dist /srv
EXPOSE 80 443
# An empty DOMAIN would leave the Caddyfile without a site address, so the shell
# supplies the plain HTTP default when DOMAIN is unset or empty.
CMD ["/bin/sh", "-c", "SITE_ADDRESS=${DOMAIN:-http://localhost} exec caddy run --config /etc/caddy/Caddyfile --adapter caddyfile"]
