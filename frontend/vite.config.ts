import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
    },
  },
  server: {
    port: 5185,
    proxy: {
      '/api':     process.env.VITE_API_PROXY || 'http://localhost:8080',
      '/healthz': process.env.VITE_API_PROXY || 'http://localhost:8080',
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    rollupOptions: {
      output: {
        // Sık değişmeyen framework paketlerini uygulama/çeviri kodundan ayırır.
        // Böylece küçük bir sayfa değişikliği React, router ve i18n önbelleğini
        // geçersiz kılmaz; CodeMirror da yalnız editör kullanan rotalara kalır.
        manualChunks(id) {
          if (!id.includes('node_modules')) return
          if (id.includes('/@codemirror/') || id.includes('/@uiw/react-codemirror/')) return 'editor-vendor'
          if (id.includes('/react/') || id.includes('/react-dom/') || id.includes('/react-router') || id.includes('/scheduler/')) return 'react-vendor'
          if (id.includes('/i18next/') || id.includes('/react-i18next/')) return 'i18n-vendor'
          if (id.includes('/axios/') || id.includes('/zustand/')) return 'data-vendor'
        },
      },
    },
  },
})
