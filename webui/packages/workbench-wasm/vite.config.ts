import {defineConfig} from 'vite';
import preact from '@preact/preset-vite';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
    base: './',
    plugins: [preact(), tailwindcss()],
    build: {
        outDir: 'dist',
        emptyOutDir: true,
        rollupOptions: {
            output: {
                entryFileNames: 'app.js',
                chunkFileNames: '[name].js',
                assetFileNames: info =>
                    (info.names ?? []).some(n => n.endsWith('.css')) ? 'style.css' : '[name][extname]',
            },
        },
    },
});
