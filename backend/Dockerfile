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

FROM scratch

WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/devlens-api /usr/local/bin/devlens-api

USER 10001:10001

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/devlens-api"]
