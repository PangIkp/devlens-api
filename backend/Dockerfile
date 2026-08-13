FROM golang:1.25-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates git

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags="-s -w -X github.com/PangIkp/devlens/backend/internal/buildinfo.Version=${VERSION} -X github.com/PangIkp/devlens/backend/internal/buildinfo.Commit=${COMMIT} -X github.com/PangIkp/devlens/backend/internal/buildinfo.BuildTime=${BUILD_TIME}" \
  -o /out/devlens-api ./cmd/api

FROM alpine:3.22

RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 appuser

WORKDIR /app

COPY --from=builder /out/devlens-api /usr/local/bin/devlens-api

USER appuser

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/devlens-api"]
