import { mkdir, writeFile } from 'node:fs/promises';
import { spawnSync } from 'node:child_process';

// Only public browser settings are used in the native bundle.
const runtimeResponse = await fetch('https://inovarapp.vercel.app/app.js', { cache: 'no-store' });
if (!runtimeResponse.ok) throw new Error(`Public configuration unavailable: ${runtimeResponse.status}`);
const runtime = await runtimeResponse.text();
const match = runtime.match(/const goappEnv = (\{[^\n]+\});/);
if (!match) throw new Error('Go browser configuration not found');
const publicSettings = JSON.parse(match[1]);
for (const key of ['SUPABASE_URL', 'SUPABASE_ANON_KEY']) {
  if (!publicSettings[key]) throw new Error(`Missing public setting: ${key}`);
  process.env[key] = publicSettings[key];
}
process.env.GOAPP_API_BASE_URL = 'https://inovarapp.vercel.app';
function run(command, args) {
  const result = spawnSync(command, args, { stdio: 'inherit', env: process.env });
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status ?? 1);
}
run('npm', ['run', 'build']);
run('go', ['run', './cmd/web/export', '-out', 'mobile/www']);
run('npx', ['esbuild', 'src/platform/nativeContactsBridge.ts', '--bundle', '--format=esm', '--outfile=mobile/www/native-contacts.js']);
run('node', ['scripts/inject-mobile-contacts-bridge.mjs', 'mobile/www/index.html']);
run('npx', ['cap', 'sync', 'ios']);
await mkdir('build/ios', { recursive: true });
await writeFile('build/ios/BUILD-STATUS.txt', 'Compiled iOS archive without Apple distribution signing. This archive cannot be installed on an iPhone or distributed as an installer. Apple Developer membership and signing are required.\n');
