import { defineConfig } from 'vitest/config';
import preact from '@preact/preset-vite';
import tailwindcss from '@tailwindcss/vite';

// Content hashing is disabled so the embedded filenames stay app.js and
// style.css: the checked-in assets under time/internal/web/dist only change
// when their content changes, keeping git diffs clean.
export default defineConfig({
  plugins: [preact(), tailwindcss()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        entryFileNames: 'app.js',
        chunkFileNames: 'app.js',
        assetFileNames: (info) =>
          (info.names ?? []).some((n) => n.endsWith('.css')) ? 'style.css' : '[name][extname]',
      },
    },
  },
  test: {
    globals: true,
    environment: 'node',
    include: ['src/**/*.test.ts', 'src/**/*.test.tsx'],
  },
});
