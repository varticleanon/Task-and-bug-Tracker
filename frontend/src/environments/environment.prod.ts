/**
 * Konfigurasi environment produksi.
 *
 * Produksi disajikan via nginx (Docker Compose) yang mem-proxy
 * "/api/v1/*" ke backend Go di port 8080, jadi apiUrl tetap relatif
 * tanpa prefix /absproxy (hanya berlaku di dev code-server).
 */
export const environment = {
  production: true,
  apiUrl: '/api/v1'
};
