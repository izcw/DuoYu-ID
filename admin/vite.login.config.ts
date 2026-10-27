import { defineConfig } from 'vite'
import { fileURLToPath } from 'node:url'

export default defineConfig({
  define: { 'process.env.NODE_ENV': JSON.stringify('production') },
  build: {
    outDir: 'dist/login-ui',
    emptyOutDir: true,
    lib: {
      entry: fileURLToPath(new URL('./src/login-message.ts', import.meta.url)),
      name: 'DuoyuLoginMessage',
      formats: ['iife'],
      fileName: () => 'message.js',
      cssFileName: 'message'
    }
  }
})
