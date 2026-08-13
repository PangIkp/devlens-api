FROM golang:1.25-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/devlens-api ./cmd/api

FROM alpine:3.22

RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 appuser

WORKDIR /app

COPY --from=builder /out/devlens-api /usr/local/bin/devlens-api

USER appuser

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/devlens-api"]
