FROM golang:1.26.2-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/logmonitor ./cmd/server

FROM alpine:3.22

WORKDIR /app

RUN apk add --no-cache ca-certificates openssh-client && \
    addgroup -S logmonitor && \
    adduser -S -G logmonitor logmonitor

COPY --from=builder /out/logmonitor /app/logmonitor
COPY config.docker.yaml /app/config.yaml
COPY migrations /app/migrations

USER logmonitor

EXPOSE 8080

ENTRYPOINT ["/app/logmonitor"]
CMD ["-config", "/app/config.yaml"]
