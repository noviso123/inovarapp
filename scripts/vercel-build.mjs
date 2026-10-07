import { createHash } from 'node:crypto';
import { execFileSync, spawnSync } from 'node:child_process';
import { chmod, mkdir, mkdtemp, readFile, rename, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';

const root = process.cwd();

function executableWorks(command, args) {
  const result = spawnSync(command, args, { cwd: root, stdio: 'ignore', shell: false });
  return !result.error && result.status === 0;
}

async function goExecutable() {
  if (executableWorks('go', ['version'])) return 'go';
  if (process.platform !== 'linux') {
    throw new Error('Go não está instalado neste ambiente e o download automático só é suportado no Linux.');
  }

  const arch = process.arch === 'arm64' ? 'arm64' : process.arch === 'x64' ? 'amd64' : '';
  if (!arch) throw new Error(`Arquitetura Linux não suportada para Go: ${process.arch}`);

  const goMod = await readFile(path.join(root, 'go.mod'), 'utf8');
  const match = goMod.match(/^go\s+(\d+\.\d+\.\d+)/m);
  if (!match) throw new Error('A versão Go não foi encontrada no go.mod.');
  const version = `go${match[1]}`;
  const filename = `${version}.linux-${arch}.tar.gz`;
  const baseDir = path.join(os.tmpdir(), 'inovar-go-toolchain', version);
  const goBinary = path.join(baseDir, 'bin', 'go');
  if (executableWorks(goBinary, ['version'])) return goBinary;

  console.log(`Go não está instalado no builder. Baixando ${version} oficial para concluir a compilação.`);
  const releasesResponse = await fetch('https://go.dev/dl/?mode=json&include=all');
  if (!releasesResponse.ok) throw new Error(`Não foi possível consultar os checksums oficiais do Go (${releasesResponse.status}).`);
  const releases = await releasesResponse.json();
  const release = releases.find((item) => item.version === version);
  const archive = release?.files?.find((item) => item.filename === filename && item.kind === 'archive');
  if (!archive?.sha256) throw new Error(`A versão ${version} ou seu checksum oficial não foi encontrado.`);

  const installRoot = await mkdtemp(path.join(os.tmpdir(), 'inovar-go-install-'));
  const archivePath = path.join(installRoot, filename);
  try {
    const response = await fetch(`https://go.dev/dl/${filename}`);
    if (!response.ok) throw new Error(`Download do Go falhou (${response.status}).`);
    const data = Buffer.from(await response.arrayBuffer());
    const checksum = createHash('sha256').update(data).digest('hex');
    if (checksum !== archive.sha256) throw new Error('O checksum do pacote Go não confere com o publicado pelo Go Team.');
    await writeFile(archivePath, data, { mode: 0o600 });
    const parent = path.dirname(baseDir);
    const extractDir = path.join(parent, `${version}-extract`);
    await rm(baseDir, { recursive: true, force: true });
    await rm(extractDir, { recursive: true, force: true });
    await mkdir(extractDir, { recursive: true });
    execFileSync('tar', ['-xzf', archivePath, '-C', extractDir]);
    await rename(path.join(extractDir, 'go'), baseDir);
    await chmod(goBinary, 0o755);
    const check = spawnSync(goBinary, ['version'], { cwd: root, encoding: 'utf8', shell: false });
    if (check.error || check.status !== 0) {
      const cause = check.error?.message || (check.stderr || '').trim() || `código ${check.status}`;
      throw new Error(`O Go foi baixado, mas não iniciou no builder: ${cause}`);
    }
    return goBinary;
  } finally {
    await rm(path.join(os.tmpdir(), 'inovar-go-toolchain', `${version}-extract`), { recursive: true, force: true });
    await rm(installRoot, { recursive: true, force: true });
  }
}

const go = await goExecutable();
const result = spawnSync(go, ['run', '-buildvcs=false', './cmd/web/vercelbuild'], {
  cwd: root,
  stdio: 'inherit',
  env: { ...process.env, GOTOOLCHAIN: 'local' },
  shell: false,
});
if (result.error) throw result.error;
if (result.status !== 0) process.exit(result.status ?? 1);

