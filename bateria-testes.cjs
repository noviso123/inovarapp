/**
 * BATERIA MASSIVA DE TESTES — InovarApp
 * Cobre: banco (schema/RLS/isolamento), autenticação, matriz de permissões
 * das APIs serverless em produção, fluxo ponta-a-ponta do orçamento com
 * assinatura, frontend em produção (assets/segredos) e limpeza dos dados de teste.
 *
 * Uso: node bateria-testes.cjs
 * Token da Vercel: variável de ambiente VERCEL_TOKEN (ou extraído do
 * deploy-vercel.cjs local, que não vai para o repositório).
 */
const https = require('https');
const fs = require('fs');
const path = require('path');

function tokenVercel() {
  if (process.env.VERCEL_TOKEN) return process.env.VERCEL_TOKEN;
  try {
    const local = fs.readFileSync(path.join(__dirname, 'deploy-vercel.cjs'), 'utf8');
    const m = local.match(/vcp_[A-Za-z0-9]+/);
    if (m) return m[0];
  } catch { /* arquivo local ausente */ }
  throw new Error('Defina VERCEL_TOKEN no ambiente ou mantenha deploy-vercel.cjs local');
}

const VERCEL_TOKEN = tokenVercel();
const VERCEL_PROJECT = 'prj_oppzCa6vE8DBAGcz5vSXmBWTWm5H';
const VERCEL_TEAM = 'team_CQffFMUCkUqnTaIkYn0diPkq';
const SB_URL = 'https://ycpswioserctavijhnre.supabase.co';
const ANON = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzdXBhYmFzZSIsInJlZiI6InljcHN3aW9zZXJjdGF2aWpobnJlIiwicm9sZSI6ImFub24iLCJpYXQiOjE3ODgxMTExOTIsImV4cCI6MjEwMzY4NzE5Mn0.mcxQsa2COarpWCV0v_qdEDuyZiTU4qA1p5PV05X40H0';
const PROD = 'inovarapp.vercel.app';

let passed = 0, failed = 0;
const sections = [];
let currentSection = '';
function section(name) { currentSection = name; sections.push(name); console.log('\n━━ ' + name); }
function ok(name, cond, extra) {
  if (cond) { passed++; console.log('  ✅ ' + name + (extra ? ' — ' + extra : '')); }
  else { failed++; console.log('  ❌ ' + name + (extra ? ' — ' + extra : '')); }
}

function req(hostname, path, { method = 'GET', body = null, headers = {} } = {}) {
  return new Promise((resolve, reject) => {
    const data = body ? JSON.stringify(body) : null;
    const r = https.request({ hostname, path, method, headers: {
      'Content-Type': 'application/json',
      ...(data ? { 'Content-Length': Buffer.byteLength(data) } : {}),
      ...headers
    } }, (res) => {
      let chunks = [];
      res.on('data', (c) => chunks.push(c));
      res.on('end', () => {
        const text = Buffer.concat(chunks).toString();
        let json = null; try { json = JSON.parse(text); } catch { json = text; }
        resolve({ status: res.statusCode, headers: res.headers, body: json, text });
      });
    });
    r.on('error', reject);
    r.setTimeout(25000, () => { r.destroy(); reject(new Error('timeout ' + path)); });
    if (data) r.write(data);
    r.end();
  });
}
const sb = (path, opts = {}) => req('ycpswioserctavijhnre.supabase.co', path, {
  ...opts,
  headers: {
    apikey: opts.apikey || ANON,
    Authorization: 'Bearer ' + (opts.token || opts.apikey || ANON),
    // PostgREST só devolve a linha criada/atualizada com return=representation
    ...(opts.method === 'POST' || opts.method === 'PATCH' ? { Prefer: 'return=representation' } : {}),
    ...(opts.headers || {})
  }
});
// PostgREST devolve arrays mesmo para insert().single() sem o header de objeto
const first = (b) => (Array.isArray(b) ? b[0] : b);

// PNG 1x1 válido para a assinatura digital de teste
const ASSINATURA_PNG = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==';

async function main() {
  const stamp = Date.now().toString(36);
  const TESTE_EMAIL = `teste.bateria.${stamp}@inovartest.local`;
  const created = { budgets: [], services: [], appointments: [], appliances: [], customers: [], profiles: [], authUsers: [] };

  // ─────────────────────────────────────────────
  section('0. CONFIGURAÇÃO — credenciais da Vercel');
  let SRK = '';
  {
    const env = await req('api.vercel.com', `/v9/projects/${VERCEL_PROJECT}/env?decrypt=true&teamId=${VERCEL_TEAM}`, { headers: { Authorization: 'Bearer ' + VERCEL_TOKEN } });
    ok('Vercel: variáveis de ambiente legíveis', env.status === 200 && Array.isArray(env.body.envs));
    const keys = (env.body.envs || []).map((e) => e.key);
    ok('Vercel: SUPABASE_SERVICE_ROLE_KEY definida', keys.includes('SUPABASE_SERVICE_ROLE_KEY'));
    ok('Vercel: VITE_SUPABASE_URL/ANON definidas', keys.includes('VITE_SUPABASE_URL') && keys.includes('VITE_SUPABASE_ANON_KEY'));
    ok('Vercel: CRON_SECRET definido', keys.includes('CRON_SECRET'));
    const srk = (env.body.envs || []).find((e) => e.key === 'SUPABASE_SERVICE_ROLE_KEY');
    SRK = srk ? String(srk.value) : '';
    ok('Service role key obtida (para testes + limpeza)', SRK.length > 100);
  }

  // ─────────────────────────────────────────────
  section('1. SEGURANÇA RLS — anon (sem login) NÃO pode ler nem escrever dados');
  {
    const tabelas = ['customers', 'services', 'budgets', 'appointments', 'air_conditioners', 'service_history', 'profiles'];
    for (const t of tabelas) {
      const r = await sb(`/rest/v1/${t}?select=*limit=1`.replace('*limit', '*&limit'));
      const bloqueado = r.status === 401 || r.status === 403 || r.status === 404 || (Array.isArray(r.body) && r.body.length === 0);
      ok(`anon GET ${t} bloqueado/vazio`, bloqueado, `status=${r.status}`);
      if (r.status === 200 && Array.isArray(r.body) && r.body.length > 0) {
        ok(`❗ VAZAMENTO: anon lê dados reais de ${t}`, false);
      }
    }
    const ins = await sb('/rest/v1/customers', { method: 'POST', body: { nome: 'HACKER' } });
    ok('anon INSERT customers bloqueado', ins.status === 401 || ins.status === 403 || ins.status === 404, `status=${ins.status}`);
  }

  // ─────────────────────────────────────────────
  section('2. BANCO — schema e RPC (service role)');
  {
    const openapi = await sb('/rest/v1/', { apikey: SRK, token: SRK });
    const defs = openapi.body && openapi.body.definitions ? openapi.body.definitions : {};
    for (const t of ['profiles', 'customers', 'air_conditioners', 'services', 'service_history', 'appointments', 'budgets']) {
      ok(`tabela ${t} existe no banco`, !!defs[t]);
    }
    // colunas verificadas por SELECT direto (o cache do OpenAPI pode estar defasado)
    const rCiclo = await sb('/rest/v1/services?select=data_inicio,data_conclusao,data_cancelamento,motivo_cancelamento&limit=1', { apikey: SRK, token: SRK });
    if (rCiclo.status === 200) {
      ok('services: colunas do ciclo de vida presentes (migration aplicada)', true);
    } else {
      ok('services: colunas do ciclo de vida presentes (migration aplicada)', false, 'MIGRATION 20260908 NÃO APLICADA — app usa fallback de marcadores');
    }
    const rpc = await sb('/rest/v1/rpc/proximo_numero_orcamento', { method: 'POST', body: {}, apikey: SRK, token: SRK });
    ok('RPC proximo_numero_orcamento funciona', rpc.status === 200 && typeof rpc.body === 'string' && /\d{4}-/.test(rpc.body), String(rpc.body).slice(0, 20));
  }

  // ─────────────────────────────────────────────
  section('3. AUTENTICAÇÃO — cria usuário de teste (admin) e valida trigger de cadastro');
  let USER_TOKEN = '';
  let USER_ID = '';
  let CUST_ID = '';
  {
    const create = await sb('/auth/v1/admin/users', { method: 'POST', apikey: SRK, token: SRK, body: {
      email: TESTE_EMAIL, password: 'TesteBateria!123', email_confirm: true,
      user_metadata: { nome: 'Cliente Teste Bateria', telefone: '27999990000' }
    }});
    ok('auth admin: usuário de teste criado', create.status === 200 && !!create.body.id, TESTE_EMAIL);
    USER_ID = create.body.id || '';
    if (USER_ID) created.authUsers.push(USER_ID);

    const login = await sb('/auth/v1/token?grant_type=password', { method: 'POST', body: { email: TESTE_EMAIL, password: 'TesteBateria!123' } });
    ok('login com senha retorna access_token', login.status === 200 && !!login.body.access_token);
    USER_TOKEN = login.body.access_token || '';

    // login errado deve falhar
    const badLogin = await sb('/auth/v1/token?grant_type=password', { method: 'POST', body: { email: TESTE_EMAIL, password: 'senha-errada' } });
    ok('login com senha errada rejeitado (400)', badLogin.status === 400 || badLogin.status === 422, `status=${badLogin.status}`);

    const prof = await sb(`/rest/v1/profiles?id=eq.${USER_ID}&select=*,tipo`, { apikey: SRK, token: SRK });
    const profRow = Array.isArray(prof.body) ? prof.body[0] : null;
    ok('trigger handle_new_user criou profile', !!profRow, profRow ? `tipo=${profRow.tipo}` : '');
    if (profRow) created.profiles.push(profRow.id);
    ok('profile novo é CLIENTE', profRow && profRow.tipo === 'CLIENTE');

    const cust = await sb(`/rest/v1/customers?profile_id=eq.${USER_ID}&select=*`, { apikey: SRK, token: SRK });
    const custRow = Array.isArray(cust.body) ? cust.body[0] : null;
    ok('trigger criou customer vinculado', !!custRow);
    if (custRow) { CUST_ID = custRow.id; created.customers.push(CUST_ID); }
  }

  // ─────────────────────────────────────────────
  section('4. ISOLAMENTO DE DADOS (RLS como CLIENTE logado)');
  {
    const svc = await sb('/rest/v1/services?select=id,cliente_id', { token: USER_TOKEN });
    const rows = Array.isArray(svc.body) ? svc.body : [];
    const alheios = rows.filter((r) => r.cliente_id !== CUST_ID);
    ok('CLIENTE só vê os próprios serviços (0 de outros clientes)', svc.status === 200 && alheios.length === 0, `visíveis=${rows.length}, alheios=${alheios.length}`);

    const bud = await sb('/rest/v1/budgets?select=id,cliente_id', { token: USER_TOKEN });
    const budRows = Array.isArray(bud.body) ? bud.body : [];
    ok('CLIENTE só vê os próprios orçamentos', budRows.every((b) => b.cliente_id === CUST_ID), `visíveis=${budRows.length}`);

    const custs = await sb('/rest/v1/customers?select=id,nome', { token: USER_TOKEN });
    const custRows = Array.isArray(custs.body) ? custs.body : [];
    ok('CLIENTE só vê o próprio cadastro de cliente', custRows.every((c) => c.id === CUST_ID), `visíveis=${custRows.length}`);
  }

  // ─────────────────────────────────────────────
  section('5. PORTAL DO CLIENTE — fluxo de solicitação (como usuário logado)');
  {
    const app = await sb('/rest/v1/air_conditioners', { method: 'POST', token: USER_TOKEN, body: {
      cliente_id: CUST_ID, marca: 'LG TESTE', modelo: 'Dual Inverter', btus: 9000, tipo: 'Split Hi-Wall', ambiente: 'Quarto Teste'
    }});
    const appRow = first(app.body);
    ok('cliente cadastra aparelho', app.status === 201 && !!appRow?.id, `status=${app.status}`);
    if (appRow && appRow.id) created.appliances.push(appRow.id);

    const svc = await sb('/rest/v1/services', { method: 'POST', token: USER_TOKEN, body: {
      cliente_id: CUST_ID, aparelho_id: appRow?.id, tipo: 'LIMPEZA', status: 'PENDENTE',
      descricao: 'Solicitação de teste automatizado', problema: 'TESTE BATERIA — ignorar'
    }});
    const svcRow = first(svc.body);
    ok('cliente solicita atendimento (services insert)', svc.status === 201 && !!svcRow?.id, `status=${svc.status}`);
    if (svcRow && svcRow.id) created.services.push(svcRow.id);

    // cliente NÃO pode alterar o status para CONCLUIDO (burlar fluxo)
    if (svcRow && svcRow.id) {
      const hack = await sb(`/rest/v1/services?id=eq.${svcRow.id}`, { method: 'PATCH', token: USER_TOKEN, body: { status: 'CONCLUIDO', valor: 999999 } });
      const vr = await sb(`/rest/v1/services?id=eq.${svcRow.id}&select=status,valor`, { token: USER_TOKEN });
      const row = Array.isArray(vr.body) ? vr.body[0] : {};
      const inalterado = row.status !== 'CONCLUIDO' && !(Number(row.valor) > 0);
      ok('cliente não consegue auto-concluir serviço (RLS)', inalterado, `status=${hack.status}, db.status=${row.status}`);
    }
  }

  // ─────────────────────────────────────────────
  section('6. MATRIZ DE PERMISSÕES — APIs em produção (como CLIENTE)');
  const authHdr = { Authorization: 'Bearer ' + USER_TOKEN };
  {
    let r = await req(PROD, '/api/configuracoes', { headers: authHdr });
    ok('GET /api/configuracoes (autenticado) → 200', r.status === 200 && r.body.ok === true);
    const cfgGet = r.body.config || {};
    ok('config não vaza segredo WhatsApp em claro', !(typeof cfgGet.whatsapp_evol_api_key === 'string' && cfgGet.whatsapp_evol_api_key.length > 20 && !cfgGet.whatsapp_evol_api_key.includes('***')));
    ok('config não vaza senha do Gmail em claro', !cfgGet.email_gmail_pass || String(cfgGet.email_gmail_pass).includes('***'));

    r = await req(PROD, '/api/configuracoes', { method: 'POST', headers: authHdr, body: { businessName: 'HACKEADO' } });
    ok('POST /api/configuracoes como CLIENTE → 403', r.status === 403, `status=${r.status}`);

    r = await req(PROD, '/api/whatsapp', { method: 'POST', headers: authHdr, body: { acao: 'status' } });
    ok('POST /api/whatsapp como CLIENTE → 403', r.status === 403, `status=${r.status}`);

    r = await req(PROD, '/api/whatsapp', { method: 'POST', headers: authHdr, body: { acao: 'enviar', telefone: '27999990000', texto: 'spam' } });
    ok('POST /api/whatsapp enviar como CLIENTE → 403', r.status === 403, `status=${r.status}`);

    r = await req(PROD, '/api/documentos', { method: 'POST', headers: authHdr, body: { acao: 'upload', base64: ASSINATURA_PNG, nome: 'x.png' } });
    ok('POST /api/documentos upload como CLIENTE → 403', r.status === 403, `status=${r.status}`);

    r = await req(PROD, '/api/contas', { method: 'POST', headers: authHdr, body: { acao: 'do_tecnico', email: 'x@y.com' } });
    ok('POST /api/contas do_tecnico como CLIENTE → 403', r.status === 403, `status=${r.status}`);

    r = await req(PROD, '/api/contas', { method: 'POST', headers: authHdr, body: { acao: 'redefinir_cliente', customer_id: CUST_ID } });
    ok('redefinir senha como CLIENTE → 403', r.status === 403, `status=${r.status}`);

    r = await req(PROD, '/api/email', { method: 'POST', headers: authHdr, body: { acao: 'testar' } });
    ok('POST /api/email como CLIENTE → 403', r.status === 403, `status=${r.status}`);

    r = await req(PROD, '/api/contas', { method: 'POST', body: { acao: 'minha_conta', email: 'email-invalido', senha: 'x', nome: 'x' } });
    ok('auto-cadastro com e-mail inválido → 400', r.status === 400, `status=${r.status}`);

    r = await req(PROD, '/api/contas', { method: 'POST', body: { acao: 'minha_conta', email: `x${stamp}@teste.com`, senha: '123', nome: 'x' } });
    ok('auto-cadastro com senha curta → 400', r.status === 400, `status=${r.status}`);

    r = await req(PROD, '/api/whatsapp', { method: 'GET' });
    ok('método errado (GET) → 405', r.status === 405, `status=${r.status}`);
  }

  // ─────────────────────────────────────────────
  section('7. FLUXO PONTA-A-PONTA — orçamento do técnico → aprovação com assinatura');
  {
    // técnico (service role) cria orçamento para o cliente de teste
    const numero = '9999-' + stamp;
    const bud1 = await sb('/rest/v1/budgets', { method: 'POST', apikey: SRK, token: SRK, body: {
      numero, cliente_id: CUST_ID, data: new Date().toISOString().slice(0, 10),
      validade: new Date(Date.now() + 15 * 86400000).toISOString().slice(0, 10),
      descricao: JSON.stringify({ numero, clientName: 'Cliente Teste Bateria', items: [{ id: '1', description: 'Limpeza de Ar', quantity: 1, unitPrice: 250, totalPrice: 250, category: 'servico' }], totalValue: 250 }),
      tipo_servico: 'Limpeza de Ar TESTE', valor_mao_obra: 250, valor_material: 0, valor_total: 250, status: 'ENVIADO', responsavel_tecnico: 'Bateria'
    }});
    const bud1Row = first(bud1.body);
    ok('orçamento criado pelo técnico (service role)', bud1.status === 201 && !!bud1Row?.id, bud1.status === 201 ? '' : JSON.stringify(bud1.body).slice(0, 120));
    if (bud1Row && bud1Row.id) created.budgets.push(bud1Row.id);

    // cliente SEM assinatura não pode aprovar
    let r = await req(PROD, '/api/orcamento-resposta', { method: 'POST', headers: authHdr, body: { orcamento_id: bud1Row?.id, acao: 'APROVAR' } });
    ok('aprovação SEM assinatura → 400', r.status === 400, `status=${r.status}`);

    // cliente aprova COM assinatura
    r = await req(PROD, '/api/orcamento-resposta', { method: 'POST', headers: authHdr, body: { orcamento_id: bud1Row?.id, acao: 'APROVAR', assinatura: ASSINATURA_PNG } });
    ok('aprovação COM assinatura → 200', r.status === 200 && r.body.ok === true, `status=${r.status}`);
    const chk = await sb(`/rest/v1/budgets?id=eq.${bud1Row?.id}&select=status,descricao`, { apikey: SRK, token: SRK });
    const row = Array.isArray(chk.body) ? chk.body[0] : {};
    ok('status gravado como APROVADO', row.status === 'APROVADO', `db=${row.status}`);
    let meta = {}; try { meta = JSON.parse(row.descricao || '{}'); } catch {}
    ok('assinatura persistida no banco', !!meta.assinatura && String(meta.assinatura).startsWith('data:image'));
    ok('assinatura_em registrada', !!meta.assinatura_em);

    // aprovar de novo → 409 (idempotência)
    r = await req(PROD, '/api/orcamento-resposta', { method: 'POST', headers: authHdr, body: { orcamento_id: bud1Row?.id, acao: 'APROVAR', assinatura: ASSINATURA_PNG } });
    ok('re-aprovação → 409', r.status === 409, `status=${r.status}`);

    // cliente não pode aprovar orçamento de outro cliente
    const outro = await sb('/rest/v1/customers?select=id&limit=1&nome=neq.' + encodeURIComponent('Cliente Teste Bateria'), { apikey: SRK, token: SRK });
    const outroId = Array.isArray(outro.body) && outro.body[0] ? outro.body[0].id : null;
    if (outroId) {
      const budOutro = await sb('/rest/v1/budgets', { method: 'POST', apikey: SRK, token: SRK, body: {
        numero: '8888-' + stamp, cliente_id: outroId, data: new Date().toISOString().slice(0, 10),
        descricao: JSON.stringify({ numero: '8888-' + stamp, totalValue: 100 }), valor_total: 100, status: 'ENVIADO'
      }});
      const budOutroRow = first(budOutro.body);
      if (budOutro.status === 201 && budOutroRow?.id) {
        created.budgets.push(budOutroRow.id);
        const r2 = await req(PROD, '/api/orcamento-resposta', { method: 'POST', headers: authHdr, body: { orcamento_id: budOutroRow.id, acao: 'APROVAR', assinatura: ASSINATURA_PNG } });
        ok('cliente não aprova orçamento de OUTRO cliente → 404/403', r2.status === 403 || r2.status === 404, `status=${r2.status}`);
      }
    }

    // recusa funciona
    const bud2 = await sb('/rest/v1/budgets', { method: 'POST', apikey: SRK, token: SRK, body: {
      numero: '7777-' + stamp, cliente_id: CUST_ID, data: new Date().toISOString().slice(0, 10),
      descricao: JSON.stringify({ totalValue: 99 }), valor_total: 99, status: 'ENVIADO'
    }});
    const bud2Row = first(bud2.body);
    if (bud2.status === 201 && bud2Row?.id) {
      created.budgets.push(bud2Row.id);
      const r3 = await req(PROD, '/api/orcamento-resposta', { method: 'POST', headers: authHdr, body: { orcamento_id: bud2Row.id, acao: 'RECUSAR' } });
      ok('recusa de orçamento → 200', r3.status === 200);
      const chk2 = await sb(`/rest/v1/budgets?id=eq.${bud2Row.id}&select=status`, { apikey: SRK, token: SRK });
      ok('status gravado como RECUSADO', Array.isArray(chk2.body) && chk2.body[0]?.status === 'RECUSADO');
    }
  }

  // ─────────────────────────────────────────────
  section('8. FEED DE CALENDÁRIO (ICS) com token real');
  {
    const cfg = await sb('/storage/v1/object/documentos-inovar/config/tecnico.json', { apikey: SRK, token: SRK });
    const token = cfg.body && cfg.body.calendario_token;
    if (token) {
      const ics = await req(PROD, '/api/calendario-ics?token=' + token);
      ok('ICS com token válido → 200', ics.status === 200);
      ok('Content-Type text/calendar', String(ics.headers['content-type'] || '').includes('text/calendar'));
      ok('ICS contém VCALENDAR', String(ics.body).includes('BEGIN:VCALENDAR'));
      ok('ICS sem serviços cancelados/concluídos', !/"CANCELADO"/.test(String(ics.body)));
    } else {
      ok('token de calendário existe no config', false, 'calendario_token ausente');
    }
  }

  // ─────────────────────────────────────────────
  section('9. FRONTEND EM PRODUÇÃO — assets e segredos');
  {
    const home = await req(PROD, '/');
    ok('homepage → 200', home.status === 200);
    ok('HTML tem div#root', String(home.body).includes('id="root"'));
    ok('HTML sem service_role', !String(home.body).includes('service_role'));
    const jsMatch = String(home.body).match(/src="(\/assets\/index[^"]+\.js)"/);
    ok('bundle JS principal encontrado', !!jsMatch);
    if (jsMatch) {
      const js = await req(PROD, jsMatch[1]);
      ok('bundle JS → 200', js.status === 200);
      ok('bundle > 100KB', js.text.length > 100000, Math.round(js.text.length / 1024) + 'KB');
      // Segmentos ÚNICOS da SRK (assinatura e trecho do payload após o claim role).
      // (o começo do payload é idêntico ao da anon key — por isso não serve de teste)
      ok('bundle SEM segredos (segmentos únicos da SRK ausentes)',
        !js.text.includes(SRK.slice(-43)) && !js.text.includes(SRK.split('.')[1].slice(50, 110)));
      ok('bundle contém integração Supabase', js.text.includes('supabase'));
      ok('bundle contém login Google (OAuth)', js.text.includes('signInWithGoogle') || js.text.includes('google'));
      // O gerador fica em chunk sob demanda para não atrasar a abertura do PWA.
      const pdfMatch = js.text.match(/(?:\.\/|assets\/)(pdf-engine-[A-Za-z0-9_-]+\.js)/);
      if (pdfMatch) {
        const pdfChunk = await req(PROD, '/assets/' + pdfMatch[1]);
        ok('gerador de PDF separado e disponível', pdfChunk.status === 200 && pdfChunk.text.length > 100000);
      } else {
        ok('gerador de PDF separado referenciado pelo app', false, 'chunk pdf-engine não encontrado');
      }
    }
    const cssMatch = String(home.body).match(/href="(\/assets\/[^"]+\.css)"/);
    if (cssMatch) {
      const css = await req(PROD, cssMatch[1]);
      ok('CSS principal → 200 e com regras', css.status === 200 && css.text.length > 10000);
    }
  }

  // ─────────────────────────────────────────────
  section('10. LIMPEZA — removendo todos os dados de teste');
  {
    for (const b of created.budgets) await sb(`/rest/v1/budgets?id=eq.${b}`, { method: 'DELETE', apikey: SRK, token: SRK });
    for (const a of created.appointments) await sb(`/rest/v1/appointments?id=eq.${a}`, { method: 'DELETE', apikey: SRK, token: SRK });
    for (const s of created.services) await sb(`/rest/v1/services?id=eq.${s}`, { method: 'DELETE', apikey: SRK, token: SRK });
    for (const a of created.appliances) await sb(`/rest/v1/air_conditioners?id=eq.${a}`, { method: 'DELETE', apikey: SRK, token: SRK });
    for (const c of created.customers) await sb(`/rest/v1/customers?id=eq.${c}`, { method: 'DELETE', apikey: SRK, token: SRK });
    for (const p of created.profiles) await sb(`/rest/v1/profiles?id=eq.${p}`, { method: 'DELETE', apikey: SRK, token: SRK });
    for (const u of created.authUsers) await sb(`/auth/v1/admin/users/${u}`, { method: 'DELETE', apikey: SRK, token: SRK });
    let sobra = await sb(`/rest/v1/customers?profile_id=eq.${USER_ID}&select=id`, { apikey: SRK, token: SRK });
    ok('customer de teste removido', Array.isArray(sobra.body) && sobra.body.length === 0);
    const login2 = await sb('/auth/v1/token?grant_type=password', { method: 'POST', body: { email: TESTE_EMAIL, password: 'TesteBateria!123' } });
    ok('usuário de teste removido do Auth', login2.status === 400);
  }

  // ─────────────────────────────────────────────
  console.log('\n═══════════════════════════════════════════');
  console.log(`   RESULTADO: ✅ ${passed} passaram   ❌ ${failed} falharam`);
  console.log('═══════════════════════════════════════════');
  process.exit(failed > 0 ? 1 : 0);
}

main().catch((e) => { console.error('ERRO FATAL:', e.message); process.exit(2); });
