# syntax=docker/dockerfile:1
# Image unique : le client Phaser compilé est servi par le serveur Go, sur le
# même port que /ws (même origine, donc rien à configurer côté WebSocket).

# 1. Client (Vite) : client/dist contient aussi assets/ (publicDir)
FROM node:22-alpine AS client
WORKDIR /src/client
COPY client/package.json client/package-lock.json ./
RUN npm ci
COPY client/ ./
COPY assets/ /src/assets/
RUN npm run build

# 2. Serveur Go, binaire statique
FROM golang:1.24-alpine AS server
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/lostvalley ./cmd/lostvalley

# 3. Exécution
FROM alpine:3.22
# alpine fournit déjà les certificats racine et wget (busybox).
RUN adduser -D -H -u 10001 vallee
WORKDIR /app
COPY --from=server /out/lostvalley /app/lostvalley
COPY assets/ /app/assets/
COPY --from=client /src/client/dist /app/public
ENV LV_ADDR=:8080 \
    LV_MAP=/app/assets/maps/valley.json \
    LV_CONTENT=/app/assets/data/contenu.json \
    LV_STATIC=/app/public
USER vallee
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
# Le serveur sauvegarde la vallée en recevant SIGTERM.
STOPSIGNAL SIGTERM
ENTRYPOINT ["/app/lostvalley"]
