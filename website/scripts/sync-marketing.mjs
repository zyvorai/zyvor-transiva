// Copy the static marketing pages from docs/ into website/static/docs/ so the
// Docusaurus build publishes them at <baseUrl>/docs/marketing/... (the URLs the
// README and the pages themselves link to). Runs automatically before `npm run build`.
import {cpSync, mkdirSync, rmSync, existsSync} from 'node:fs';
import {dirname, join} from 'node:path';
import {fileURLToPath} from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const out = join(root, 'website', 'static', 'docs');
rmSync(out, {recursive: true, force: true});
for (const sub of ['marketing', 'social']) {
  const from = join(root, 'docs', sub);
  if (!existsSync(from)) continue;
  mkdirSync(join(out, sub), {recursive: true});
  cpSync(from, join(out, sub), {recursive: true});
}
console.log('synced docs/marketing and docs/social into website/static/docs');
