import { copyFile, mkdir } from "node:fs/promises"
import { createRequire } from "node:module"
import { dirname } from "node:path"
import { fileURLToPath } from "node:url"

// Vendor zxing-wasm's .wasm same-origin (the offline station never hits a CDN),
// resolved from the installed package so it can't drift from the JS glue.
const require = createRequire(import.meta.url)
const src = require.resolve("zxing-wasm/reader/zxing_reader.wasm")
const dest = fileURLToPath(new URL("../public/assets/zxing_reader.wasm", import.meta.url))

await mkdir(dirname(dest), { recursive: true })
await copyFile(src, dest)
console.log(`copy-zxing-wasm: ${src} -> ${dest}`)
