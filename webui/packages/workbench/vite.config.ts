import {defineConfig} from 'vitest/config';
import preact from '@preact/preset-vite';

export default defineConfig({
    plugins: [preact()],
    test: {
        environment: 'node',
        include: ['src/**/*.test.ts', 'src/**/*.test.tsx'],
    },
});
