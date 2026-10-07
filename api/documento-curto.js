// Redirecionador público de documento. O link curto não revela URL assinada,
// dados do cliente nem o caminho do arquivo; ele expira no mesmo prazo definido
// pelo responsável ao enviar a OS ou proposta.
import { SUPABASE_URL, queryAsService } from './_supabase.js';

const BUCKET = 'documentos-inovar';
const CODIGO = /^[A-Za-z0-9_-]{10,20}$/;

function responder(res, status, texto) {
  res.statusCode = status;
  res.setHeader('Content-Type', 'text/plain; charset=utf-8');
  res.end(texto);
}

export default async function handler(req, res) {
  if (req.method !== 'GET') return responder(res, 405, 'Método não permitido.');
  const codigo = String(req.query?.c || '');
  if (!CODIGO.test(codigo)) return responder(res, 404, 'Documento não encontrado.');

  try {
    const arquivo = await fetch(`${SUPABASE_URL}/storage/v1/object/${BUCKET}/links/${codigo}.json`, {
      headers: {
        apikey: process.env.SUPABASE_SERVICE_ROLE_KEY,
        Authorization: `Bearer ${process.env.SUPABASE_SERVICE_ROLE_KEY}`
      }
    });
    const referencia = await arquivo.json().catch(() => null);
    if (!arquivo.ok || !referencia?.path || !referencia?.expiraEm || Date.now() > Number(referencia.expiraEm)) {
      return responder(res, 410, 'Este link de documento expirou. Solicite um novo à Inovar Refrigeração.');
    }
    if (!/^(os-orcamentos)\/[a-zA-Z0-9_ .\/-]+$/.test(String(referencia.path))) {
      return responder(res, 404, 'Documento não encontrado.');
    }
    const restante = Math.max(60, Math.floor((Number(referencia.expiraEm) - Date.now()) / 1000));
    const assinado = await queryAsService(`/storage/v1/object/sign/${BUCKET}/${referencia.path}`, {
      method: 'POST', body: { expiresIn: restante }
    });
    if (assinado.status !== 200 || !assinado.data?.signedURL) return responder(res, 404, 'Documento não encontrado.');
    res.statusCode = 302;
    res.setHeader('Cache-Control', 'private, no-store');
    res.setHeader('Location', SUPABASE_URL + '/storage/v1' + assinado.data.signedURL);
    res.end();
  } catch {
    responder(res, 503, 'Não foi possível abrir o documento agora. Tente novamente em instantes.');
  }
}
