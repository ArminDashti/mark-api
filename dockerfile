# syntax=docker/dockerfile:1
# Build from repo root: docker build -f dockerfile -t mark-api:latest .

FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY migrations/ ./migrations/
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
  && adduser -D -u 10001 appuser \
  && mkdir -p /data \
  && chown -R appuser:appuser /data
WORKDIR /app
COPY --from=build /out/server /app/server
COPY --from=build /src/migrations /app/migrations
RUN chown -R appuser:appuser /app
USER appuser
ENV ADDR=:8130 \
    MIGRATIONS_DIR=/app/migrations \
    DATA_DIR=/data \
    GIN_MODE=release
EXPOSE 8130
CMD ["/app/server"]
