import { defineConfig } from 'vite';

export default defineConfig({
  publicDir: false,
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
        manualChunks: {
          codemirror: [
            '@codemirror/view',
            '@codemirror/state',
            '@codemirror/commands',
            '@codemirror/language',
            '@codemirror/autocomplete',
          ],
          diff: ['diff'],
        },
      },
    },
  },
});
