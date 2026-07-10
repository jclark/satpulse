import {defineConfig} from 'vite'
import preact from '@preact/preset-vite'
import tailwindcss from '@tailwindcss/vite'

// Content hashing is disabled so the embedded filenames stay app.js and
// style.css: the checked-in assets under cmd/satpulsewb/dist only change
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
})
