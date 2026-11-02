# syntax=docker/dockerfile:1
FROM node:24-alpine@sha256:8a32c8cf649a49915a66e81e7a609223c708d0adabf288ee3f4077d765542f85 AS ui
WORKDIR /src
COPY package.json package-lock.json ./
# Optional combined trust bundle for managed environments. Never copied into layers.
RUN --mount=type=secret,id=proxy_ca \
    if [ -f /run/secrets/proxy_ca ]; then export NODE_EXTRA_CA_CERTS=/run/secrets/proxy_ca; fi; \
    npm ci --strict-ssl=true
COPY vite.config.js ./
COPY web ./web
COPY internal/ui/templates ./internal/ui/templates
RUN npm run build

FROM golang:1.27.3-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=secret,id=proxy_ca --mount=type=cache,target=/go/pkg/mod \
    if [ -f /run/secrets/proxy_ca ]; then export SSL_CERT_FILE=/run/secrets/proxy_ca; fi; \
    go mod download
COPY . .
COPY --from=ui /src/internal/ui/static/dist ./internal/ui/static/dist
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOMAXPROCS=4 go build -trimpath -ldflags='-s -w' -o /out/kb ./cmd/kb
RUN mkdir -p /out/data /out/backups

FROM scratch
COPY --from=build /out/kb /app/kb
COPY --from=build --chown=65532:65532 /out/data /data
COPY --from=build --chown=65532:65532 /out/backups /backups
USER 65532:65532
ENV KB_DATA_DIR=/data KB_BACKUP_DIR=/backups KB_LISTEN_ADDR=:8080 GOMEMLIMIT=512MiB GOMAXPROCS=2
EXPOSE 8080
ENTRYPOINT ["/app/kb"]
CMD ["serve"]
