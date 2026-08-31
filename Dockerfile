# Build stage
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o zenithd ./cmd/zenithd

FROM alpine:latest
RUN apk --no-cache add ca-certificates \
    && adduser -D -H -u 65532 zenith
USER 65532:65532
WORKDIR /home/zenith
COPY --from=builder --chown=65532:65532 /app/zenithd .
EXPOSE 7001 8001 9001
ENTRYPOINT ["./zenithd"]
