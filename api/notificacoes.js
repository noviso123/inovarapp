import { authCaller, json } from './_supabase.js';
import { savePushSubscription, sendPushToUser } from './_push.js';

export default async function handler(req, res) {
  if (req.method !== 'POST') return json(res, 405, { error: 'Método não permitido' });
  const caller = await authCaller(req);
  if (!caller) return json(res, 401, { error: 'Entre na sua conta para ativar notificações.' });
  const acao = req.body?.acao;
  try {
    if (acao === 'inscrever') {
      await savePushSubscription(caller.userId, req.body.subscription);
      return json(res, 200, { ok: true });
    }
    // Notifica todas as instalações da própria conta. O cliente nunca pode
    // escolher outro usuário como destino por esta rota.
    if (acao === 'enviar') {
      const titulo = String(req.body?.titulo || 'InovarApp').slice(0, 120);
      const texto = String(req.body?.texto || 'Há uma atualização no aplicativo.').slice(0, 400);
      const url = String(req.body?.url || '/').startsWith('/') ? String(req.body?.url || '/') : '/';
      const result = await sendPushToUser(caller.userId, { title: titulo, body: texto, url, tag: `inovar-${Date.now()}` });
      return json(res, 200, { ok: true, ...result });
    }
    return json(res, 400, { error: 'Ação inválida.' });
  } catch (error) {
    return json(res, 400, { error: error.message || 'Não foi possível ativar as notificações.' });
  }
}
