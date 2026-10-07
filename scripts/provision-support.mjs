import fs from 'node:fs/promises';

const managementToken = process.env.SUPABASE_MANAGEMENT_TOKEN;
if (!managementToken) throw new Error('SUPABASE_MANAGEMENT_TOKEN ausente');
const project = 'ycpswioserctavijhnre';
const api = `https://api.supabase.com/v1/projects/${project}`;
const mgmtHeaders = { Authorization: `Bearer ${managementToken}`, 'Content-Type': 'application/json' };

async function responseJson(response) {
  const result = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(result.message || result.error || `HTTP ${response.status}`);
  return result;
}

// Aplica migrações idempotentes pendentes pelo canal oficial de gerenciamento.
for (const filename of [
  'supabase/migrations/20260908_service_lifecycle_dates.sql',
  'supabase/migrations/20260912_lifecycle_e_acesso_tecnico.sql'
]) {
  const query = await fs.readFile(filename, 'utf8');
  await responseJson(await fetch(`${api}/database/query`, {
    method: 'POST', headers: mgmtHeaders, body: JSON.stringify({ query })
  }));
  console.log(`MIGRATION_OK ${filename}`);
}

const keys = await responseJson(await fetch(`${api}/api-keys`, { headers: mgmtHeaders }));
const serviceKey = keys.find((entry) => entry.name === 'service_role' || entry.type === 'service_role')?.api_key;
if (!serviceKey) throw new Error('service_role não encontrada');
const base = `https://${project}.supabase.co`;
const adminHeaders = { apikey: serviceKey, Authorization: `Bearer ${serviceKey}`, 'Content-Type': 'application/json' };
const email = String(process.env.SUPPORT_ADMIN_EMAIL || '').trim().toLowerCase();
const password = String(process.env.SUPPORT_ADMIN_PASSWORD || '');
if (!email || password.length < 6) throw new Error('Credenciais iniciais inválidas');

const usersResult = await responseJson(await fetch(`${base}/auth/v1/admin/users?page=1&per_page=1000`, { headers: adminHeaders }));
let user = (usersResult.users || []).find((entry) => String(entry.email).toLowerCase() === email);
const metadata = { ...(user?.user_metadata || {}), nome: 'Jhonatan Satiro', must_change_password: true };
if (user) {
  user = await responseJson(await fetch(`${base}/auth/v1/admin/users/${user.id}`, {
    method: 'PUT', headers: adminHeaders, body: JSON.stringify({ password, email_confirm: true, user_metadata: metadata })
  }));
  console.log('SUPPORT_ADMIN_UPDATED');
} else {
  user = await responseJson(await fetch(`${base}/auth/v1/admin/users`, {
    method: 'POST', headers: adminHeaders, body: JSON.stringify({ email, password, email_confirm: true, user_metadata: metadata })
  }));
  console.log('SUPPORT_ADMIN_CREATED');
}

await responseJson(await fetch(`${base}/rest/v1/profiles?on_conflict=id`, {
  method: 'POST',
  headers: { ...adminHeaders, Prefer: 'resolution=merge-duplicates,return=representation' },
  body: JSON.stringify({ id: user.id, nome: 'Jhonatan Satiro', email, tipo: 'ADMIN' })
}));
console.log('SUPPORT_PROFILE_ADMIN_OK');
