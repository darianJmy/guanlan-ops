FROM node:22-bookworm-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
ENV NODE_OPTIONS=--max-old-space-size=4096
RUN npm run build

FROM golang:1.25-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/guanlan .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/service-portal ./cmd/service-portal

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends bash ca-certificates coreutils curl git gzip iputils-ping openssh-client python3 tar tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 guanlan \
    && useradd --uid 10001 --gid 10001 --create-home guanlan \
    && mkdir -p /app/data \
    && chown -R 10001:10001 /app
WORKDIR /app
COPY --from=builder /out/guanlan /usr/local/bin/guanlan
COPY --from=builder /out/service-portal /usr/local/bin/service-portal
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/guanlan"]
