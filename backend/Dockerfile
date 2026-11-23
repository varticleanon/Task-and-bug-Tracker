# ---- Build stage ----
FROM golang:1.27-alpine AS builder

WORKDIR /app

# Unduh dependensi terlebih dahulu agar cache Docker optimal.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Satu binary per command: api, migrate, bootstrap.
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/migrate ./cmd/migrate
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/bootstrap ./cmd/bootstrap

# ---- Runtime stage ----
FROM alpine:3.21

# Versi aplikasi disuntikkan dari file VERSION oleh Compose/CI.
ARG APP_VERSION=dev
LABEL org.opencontainers.image.title="task-bug-tracker-api" \
      org.opencontainers.image.version="${APP_VERSION}"

RUN apk add --no-cache ca-certificates curl

WORKDIR /app

COPY --from=builder /app/bin/api /app/api
COPY --from=builder /app/bin/migrate /app/migrate
COPY --from=builder /app/bin/bootstrap /app/bootstrap

# Direktori privat untuk lampiran (persistent volume) + pengguna non-root.
# Chown dilakukan setelah COPY supaya biner dan direktori dimiliki app.
RUN addgroup -S app && adduser -S -G app app && \
    mkdir -p /var/lib/task-bug-tracker/attachments && \
    chown -R app:app /app /var/lib/task-bug-tracker && \
    chmod 700 /var/lib/task-bug-tracker/attachments

USER app

EXPOSE 8080

CMD ["/app/api"]
