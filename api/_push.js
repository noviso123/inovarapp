// Web Push padrão do navegador: sem SDK proprietário e compatível com PWA
// instalado em Android, desktop e iOS moderno. Inscrições ficam privadas no
// Storage e são separadas por usuário/dispositivo.
import crypto from 'node:crypto';
import { SUPABASE_URL } from './_supabase.js';

const BUCKET = 'documentos-inovar';
const SRK = () => process.env.SUPABASE_SERVICE_ROLE_KEY;
const b64url = (data) => Buffer.from(data).toString('base64url');
const from64url = (value) => Buffer.from(String(value), 'base64url');
const storageHeaders = () => ({ apikey: SRK(), Authorization: `Bearer ${SRK()}`, 'Content-Type': 'application/json' });
const endpointId = endpoint => crypto.createHash('sha256').update(endpoint).digest('hex');
const pathOf = (userId, endpoint) => `push/${userId}/${endpointId(endpoint)}.json`;

function hkdf(ikm, salt, info, length) {
  const prk = crypto.createHmac('sha256', salt).update(ikm).digest();
  let previous = Buffer.alloc(0); let output = Buffer.alloc(0); let index = 0;
  while (output.length < length) {
    previous = crypto.createHmac('sha256', prk).update(Buffer.concat([previous, Buffer.from(info), Buffer.from([++index])])).digest();
    output = Buffer.concat([output, previous]);
  }
  return output.subarray(0, length);
}

function derToJose(signature) {
  let offset = 3;
  if (signature[1] & 0x80) offset += signature[1] & 0x7f;
  if (signature[offset] !== 0x02) throw new Error('Assinatura VAPID inválida.');
  const rLength = signature[offset + 1]; const r = signature.subarray(offset + 2, offset + 2 + rLength);
  offset += 2 + rLength;
  const sLength = signature[offset + 1]; const s = signature.subarray(offset + 2, offset + 2 + sLength);
  const normalize = (part) => {
    const raw = part[0] === 0 ? part.subarray(1) : part;
    return raw.length >= 32 ? raw.subarray(raw.length - 32) : Buffer.concat([Buffer.alloc(32 - raw.length), raw]);
  };
  return Buffer.concat([normalize(r), normalize(s)]);
}

function vapidAuthorization(endpoint) {
  const publicKey = process.env.VAPID_PUBLIC_KEY;
  const privateKey = process.env.VAPID_PRIVATE_KEY;
  if (!publicKey || !privateKey) throw new Error('Notificações ainda não estão configuradas no servidor.');
  const curve = crypto.createECDH('prime256v1'); curve.setPrivateKey(from64url(privateKey));
  const publicRaw = curve.getPublicKey();
  const header = b64url(JSON.stringify({ typ: 'JWT', alg: 'ES256' }));
  const aud = new URL(endpoint).origin;
  const payload = b64url(JSON.stringify({ aud, exp: Math.floor(Date.now() / 1000) + 12 * 60 * 60, sub: process.env.VAPID_SUBJECT || 'mailto:contato@inovarapp.vercel.app' }));
  const point = publicRaw.subarray(1);
  const key = crypto.createPrivateKey({ key: { kty: 'EC', crv: 'P-256', d: privateKey, x: b64url(point.subarray(0, 32)), y: b64url(point.subarray(32, 64)) }, format: 'jwk' });
  const signature = derToJose(crypto.sign('sha256', Buffer.from(`${header}.${payload}`), key));
  return `vapid t=${header}.${payload}.${b64url(signature)}, k=${publicKey}`;
}

function encryptedPayload(subscription, payload) {
  const clientPublic = from64url(subscription.keys.p256dh);
  const auth = from64url(subscription.keys.auth);
  const server = crypto.createECDH('prime256v1'); server.generateKeys();
  const serverPublic = server.getPublicKey();
  const shared = server.computeSecret(clientPublic);
  const info = Buffer.concat([Buffer.from('WebPush: info\0'), clientPublic, serverPublic]);
  const ikm = hkdf(shared, auth, info, 32);
  const salt = crypto.randomBytes(16);
  const cek = hkdf(ikm, salt, 'Content-Encoding: aes128gcm\0', 16);
  const nonce = hkdf(ikm, salt, 'Content-Encoding: nonce\0', 12);
  const cipher = crypto.createCipheriv('aes-128-gcm', cek, nonce);
  const encrypted = Buffer.concat([cipher.update(Buffer.concat([Buffer.from(JSON.stringify(payload)), Buffer.from([2])])), cipher.final()]);
  const tag = cipher.getAuthTag();
  const recordSize = Buffer.alloc(4); recordSize.writeUInt32BE(4096);
  return Buffer.concat([salt, recordSize, Buffer.from([serverPublic.length]), serverPublic, encrypted, tag]);
}

export async function savePushSubscription(userId, subscription) {
  if (!subscription?.endpoint || !subscription?.keys?.p256dh || !subscription?.keys?.auth) throw new Error('Inscrição de notificação inválida.');
  const r = await fetch(`${SUPABASE_URL}/storage/v1/object/${BUCKET}/${pathOf(userId, subscription.endpoint)}`, {
    method: 'POST', headers: { ...storageHeaders(), 'x-upsert': 'true' }, body: JSON.stringify({ subscription, updatedAt: new Date().toISOString() })
  });
  if (!r.ok) throw new Error('Não foi possível registrar este dispositivo para notificações.');
}

export async function sendPushToUser(userId, payload) {
  if (!userId || !process.env.VAPID_PUBLIC_KEY || !process.env.VAPID_PRIVATE_KEY) return { sent: 0, skipped: true };
  // Go-app's worker routes clicks with `path`; the legacy worker consumes
  // `url`. Sending both keeps notification behavior intact during migration.
  const notification = { ...payload, path: payload.path || payload.url || '/' };
  const prefix = `push/${userId}/`;
  const list = await fetch(`${SUPABASE_URL}/storage/v1/object/list/${BUCKET}`, { method: 'POST', headers: storageHeaders(), body: JSON.stringify({ prefix, limit: 100 }) });
  const objects = await list.json().catch(() => []); let sent = 0;
  for (const object of (Array.isArray(objects) ? objects : [])) {
    const name = object.name.includes('/') ? object.name : prefix + object.name;
    const saved = await fetch(`${SUPABASE_URL}/storage/v1/object/${BUCKET}/${name}`, { headers: storageHeaders() });
    const data = await saved.json().catch(() => null); const subscription = data?.subscription;
    if (!subscription) continue;
    try {
      const r = await fetch(subscription.endpoint, { method: 'POST', headers: { TTL: '86400', Urgency: 'high', 'Content-Encoding': 'aes128gcm', Authorization: vapidAuthorization(subscription.endpoint) }, body: encryptedPayload(subscription, notification) });
      if (r.status === 404 || r.status === 410) await fetch(`${SUPABASE_URL}/storage/v1/object/${BUCKET}/${name}`, { method: 'DELETE', headers: storageHeaders() });
      else if (r.ok) sent++;
    } catch { /* um dispositivo indisponível não impede os demais */ }
  }
  return { sent };
}
