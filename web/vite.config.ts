import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { fileURLToPath, URL } from 'node:url'
import { execSync } from 'node:child_process'
import { readFileSync } from 'node:fs'

// Read the version straight from package.json so it stays in sync with releases.
const pkg = JSON.parse(
  readFileSync(fileURLToPath(new URL('./package.json', import.meta.url)), 'utf8'),
)

// Best-effort git short hash; falls back gracefully when .git is absent
// (e.g. an extracted tarball) so the build never fails.
function gitShortHash(): string {
  try {
    return execSync('git rev-parse --short HEAD', {
      stdio: ['ignore', 'pipe', 'ignore'],
    })
      .toString()
      .trim()
  } catch {
    return 'unknown'
  }
}

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  define: {
    __APP_VERSION__: JSON.stringify(pkg.version),
    __APP_GIT_HASH__: JSON.stringify(gitShortHash()),
  },
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  build: {
    // Emit straight into the Go embed directory so the binary bundles the UI.
    outDir: fileURLToPath(new URL('../internal/web/dist', import.meta.url)),
    emptyOutDir: true,
    assetsDir: 'assets/vite',
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
