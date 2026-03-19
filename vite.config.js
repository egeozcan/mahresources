import { defineConfig } from 'vite';

export default defineConfig({
  publicDir: false,
  base: '/public/dist/',
  build: {
    outDir: 'public/dist',
    emptyOutDir: true,
    rollupOptions: {
      input: 'src/main.js',
      output: {
        // Keep entry name stable for hardcoded template references
        entryFileNames: 'main.js',
        // Content hashes on dynamic chunks for cache busting
        chunkFileNames: 'assets/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash][extname]',
        manualChunks(id) {
          if (id.includes('/diff/')) return 'diff';
        },
      },
    },
  },
});
