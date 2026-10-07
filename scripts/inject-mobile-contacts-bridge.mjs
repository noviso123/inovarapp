import { readFile, writeFile } from 'node:fs/promises';

const htmlPath = process.argv[2];
if (!htmlPath) throw new Error('Informe o caminho do index.html exportado.');
const html = await readFile(htmlPath, 'utf8');
const bootstrapMarker = '<script type="module" data-inovar-mobile-bootstrap>';
if (html.includes(bootstrapMarker)) process.exit(0);
const wasmRuntime = html.match(/<script\b(?=[^>]*\bsrc=["']\/wasm_exec\.js(?:\?[^"']*)?["'])[^>]*>\s*<\/script>/i)?.[0];
const appRuntime = html.match(/<script\b(?=[^>]*\bsrc=["']\/app\.js(?:\?[^"']*)?["'])[^>]*>\s*<\/script>/i)?.[0];
if (!wasmRuntime || !appRuntime) {
  const scripts = html.match(/<script\b[^>]*>/gi)?.join('\n') || '(nenhuma tag script encontrada)';
  throw new Error(`Bootstrap WebAssembly não encontrado no HTML exportado. Scripts encontrados:\n${scripts}`);
}
const bootstrap = `${bootstrapMarker}
      import('/native-contacts.js')
        .then(() => Promise.all([window.inovarNativeOAuthReady, window.inovarNativePushReady]))
        .catch((error) => { console.error('Falha ao inicializar as APIs nativas.', error); })
        .then(() => new Promise((resolve, reject) => {
          const script = document.createElement('script');
          script.src = '/wasm_exec.js';
          script.onload = resolve;
          script.onerror = () => reject(new Error('Falha ao carregar o runtime WebAssembly.'));
          document.head.append(script);
        }))
        .then(() => new Promise((resolve, reject) => {
          const script = document.createElement('script');
          script.src = '/app.js';
          script.onload = resolve;
          script.onerror = () => reject(new Error('Falha ao carregar a interface Go.'));
          document.head.append(script);
        }))
        .catch((error) => console.error('Falha ao iniciar o app Go móvel.', error));
    </script>`;
const updated = html.replace(wasmRuntime, '').replace(appRuntime, bootstrap);
if (updated === html) throw new Error('Não foi possível substituir a sequência de inicialização do WASM.');
await writeFile(htmlPath, updated, 'utf8');
