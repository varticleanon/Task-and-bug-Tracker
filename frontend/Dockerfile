# ---- Build stage ----
FROM oven/bun:1 AS build

WORKDIR /app

# Instal dependensi terlebih dahulu agar cache Docker optimal.
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile

COPY . .

# Build SPA untuk produksi.
# base-href "/" dipakai karena nginx menyajikan aplikasi di root,
# bukan di prefix /absproxy/4200/ seperti lingkungan code-server.
# Dijalankan lewat bun (bun-only image) agar tidak butuh Node.js.
RUN bun ./node_modules/.bin/ng build --configuration production --base-href /

# ---- Runtime stage ----
FROM nginxinc/nginx-unprivileged:1.27-alpine

# Versi aplikasi disuntikkan dari file VERSION oleh Compose/CI.
ARG APP_VERSION=dev
LABEL org.opencontainers.image.title="task-bug-tracker-web" \
      org.opencontainers.image.version="${APP_VERSION}"

# Konfigurasi server: SPA fallback + proxy API same-origin.
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist/task-bug-tracker/browser /usr/share/nginx/html

EXPOSE 8080
