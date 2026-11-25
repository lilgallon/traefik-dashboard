import { svelte } from '@sveltejs/vite-plugin-svelte'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  plugins: [svelte()],
  // Le binaire Go embarque dist/ ; public/.gitkeep y est recopié pour que le
  // dossier existe dans Git (go:embed en a besoin pour compiler sans le front).
  build: { outDir: 'dist', target: 'es2022', sourcemap: false },
  server: {
    // npm run dev : l'API vient d'une instance du service lancée à côté.
    proxy: { '/api': 'http://127.0.0.1:18080', '/health': 'http://127.0.0.1:18080' },
  },
  test: { environment: 'node', include: ['src/**/*.test.ts'] },
})
