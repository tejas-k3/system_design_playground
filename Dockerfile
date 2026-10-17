# syntax=docker/dockerfile:1

# 1. build the frontend
FROM node:22-alpine AS web
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# 2. build a static Go binary
FROM golang:1.23-alpine AS api
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
 && mkdir /out/data

# 3. run it on a minimal image as a non-root user
FROM gcr.io/distroless/static-debian12:nonroot
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
