import { copyFile, mkdir } from "node:fs/promises"
import { createRequire } from "node:module"
import { dirname } from "node:path"
import { fileURLToPath } from "node:url"

// The station scanner (zxing-wasm) must load its .wasm same-origin so the fully
// offline pickup station never reaches a CDN. Copy the wasm that ships with the
// installed package — resolved via its exports map so it stays in lockstep with
// the JS glue — into public/ where the service worker precaches it.
const require = createRequire(import.meta.url)
const src = require.resolve("zxing-wasm/reader/zxing_reader.wasm")
const dest = fileURLToPath(new URL("../public/assets/zxing_reader.wasm", import.meta.url))

await mkdir(dirname(dest), { recursive: true })
await copyFile(src, dest)
console.log(`copy-zxing-wasm: ${src} -> ${dest}`)
