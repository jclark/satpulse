import {defineConfig} from 'vite'
import preact from '@preact/preset-vite'
import tailwindcss from '@tailwindcss/vite'

// Content hashing is disabled so the embedded filenames stay app.js and
// style.css: the checked-in assets under cmd/satpulsewb/dist only change
// when their content changes, keeping git diffs clean.
export default defineConfig({
  plugins: [preact(), tailwindcss()],
  // Dev-only: `npm run dev` serves HMR assets and proxies the API and event
  // stream to a running satpulsewb, so the browser gets live-reloaded assets
  // from vite but real events from the session. Inert in the embedded build
  // (`vite build` ignores `server`). Point satpulsewb at this fixed loopback
  // port with `satpulsewb --listen 127.0.0.1:15754`, which also disables the
  // access token, so no ?t= is needed. See ../../CLAUDE.md.
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:15754',
      '/sse': 'http://127.0.0.1:15754',
    },
  },
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
