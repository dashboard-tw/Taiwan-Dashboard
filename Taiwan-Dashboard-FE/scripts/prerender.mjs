import { mkdir, rm, writeFile } from 'node:fs/promises'
import { pathToFileURL } from 'node:url'
import { build } from 'vite'

const out = 'node_modules/.tmp/prerender'
await build({
  logLevel: 'warn',
  build: { ssr: 'src/prerender.ts', outDir: out, emptyOutDir: true, copyPublicDir: false },
})
const { pages, render } = await import(pathToFileURL(`${out}/prerender.js`).href)
await mkdir('dist/prerender', { recursive: true })
for (const [name, view] of Object.entries(pages)) {
  const html = (await render(view, `/${name}`)).replace(/<!--[\s\S]*?-->/g, '')
  await writeFile(`dist/prerender/${name}.html`, html)
  console.log(`prerender /${name}: ${html.length} bytes`)
}
await rm(out, { recursive: true, force: true })
