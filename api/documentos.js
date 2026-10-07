// Documentos da InovarApp — função única com ações.
// acao: 'upload' (PDF/imagem -> storage privado) | 'link' (URL assinada)
//       'fotos' (lista fotos de uma OS) | 'excluirfoto' (remove foto)
// upload: somente ADMIN/TECNICO. link/fotos: qualquer usuário autenticado.
import { json, authCaller, queryAsService, queryAsUser } from './_supabase.js';

const SUPABASE_URL = process.env.VITE_SUPABASE_URL || 'https://ycpswioserctavijhnre.supabase.co';
const BUCKET = 'documentos-inovar';

async function criarLinkCurto(req, objPath, expiraEmSegundos) {
  // O código é aleatório e não contém nome, cliente ou assinatura do Storage.
  // O destino continua privado e expira na mesma data do link assinado.
  const expiraEm = Date.now() + expiraEmSegundos * 1000;
  for (let tentativa = 0; tentativa < 3; tentativa++) {
    const codigo = crypto.randomBytes(9).toString('base64url');
    const r = await fetch(`${SUPABASE_URL}/storage/v1/object/${BUCKET}/links/${codigo}.json`, {
      method: 'POST',
      headers: {
        apikey: process.env.SUPABASE_SERVICE_ROLE_KEY,
        Authorization: `Bearer ${process.env.SUPABASE_SERVICE_ROLE_KEY}`,
        'Content-Type': 'application/json',
        'x-upsert': 'false'
      },
      body: JSON.stringify({ path: objPath, expiraEm })
    });
    if (r.ok) {
      const proto = String(req.headers['x-forwarded-proto'] || 'https').split(',')[0];
      return `${proto}://${req.headers.host}/d/${codigo}`;
    }
    if (r.status !== 409) break;
  }
  return null;
}

async function uploadHandler(req, res, caller) {
  if (caller.role === 'CLIENTE') return json(res, 403, { error: 'Somente a equipe Inovar pode enviar documentos' });

  const { nome, base64, pasta, serviceId, refId } = req.body || {};
  const b64 = String(base64 || '');
  const ehPdf = b64.startsWith('data:application/pdf');
  const mImg = b64.match(/^data:image\/(png|jpeg|jpg|webp);base64,/);
  if (!ehPdf && !mImg) {
    return json(res, 400, { error: 'Envie um PDF ou imagem (PNG/JPEG/WebP) em base64' });
  }
  if (b64.length > 10_500_000) return json(res, 413, { error: 'Arquivo muito grande (máx. ~10MB)' });

  const mime = ehPdf ? 'application/pdf' : (mImg[1] === 'jpg' ? 'image/jpeg' : 'image/' + mImg[1]);
  const ext = ehPdf ? 'pdf' : (mImg[1] === 'jpeg' ? 'jpg' : mImg[1]);
  const limpo = (nome || 'documento').replace(/[^\w\-. ]+/g, '_').slice(0, 80);
  // Fotos: da OS (fotos-os/{serviceId}/) ou do aparelho (fotos-aparelho/{refId}/);
  // PDFs de OS/orçamentos vão para os-orcamentos/{data}/
  let objPath;
  if (pasta === 'fotos') {
    const pastaFoto = serviceId ? `fotos-os/${String(serviceId)}` : `fotos-os/avulsas-${new Date().toISOString().slice(0, 10)}`;
    objPath = `${pastaFoto}/${crypto.randomUUID()}-${limpo}.${ext}`;
  } else if (pasta === 'aparelho') {
    if (!refId) return json(res, 400, { error: 'refId (aparelho) obrigatório' });
    objPath = `fotos-aparelho/${String(refId)}/${crypto.randomUUID()}-${limpo}.${ext}`;
  } else {
    objPath = `os-orcamentos/${new Date().toISOString().slice(0, 10)}/${crypto.randomUUID()}-${limpo}.${ext}`;
  }
  const bytes = Buffer.from(b64.split(',')[1], 'base64');

  const r = await fetch(`${SUPABASE_URL}/storage/v1/object/documentos-inovar/${objPath}`, {
    method: 'POST',
    headers: {
      apikey: process.env.SUPABASE_SERVICE_ROLE_KEY,
      Authorization: `Bearer ${process.env.SUPABASE_SERVICE_ROLE_KEY}`,
      'Content-Type': mime,
      'x-upsert': 'false'
    },
    body: bytes
  });

  if (!r.ok) {
    const t = await r.text();
    return json(res, 500, { error: 'Falha ao enviar arquivo ao storage', detalhe: t.slice(0, 200) });
  }
  return json(res, 200, { ok: true, path: objPath });
}

async function linkHandler(req, res, caller) {
  const { path: objPath, dias } = req.body || {};
  if (!objPath || String(objPath).includes('..')) return json(res, 400, { error: 'Caminho inválido' });
  if (!/^(os-orcamentos|fotos-os|fotos-aparelho|fotos-perfil)\/[a-zA-Z0-9_ .\/-]+$/.test(String(objPath))) return json(res, 400, { error: 'Caminho de documento não permitido' });
  if (caller.role === 'CLIENTE') return json(res, 403, { error: 'Use os documentos disponibilizados no seu atendimento.' });
  const expiraEm = Math.min(Math.max(Number(dias) || 7, 1), 30);

  const { status, data } = await queryAsService(
    `/storage/v1/object/sign/documentos-inovar/${objPath}`,
    { method: 'POST', body: { expiresIn: expiraEm * 24 * 60 * 60 } }
  );
  if (status !== 200 || !data?.signedURL) {
    return json(res, 404, { error: 'Documento não encontrado ou expirado' });
  }
  const expiraEmSegundos = expiraEm * 24 * 60 * 60;
  const shortUrl = await criarLinkCurto(req, objPath, expiraEmSegundos);
  return json(res, 200, {
    ok: true,
    // Usado pelo envio automático do WhatsApp, que precisa baixar o arquivo diretamente.
    url: SUPABASE_URL + '/storage/v1' + data.signedURL,
    // Usado pela abertura manual do WhatsApp: curto, legível e com a mesma validade.
    shortUrl,
    expiraEmDias: expiraEm
  });
}

// Lista fotos de uma OS (prefixo fotos-os/{serviceId}/) com links assinados
async function fotosHandler(req, res, caller) {
  const { serviceId } = req.body || {};
  if (!serviceId) return json(res, 400, { error: 'serviceId obrigatório' });
  const prefix = 'fotos-os/' + serviceId + '/';
  // CLIENTE so ve fotos de servicos do proprio cadastro
  if (caller.role === 'CLIENTE') {
    const svc = await queryAsService(`/rest/v1/services?id=eq.${serviceId}&select=cliente_id`, { method: 'GET' });
    const clienteId = svc?.data?.[0]?.cliente_id;
    const meuCust = await queryAsService(`/rest/v1/customers?profile_id=eq.${caller.userId}&select=id`, { method: 'GET' });
    const meuId = Array.isArray(meuCust?.data) && meuCust.data[0]?.id;
    if (!clienteId || clienteId !== meuId) {
      return json(res, 403, { error: 'Estas fotos pertencem a outro cliente' });
    }
  }
  const SRK = process.env.SUPABASE_SERVICE_ROLE_KEY;

  const lr = await fetch(`${SUPABASE_URL}/storage/v1/object/list/documentos-inovar`, {
    method: 'POST',
    headers: { apikey: SRK, Authorization: `Bearer ${SRK}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ prefix, limit: 60, sortBy: { column: 'created_at', order: 'desc' } })
  });
  const objs = await lr.json().catch(() => []);
  const fotos = [];
  for (const o of (Array.isArray(objs) ? objs : [])) {
    // Storage pode retornar o name relativo ao prefixo ou o caminho completo — normaliza
    const fullPath = o.name.startsWith(prefix) ? o.name : prefix + o.name;
    const nomeRel = fullPath.startsWith(prefix) ? fullPath.slice(prefix.length) : fullPath;
    const sr = await fetch(`${SUPABASE_URL}/storage/v1/object/sign/documentos-inovar/${fullPath}`, {
      method: 'POST',
      headers: { apikey: SRK, Authorization: `Bearer ${SRK}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ expiresIn: 604800 })
    });
    const sj = await sr.json().catch(() => ({}));
    if (sj?.signedURL) {
      fotos.push({ nome: nomeRel, caminho: fullPath, url: SUPABASE_URL + '/storage/v1' + sj.signedURL, criadoEm: o.created_at || null });
    }
  }
  return json(res, 200, { ok: true, fotos });
}

// Lista fotos de um aparelho (prefixo fotos-aparelho/{id}/) com links assinados
async function fotosAparelhoHandler(req, res, caller) {
  const { aparelhoId } = req.body || {};
  if (!aparelhoId) return json(res, 400, { error: 'aparelhoId obrigatório' });
  if (!/^[a-f0-9-]{36}$/i.test(String(aparelhoId))) return json(res, 400, { error: 'Aparelho inválido' });
  if (caller.role === 'CLIENTE') {
    const visible = await queryAsUser(caller.token, `/rest/v1/air_conditioners?id=eq.${aparelhoId}&select=id`);
    if (visible.status !== 200 || !visible.data?.length) return json(res, 403, { error: 'Aparelho não disponível para esta conta' });
  }
  const prefix = 'fotos-aparelho/' + aparelhoId + '/';
  const SRK = process.env.SUPABASE_SERVICE_ROLE_KEY;
  const lr = await fetch(`${SUPABASE_URL}/storage/v1/object/list/documentos-inovar`, {
    method: 'POST',
    headers: { apikey: SRK, Authorization: `Bearer ${SRK}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ prefix, limit: 100, sortBy: { column: 'created_at', order: 'desc' } })
  });
  const objs = await lr.json().catch(() => []);
  const fotos = [];
  for (const o of (Array.isArray(objs) ? objs : [])) {
    if (!o.name || o.name.includes('/')) continue;
    const fullPath = prefix + o.name;
    const sr = await fetch(`${SUPABASE_URL}/storage/v1/object/sign/documentos-inovar/${fullPath}`, {
      method: 'POST',
      headers: { apikey: SRK, Authorization: `Bearer ${SRK}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ expiresIn: 604800 })
    });
    const sj = await sr.json().catch(() => ({}));
    if (sj?.signedURL) fotos.push({ nome: o.name, caminho: fullPath, url: SUPABASE_URL + '/storage/v1' + sj.signedURL, criadoEm: o.created_at || null });
  }
  return json(res, 200, { ok: true, fotos });
}

async function excluirFotoHandler(req, res, caller) {
  if (caller.role === 'CLIENTE') return json(res, 403, { error: 'Somente a equipe Inovar exclui fotos' });
  const { path: fotoPath } = req.body || {};
  if (!fotoPath || String(fotoPath).includes('..')) return json(res, 400, { error: 'Caminho inválido' });
  if (!/^fotos-(os|aparelho)\/[a-zA-Z0-9_ .\/-]+$/.test(String(fotoPath))) return json(res, 400, { error: 'Caminho de foto não permitido' });
  const d = await fetch(`${SUPABASE_URL}/storage/v1/object/documentos-inovar/${fotoPath}`, {
    method: 'DELETE',
    headers: { apikey: process.env.SUPABASE_SERVICE_ROLE_KEY, Authorization: `Bearer ${process.env.SUPABASE_SERVICE_ROLE_KEY}` }
  });
  return json(res, 200, { ok: d.status === 200 || d.status === 404 });
}

// ---- Foto de perfil (qualquer usuário autenticado gerencia a PRÓPRIA) ----
const FOTO_PERFIL_PATH = (userId) => `fotos-perfil/${userId}.jpg`;

async function fotoPerfilUploadHandler(req, res, caller) {
  const { base64 } = req.body || {};
  const b64 = String(base64 || '');
  const mImg = b64.match(/^data:image\/(png|jpeg|jpg|webp);base64,/);
  if (!mImg) return json(res, 400, { error: 'Envie uma imagem (PNG/JPEG/WebP) em base64' });
  // imagem já comprimida no cliente (~320px); teto duro por segurança
  if (b64.length > 900_000) return json(res, 413, { error: 'Foto muito grande (máx. ~650KB)' });
  const objPath = FOTO_PERFIL_PATH(caller.userId);
  const bytes = Buffer.from(b64.split(',')[1], 'base64');
  const r = await fetch(`${SUPABASE_URL}/storage/v1/object/documentos-inovar/${objPath}`, {
    method: 'POST',
    headers: {
      apikey: process.env.SUPABASE_SERVICE_ROLE_KEY,
      Authorization: `Bearer ${process.env.SUPABASE_SERVICE_ROLE_KEY}`,
      'Content-Type': 'image/jpeg',
      'x-upsert': 'true'
    },
    body: bytes
  });
  if (!r.ok) {
    const t = await r.text();
    return json(res, 500, { error: 'Falha ao salvar a foto', detalhe: t.slice(0, 200) });
  }
  return json(res, 200, { ok: true, path: objPath });
}

async function fotoPerfilUrlHandler(req, res, caller) {
  const SRK = process.env.SUPABASE_SERVICE_ROLE_KEY;
  const objPath = FOTO_PERFIL_PATH(caller.userId);
  // confirma existência antes de assinar (sign não valida existência)
  const lr = await fetch(`${SUPABASE_URL}/storage/v1/object/list/documentos-inovar`, {
    method: 'POST',
    headers: { apikey: SRK, Authorization: `Bearer ${SRK}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ prefix: 'fotos-perfil/', limit: 200 })
  });
  const objs = await lr.json().catch(() => []);
  const existe = Array.isArray(objs) && objs.some((o) => o.name === objPath || o.name === caller.userId + '.jpg');
  if (!existe) return json(res, 200, { ok: false, url: null });
  const sr = await fetch(`${SUPABASE_URL}/storage/v1/object/sign/documentos-inovar/${objPath}`, {
    method: 'POST',
    headers: { apikey: SRK, Authorization: `Bearer ${SRK}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ expiresIn: 604800 })
  });
  const sj = await sr.json().catch(() => ({}));
  if (!sj?.signedURL) return json(res, 200, { ok: false, url: null });
  return json(res, 200, { ok: true, url: SUPABASE_URL + '/storage/v1' + sj.signedURL });
}

async function fotoPerfilRemoverHandler(req, res, caller) {
  const r = await fetch(`${SUPABASE_URL}/storage/v1/object/documentos-inovar/${FOTO_PERFIL_PATH(caller.userId)}`, {
    method: 'DELETE',
    headers: { apikey: process.env.SUPABASE_SERVICE_ROLE_KEY, Authorization: `Bearer ${process.env.SUPABASE_SERVICE_ROLE_KEY}` }
  });
  return json(res, 200, { ok: r.status === 200 || r.status === 404 });
}

export default async function handler(req, res) {
  if (req.method !== 'POST') return json(res, 405, { error: 'Método não permitido' });

  const { acao } = req.body || {};
  if (!['upload', 'link', 'fotos', 'excluirfoto', 'fotoperfil', 'fotoperfil-url', 'fotoperfil-remover', 'fotos-aparelho'].includes(acao)) return json(res, 400, { error: 'Ação inválida' });

  const caller = await authCaller(req);
  if (!caller) return json(res, 401, { error: 'Não autenticado' });

  if (acao === 'upload') return uploadHandler(req, res, caller);
  if (acao === 'fotos') return fotosHandler(req, res, caller);
  if (acao === 'fotos-aparelho') return fotosAparelhoHandler(req, res, caller);
  if (acao === 'excluirfoto') return excluirFotoHandler(req, res, caller);
  if (acao === 'fotoperfil') return fotoPerfilUploadHandler(req, res, caller);
  if (acao === 'fotoperfil-url') return fotoPerfilUrlHandler(req, res, caller);
  if (acao === 'fotoperfil-remover') return fotoPerfilRemoverHandler(req, res, caller);
  return linkHandler(req, res, caller);
}
