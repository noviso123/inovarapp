import { createCipheriv, createDecipheriv, createHash, randomBytes } from 'node:crypto';
import { SUPABASE_URL, queryAsUser } from './_supabase.js';

const key = () => createHash('sha256').update('inovar-google-v1:' + process.env.SUPABASE_SERVICE_ROLE_KEY).digest();
const objectPath = userId => `config/google/${userId}.json`;
const headers = () => ({ apikey: process.env.SUPABASE_SERVICE_ROLE_KEY, Authorization: `Bearer ${process.env.SUPABASE_SERVICE_ROLE_KEY}` });
export function seal(data) {
  const iv = randomBytes(12); const cipher = createCipheriv('aes-256-gcm', key(), iv);
  const encrypted = Buffer.concat([cipher.update(JSON.stringify(data), 'utf8'), cipher.final()]);
  return { iv: iv.toString('base64'), tag: cipher.getAuthTag().toString('base64'), data: encrypted.toString('base64') };
}
export function unseal(envelope) {
  const decipher = createDecipheriv('aes-256-gcm', key(), Buffer.from(envelope.iv, 'base64'));
  decipher.setAuthTag(Buffer.from(envelope.tag, 'base64'));
  return JSON.parse(Buffer.concat([decipher.update(Buffer.from(envelope.data, 'base64')), decipher.final()]).toString('utf8'));
}
export async function readConnection(userId) {
  const r = await fetch(`${SUPABASE_URL}/storage/v1/object/documentos-inovar/${objectPath(userId)}`, { headers: headers() });
  if (r.status === 400 || r.status === 404) return null;
  if (!r.ok) throw new Error('Falha ao consultar a conexão Google.');
  return unseal(await r.json());
}
export async function saveConnection(userId, connection) {
  const r = await fetch(`${SUPABASE_URL}/storage/v1/object/documentos-inovar/${objectPath(userId)}`, {
    method: 'POST', headers: { ...headers(), 'Content-Type': 'application/json', 'x-upsert': 'true' }, body: JSON.stringify(seal(connection))
  });
  if (!r.ok) throw new Error('Não foi possível salvar a conexão Google.');
}
export async function disconnect(userId) {
  // Stop future sync; existing events remain in the user's Google calendar.
  await saveConnection(userId, { disconnected: true });
}
export const calendarEventId = id => 'inovar' + createHash('sha256').update(String(id)).digest('hex').slice(0, 40);
export function eventPayload(service, customer = {}) {
  const day = String(service.data_agendamento || '').slice(0, 10);
  const time = String(service.hora_agendamento || '09:00').slice(0, 5);
  if (!/^\d{4}-\d{2}-\d{2}$/.test(day) || !/^([01]\d|2[0-3]):[0-5]\d$/.test(time)) throw new Error('Data ou horário de agendamento inválido.');
  const start = new Date(`${day}T${time}:00-03:00`);
  if (!Number.isFinite(start.getTime())) throw new Error('Data de agendamento inválida.');
  return {
    summary: `Inovar: ${service.descricao || service.tipo || 'Atendimento'} — ${customer.nome || 'Cliente'}`,
    description: `OS ${service.id}\nSituação: ${service.status}\nContato: ${customer.whatsapp || ''}`,
    location: [customer.endereco, customer.bairro, customer.cidade].filter(Boolean).join(', '),
    start: { dateTime: start.toISOString(), timeZone: 'America/Sao_Paulo' },
    end: { dateTime: new Date(start.getTime() + 7200000).toISOString(), timeZone: 'America/Sao_Paulo' },
    extendedProperties: { private: { inovarServiceId: service.id } }
  };
}
async function accessToken(connection) {
  if (connection.expiresAt > Date.now() + 60000) return connection.accessToken;
  if (!connection.refreshToken || !process.env.GOOGLE_CLIENT_ID || !process.env.GOOGLE_CLIENT_SECRET) throw new Error('Reconecte o Google Agenda para renovar a autorização.');
  const r = await fetch('https://oauth2.googleapis.com/token', { method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded' }, body: new URLSearchParams({ grant_type: 'refresh_token', refresh_token: connection.refreshToken, client_id: process.env.GOOGLE_CLIENT_ID, client_secret: process.env.GOOGLE_CLIENT_SECRET }) });
  const j = await r.json();
  if (!r.ok || !j.access_token) throw new Error('O Google expirou ou revogou a autorização. Reconecte a agenda.');
  connection.accessToken = j.access_token; connection.expiresAt = Date.now() + j.expires_in * 1000;
  return connection.accessToken;
}
export async function syncCalendar(caller, connection) {
  const token = await accessToken(connection);
  const services = await queryAsUser(caller.token, '/rest/v1/services?select=id,cliente_id,tipo,descricao,status,data_agendamento,hora_agendamento&limit=1000');
  const customers = await queryAsUser(caller.token, '/rest/v1/customers?select=id,nome,endereco,bairro,cidade,whatsapp&limit=1000');
  if (services.status !== 200 || customers.status !== 200 || !Array.isArray(services.data) || !Array.isArray(customers.data)) throw new Error('Não foi possível ler a agenda. Nenhum evento foi removido.');
  if (services.data.length >= 1000 || customers.data.length >= 1000) throw new Error('A agenda excedeu o limite de sincronização. Nenhum evento foi removido.');
  const clients = new Map(customers.data.map(c => [c.id, c]));
  const previous = connection.events || {}; const next = { ...previous }; let changed = 0;
  const base = 'https://www.googleapis.com/calendar/v3/calendars/primary/events';
  const google = async (url, method, body) => {
    const r = await fetch(url, { method, headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }, body: body ? JSON.stringify(body) : undefined });
    if (r.status === 401 || r.status === 403) throw new Error('Reconecte a agenda e autorize o acesso ao Google Calendar.');
    return r;
  };
  const active = new Set();
  for (const s of services.data) {
    if (!['AGENDADO', 'EM_ANDAMENTO'].includes(s.status) || !s.data_agendamento) continue;
    active.add(s.id);
    const payload = eventPayload(s, clients.get(s.cliente_id));
    const fingerprint = createHash('sha256').update(JSON.stringify(payload)).digest('hex');
    if (previous[s.id] === fingerprint) continue;
    const id = calendarEventId(s.id);
    let r = await google(`${base}/${id}`, 'PUT', payload);
    if (r.status === 404) {
      r = await google(base, 'POST', { ...payload, id });
      if (r.status === 409) r = await google(`${base}/${id}`, 'PUT', payload);
    }
    if (!r.ok) throw new Error('O Google não confirmou a atualização de um agendamento. Tente sincronizar novamente.');
    next[s.id] = fingerprint; changed++;
  }
  for (const id of Object.keys(previous)) {
    if (active.has(id)) continue;
    const r = await google(`${base}/${calendarEventId(id)}`, 'DELETE');
    if (!r.ok && r.status !== 404 && r.status !== 410) throw new Error('Não foi possível remover um evento encerrado. Tente sincronizar novamente.');
    delete next[id]; changed++;
  }
  connection.events = next; connection.lastSync = new Date().toISOString();
  await saveConnection(caller.userId, connection);
  return { changed, lastSync: connection.lastSync };
}
