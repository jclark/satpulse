#!/usr/bin/env node

import * as esbuild from 'esbuild'
import { writeFile } from 'fs/promises'

// Bundle the TSX test code to an in-memory string
const result = await esbuild.build({
  entryPoints: ['test-skyview.tsx'],
  bundle: true,
  write: false,
  format: 'esm',
})

// Get bundled JS string
const jsCode = result.outputFiles[0].text

// Basic HTML template with inline JS
const html = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <title>SkyView Test</title>
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <link href="style.css" rel="stylesheet" />
</head>
<body class="p-4">
  <div id="root"></div>
  <script type="module">
${jsCode}
  </script>
</body>
</html>
`

// Write out a single test file
await writeFile('test-skyview.html', html)

console.log('✅ Wrote test-skyview.html')