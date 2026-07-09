import {defineConfig} from 'vite'
import preact from '@preact/preset-vite'
import tailwindcss from '@tailwindcss/vite'

// The workbench package is a file: symlink; imports inside its sources
// resolve preact from webui/node_modules. Dedupe so the app gets a
// single preact instance.
export default defineConfig({
  plugins: [preact(), tailwindcss()],
  resolve: {dedupe: ['preact']},
})
