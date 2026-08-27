# syntax=docker/dockerfile:1.7

ARG BASE_BUILDER_IMAGE=golang:1.26.1-alpine
ARG BASE_RUNTIME_IMAGE=alpine:3.21
FROM ${BASE_BUILDER_IMAGE} AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates git tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN workbench_version="$(tr -d '[:space:]' < VERSION)" \
  && echo "$workbench_version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' \
  && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath \
  -ldflags="-s -w -X github.com/endge-lab/service-ai-workbench/internal/buildinfo.Version=$workbench_version" \
  -buildvcs=false -o /out/service-ai-workbench ./cmd/main.go

FROM ${BASE_RUNTIME_IMAGE}

WORKDIR /app

RUN addgroup -S app && adduser -S app -G app \
  && apk add --no-cache ca-certificates tzdata

COPY --from=builder /out/service-ai-workbench /app/service-ai-workbench
COPY docs /app/docs

USER app

EXPOSE 8081

ENTRYPOINT ["/app/service-ai-workbench"]
