// Utilitário compartilhado das funções serverless da InovarApp.
// A SERVICE ROLE fica apenas no servidor (variável de ambiente da Vercel) e
// nunca é exposta ao navegador. Toda requisição exige JWT válido do chamador
// e a autorização é revalidada via PostgREST com o próprio JWT (RLS).

export const SUPABASE_URL = process.env.VITE_SUPABASE_URL || 'https://ycpswioserctavijhnre.supabase.co';
const ANON_KEY = process.env.VITE_SUPABASE_ANON_KEY;
const SERVICE_ROLE = process.env.SUPABASE_SERVICE_ROLE_KEY;

export function json(res, status, body) {
  res.statusCode = status;
  res.setHeader('Content-Type', 'application/json; charset=utf-8');
  res.end(JSON.stringify(body));
}

export function bearerToken(req) {
  const h = req.headers.authorization || '';
  return h.startsWith('Bearer ') ? h.slice(7) : null;
}

// Decodifica o payload do JWT (sem verificar assinatura — a validação de
// verdade acontece no PostgREST, que recebe o token e aplica RLS).
export function decodeJwtPayload(token) {
  try {
    const part = token.split('.')[1];
    return JSON.parse(Buffer.from(part, 'base64url').toString('utf8'));
  } catch {
    return null;
  }
}

// Consulta o PostgREST como o chamador (RLS aplica-se). Retorna {status, data}.
export async function queryAsUser(token, path, options = {}) {
  const r = await fetch(SUPABASE_URL + path, {
    method: options.method || 'GET',
    headers: {
      apikey: ANON_KEY,
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
      Prefer: options.prefer || 'return=representation'
    },
    body: options.body ? JSON.stringify(options.body) : undefined
  });
  const text = await r.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = text; }
  return { status: r.status, data };
}

// Escrita privilegiada (somente após autorização manual do chamador).
export async function queryAsService(path, options = {}) {
  const r = await fetch(SUPABASE_URL + path, {
    method: options.method || 'GET',
    headers: {
      apikey: SERVICE_ROLE,
      Authorization: `Bearer ${SERVICE_ROLE}`,
      'Content-Type': 'application/json',
      Prefer: options.prefer || 'return=representation'
    },
    body: options.body ? JSON.stringify(options.body) : undefined
  });
  const text = await r.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = text; }
  return { status: r.status, data };
}

// Retorna { role, userId, token } ou null se o chamador não for válido.
export async function authCaller(req) {
  const token = bearerToken(req);
  if (!token || !SERVICE_ROLE) return null;
  const payload = decodeJwtPayload(token);
  if (!payload?.sub || (payload.exp && payload.exp * 1000 < Date.now())) return null;

  // RLS: só retorna a linha se for o próprio perfil (ou admin/tecnico, que
  // enxergam todos os perfis).
  const { status, data } = await queryAsUser(token, `/rest/v1/profiles?id=eq.${payload.sub}&select=id,tipo`);
  if (status !== 200 || !Array.isArray(data) || data.length !== 1) return null;
  return { role: data[0].tipo, userId: payload.sub, token };
}
