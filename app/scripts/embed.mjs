// Copies the web build (dist/) into the control center, which embeds it with
// go:embed (control/internal/web/dist). Run after `npm run build`.
import { cpSync, existsSync, readdirSync, rmSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const source = join(here, '..', 'dist');
const target = join(here, '..', '..', 'control', 'internal', 'web', 'dist');

if (!existsSync(join(source, 'index.html'))) {
  console.error('dist/index.html missing: run `npm run build` first');
  process.exit(1);
}
for (const entry of readdirSync(target)) {
  if (entry !== '.gitkeep') rmSync(join(target, entry), { recursive: true, force: true });
}
cpSync(source, target, { recursive: true });
console.log(`copied ${source} -> ${target}`);
