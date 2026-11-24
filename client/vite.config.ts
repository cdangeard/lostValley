import { defineConfig } from 'vite';

// The art and maps live in ../assets so the Go server reads the very same
// Tiled files. In dev, /ws is proxied to the Go server on :8080.
export default defineConfig({
  publicDir: '../assets',
  server: {
    port: 5173,
    proxy: {
      '/ws': { target: 'ws://localhost:8080', ws: true },
    },
  },
  build: {
    chunkSizeWarningLimit: 2000, // Phaser alone is ~1.2 MB
  },
});
