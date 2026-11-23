/**
 * Konfigurasi environment development (code-server).
 *
 * `apiUrl` mengarah ke backend Go melalui code-server proxy di port 8081:
 *   /absproxy/8081/api/v1/auth/login  ->  code-server -> localhost:8081/api/v1/auth/login
 *
 * Semua request melalui code-server (same origin), tidak ada masalah CORS.
 * base href tetap "/absproxy/4200/" untuk frontend.
 */
export const environment = {
  production: false,
  apiUrl: '/absproxy/8081/api/v1'
};
