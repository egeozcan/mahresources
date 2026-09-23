import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [{
    name: 'escape-trailing-template-tab',
    generateBundle(_options, bundle) {
      for (const output of Object.values(bundle)) {
        if (output.type !== 'chunk') continue;
        // Lit's SPACE_CHAR template includes a literal tab at the end of this line.
        // Keep the same cooked template value while avoiding trailing whitespace in the bundle.
        output.code = output.code.replace(/\[ \t\n/g, '[ \\t\n');
      }
    },
  }],
  test: {
    include: ['src/**/*.test.ts'],
    exclude: ['e2e/**', 'node_modules/**'],
  },
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
          if (id.includes('mrqlEditor')) return 'mrql';
        },
      },
    },
  },
});
