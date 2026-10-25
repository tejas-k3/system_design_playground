# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

# Base images are pinned by digest so a build does not depend on what a mutable tag points at today, and
# a registry that is slow or half-down cannot change what we get. To refresh them (do this deliberately,
# then run `make docker-build` and the smoke test): `bash docs/review/tools/image-digests.sh`, and paste
# the digests it prints here. The # syntax line is the Dockerfile frontend image, which BuildKit also pulls.

# 1. build the frontend
FROM node:22-alpine@sha256:0a7108bf6c7bf5de370ffb1a3ed6be93d405b43ff159f681a8d18c0e2bc2e402 AS web
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# 2. build a static Go binary
FROM golang:1.27-alpine@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS api
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
 && mkdir /out/data

# 3. run it on a minimal image as a non-root user
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=api /out/server /app/server
COPY --from=web /src/frontend/dist /app/web
# shared designs are stored here; mount a volume to keep them across container restarts
COPY --from=api --chown=nonroot:nonroot /out/data /data
ENV ADDR=:8090 STATIC_DIR=/app/web DATA_DIR=/data
EXPOSE 8090
VOLUME /data
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 CMD ["/app/server", "-healthcheck"]
ENTRYPOINT ["/app/server"]
