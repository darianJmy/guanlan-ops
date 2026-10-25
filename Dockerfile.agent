FROM golang:1.25-alpine AS builder

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download github.com/gorilla/websocket
COPY cmd/aiops-agent ./cmd/aiops-agent
COPY internal/agent ./internal/agent
COPY pkg/syncbuffer ./pkg/syncbuffer
COPY pkg/archiveutil ./pkg/archiveutil
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/aiops-agent ./cmd/aiops-agent

FROM python:3.12-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends bash ca-certificates coreutils curl gzip iputils-ping tar \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --no-create-home --shell /usr/sbin/nologin aiops

ENV AIOPS_AGENT_RUNTIME=docker \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

USER aiops
COPY --from=builder /out/aiops-agent /usr/local/bin/aiops-agent

ENTRYPOINT ["/usr/local/bin/aiops-agent"]
