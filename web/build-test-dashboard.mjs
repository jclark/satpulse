#!/usr/bin/env node

import * as esbuild from 'esbuild'
import { writeFile } from 'fs/promises'

// Bundle the TSX test code to an in-memory string
const result = await esbuild.build({
  entryPoints: ['test-dashboard.tsx'],
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
  <title>Dashboard Test</title>
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <link href="style.css" rel="stylesheet" />
</head>
<body class="bg-gray-100 dark:bg-gray-900 font-sans m-0 p-0">
  <div id="root"></div>
  <script type="module">
  // Include the bundled code
  ${jsCode}
  // Call renderDashboard with window.location.search
  renderDashboard(window.location.search);
  </script>
</body>
</html>
`

// Write out a single test file
await writeFile('test-dashboard.html', html)

console.log('✅ Wrote test-dashboard.html')