import { authCaller, json, queryAsUser } from './_supabase.js';
import { readConnection, saveConnection, disconnect, syncCalendar } from './_google-calendar.js';

export default async function handler(req, res) {
  res.setHeader('Cache-Control', 'no-store');
  if (!['GET', 'POST'].includes(req.method)) return json(res, 405, { error: 'Método não permitido' });
  const caller = await authCaller(req);
  if (!caller) return json(res, 401, { error: 'Não autenticado' });
  if (!['ADMIN', 'TECNICO'].includes(caller.role)) return json(res, 403, { error: 'A sincronização da agenda operacional é exclusiva da equipe.' });
  try {
    let connection = await readConnection(caller.userId);
    if (req.method === 'GET') return json(res, 200, { connected: !!connection && !connection.disconnected, lastSync: connection?.lastSync || null, renewable: !!(connection?.refreshToken && process.env.GOOGLE_CLIENT_ID && process.env.GOOGLE_CLIENT_SECRET) });
    const action = req.body?.action;
    if (action === 'disconnect') { await disconnect(caller.userId); return json(res, 200, { ok: true }); }
    if (action === 'connect') {
      const { providerToken, providerRefreshToken } = req.body || {};
      if (typeof providerToken !== 'string' || providerToken.length > 10000) return json(res, 400, { error: 'Autorização Google inválida.' });
      const [identityResponse, user] = await Promise.all([
        fetch('https://openidconnect.googleapis.com/v1/userinfo', { headers: { Authorization: `Bearer ${providerToken}` } }),
        queryAsUser(caller.token, '/auth/v1/user')
      ]);
      const identity = await identityResponse.json();
      if (!identityResponse.ok || user.status !== 200 || !user.data?.identities?.some(i => i.provider === 'google' && String(i.identity_data?.sub) === String(identity.sub))) return json(res, 403, { error: 'Conecte a conta Google vinculada ao seu login Inovar.' });
      const permission = await fetch('https://www.googleapis.com/calendar/v3/calendars/primary/events?maxResults=1', { headers: { Authorization: `Bearer ${providerToken}` } });
      if (!permission.ok) return json(res, 403, { error: 'Autorize o acesso ao Google Agenda ao conectar.' });
      connection = { accessToken: providerToken, refreshToken: typeof providerRefreshToken === 'string' ? providerRefreshToken : connection?.refreshToken, expiresAt: Date.now() + 3000000, events: connection?.events || {}, lastSync: connection?.lastSync };
      await saveConnection(caller.userId, connection);
      return json(res, 200, { ok: true });
    }
    if (action !== 'sync') return json(res, 400, { error: 'Ação inválida' });
    if (!connection || connection.disconnected) return json(res, 200, { connected: false });
    return json(res, 200, { connected: true, ...(await syncCalendar(caller, connection)) });
  } catch (error) { return json(res, 502, { error: error.message || 'Falha na conexão Google.' }); }
}
