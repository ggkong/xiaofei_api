// Run the full type check and bundler sequentially to fit a small deployment host.
// This does not skip type checking; it prevents Vite from starting a second copy.
import { spawnSync } from 'node:child_process'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('../', import.meta.url))
process.chdir(root)
const require = createRequire(import.meta.url)
const typecheck = spawnSync(process.execPath, [
  '--max-old-space-size=2304', require.resolve('vue-tsc/bin/vue-tsc.js'), '--noEmit'
], { cwd: root, stdio: 'inherit' })
if (typecheck.status !== 0) process.exit(typecheck.status ?? 1)

const { build, loadConfigFromFile } = await import('vite')
const loaded = await loadConfigFromFile({ command: 'build', mode: 'production' }, `${root}vite.config.ts`)
if (!loaded) throw new Error('Cannot load Vite config')
const plugins = loaded.config.plugins.flat(Infinity).filter(plugin => plugin && plugin.name !== 'vite-plugin-checker')
await build({ ...loaded.config, configFile: false, plugins })
