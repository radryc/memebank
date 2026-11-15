# syntax=docker/dockerfile:1

FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS api
ARG VERSION=dev
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/memebank .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 -h /app memebank \
    && mkdir -p /data \
    && chown -R memebank:memebank /data /app
WORKDIR /app
COPY --from=api /out/memebank /usr/local/bin/memebank
COPY --from=web /src/web/dist /app/web/dist
USER memebank
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/memebank"]
CMD ["-addr", ":8080", "-data", "/data", "-static", "/app/web/dist"]
