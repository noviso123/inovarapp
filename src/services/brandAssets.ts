// Imagens da marca para os PDFs — servidas como ARQUIVOS pelo CDN (/public)
// e carregadas sob demanda somente na hora de gerar um PDF. Antes elas ficavam
// embutidas em base64 dentro do bundle JavaScript (1,9MB carregados em toda
// visita — agora saem do caminho crítico e ficam em cache na memória).

export const BRAND_IMG_URLS = {
  // Versões otimizadas especificamente para A4. Reduzem drasticamente o
  // tempo de conversão para base64/jsPDF, sem perder definição na impressão.
  INOVAR_HEADER: '/inovar-brand/INOVAR_HEADER_PDF.png',
  INOVAR_FOOTER_REFERENCE: '/inovar-brand/INOVAR_FOOTER_PDF.png',
  INOVAR_SIGNATURE_GABRIEL: '/inovar-brand/INOVAR_SIGNATURE_GABRIEL_PDF.png'
};

const cache: Record<string, string> = {};

// Busca a imagem e devolve como dataURL (formato que o doc.addImage do jsPDF consome)
export async function carregarImagemMarca(url: string): Promise<string> {
  if (cache[url]) return cache[url];
  const r = await fetch(url);
  if (!r.ok) throw new Error('Falha ao carregar imagem da marca: ' + url);
  const blob = await r.blob();
  const dataUrl = await new Promise<string>((resolve, reject) => {
    const fr = new FileReader();
    fr.onload = () => resolve(String(fr.result));
    fr.onerror = () => reject(new Error('FileReader falhou'));
    fr.readAsDataURL(blob);
  });
  cache[url] = dataUrl;
  return dataUrl;
}

// Conjunto mínimo usado pelos documentos. Nenhum ativo não utilizado entra
// na primeira geração de PDF.
export async function imagensMarca() {
  const [INOVAR_HEADER, INOVAR_FOOTER_REFERENCE, INOVAR_SIGNATURE_GABRIEL] = await Promise.all([
    carregarImagemMarca(BRAND_IMG_URLS.INOVAR_HEADER),
    carregarImagemMarca(BRAND_IMG_URLS.INOVAR_FOOTER_REFERENCE),
    carregarImagemMarca(BRAND_IMG_URLS.INOVAR_SIGNATURE_GABRIEL)
  ]);
  return { INOVAR_HEADER, INOVAR_FOOTER_REFERENCE, INOVAR_SIGNATURE_GABRIEL };
}

/** Pré-aquece os ativos no tempo ocioso; abrir/emitir o documento fica imediato. */
export function preloadPdfAssets(): Promise<unknown> {
  return imagensMarca().catch(() => undefined);
}
