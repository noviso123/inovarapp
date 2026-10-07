// Adaptador do app para o serviço Go único. Segredos existem apenas no servidor.
const ownConfig = () => ({
  url: String(process.env.WHATSAPP_OWN_URL || '').trim().replace(/\/+$/, ''),
  token: String(process.env.WHATSAPP_OWN_TOKEN || '').trim(),
  session: String(process.env.WHATSAPP_OWN_SESSION || 'inovar').trim() || 'inovar'
});

const sessionPath = (config) => `/v1/sessions/${encodeURIComponent(config.session)}`;

function configured(config) {
  return Boolean(config.url && config.token && config.session);
}

async function request(config, path, options = {}) {
  const response = await fetch(`${config.url}${path}`, {
    ...options,
    headers: { Authorization: `Bearer ${config.token}`, ...(options.headers || {}) }
  });
  const contentType = response.headers.get('content-type') || '';
  let body;
  if (contentType.includes('application/json')) body = await response.json().catch(() => ({}));
  else if (contentType.includes('image/')) body = Buffer.from(await response.arrayBuffer());
  else body = await response.text().catch(() => '');
  return { response, body };
}

async function readSession(config) {
  return request(config, sessionPath(config));
}

// Session creation is idempotent from the app's perspective. A conflict means
// the single configured session already exists; the status read below confirms it.
async function ensureSession(config) {
  const current = await readSession(config);
  if (current.response.ok) return current;
  if (current.response.status !== 404) return current;

  const created = await request(config, '/v1/sessions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id: config.session })
  });
  if (created.response.ok || created.response.status === 409) return readSession(config);
  return created;
}

async function pairingDetails(config, state) {
  if (state?.pairing_method === 'qr' || state?.status === 'pairing') {
    const qrResult = await request(config, `${sessionPath(config)}/pairing?format=png`);
    if (qrResult.response.ok && Buffer.isBuffer(qrResult.body) && qrResult.body.length) {
      return { qr: `data:image/png;base64,${qrResult.body.toString('base64')}` };
    }

    // The QR may be published a moment after /connect acknowledges the socket.
    const textResult = await request(config, `${sessionPath(config)}/pairing`);
    if (textResult.response.ok && textResult.body?.method === 'phone') {
      return { pairingCode: textResult.body.code || '' };
    }
  }
  if (state?.pairing_method === 'phone') {
    const pairing = await request(config, `${sessionPath(config)}/pairing`);
    if (pairing.response.ok && pairing.body?.method === 'phone') return { pairingCode: pairing.body.code || '' };
  }
  return {};
}

export async function canaisConfigurados() {
  const config = ownConfig();
  return { proprio: configured(config) };
}

export async function statusWhatsApp() {
  const config = ownConfig();
  if (!configured(config)) {
    return { configurado: false, conectado: false, mensagem: 'O serviço WhatsApp Go ainda não foi configurado no servidor.' };
  }
  try {
    const { response, body } = await readSession(config);
    if (response.status === 404) {
      return { configurado: true, conectado: false, estado: 'nao_criada', instancia: config.session, mensagem: 'A instância ainda não foi criada. Toque em Conectar para iniciar.' };
    }
    if (!response.ok) return { configurado: true, conectado: false, estado: 'erro', mensagem: serviceError(body, 'Não foi possível consultar o serviço WhatsApp Go.') };
    const connected = body?.status === 'connected';
    const pairing = connected ? {} : await pairingDetails(config, body);
    return {
      configurado: true,
      instancia: config.session,
      conectado: connected,
      estado: connected ? 'open' : body?.status || 'desconhecido',
      ...pairing,
      mensagem: connected ? 'WhatsApp conectado. Os envios automáticos estão ativos.' :
        pairing.pairingCode ? 'No celular, abra Aparelhos conectados, escolha Conectar com número de telefone e digite o código.' :
        pairing.qr ? 'Escaneie o QR Code com o WhatsApp no celular.' : 'Preparando o pareamento. Esta tela será atualizada automaticamente.'
    };
  } catch {
    return { configurado: true, conectado: false, estado: 'erro', mensagem: 'Não foi possível consultar o serviço WhatsApp Go.' };
  }
}

export async function conectarWhatsApp(phone = '') {
  const config = ownConfig();
  if (!configured(config)) return { status: 503, body: { error: 'O serviço WhatsApp Go ainda não foi configurado no servidor.' } };
  try {
    const session = await ensureSession(config);
    if (!session.response.ok) return { status: 502, body: { error: serviceError(session.body, 'Não foi possível criar ou localizar a instância WhatsApp.') } };
    if (session.body?.status === 'connected') return { status: 200, body: { ok: true, configurado: true, conectado: true, instancia: config.session, mensagem: 'WhatsApp já está conectado.' } };

    const connected = await request(config, `${sessionPath(config)}/connect`, { method: 'POST' });
    if (!connected.response.ok) return { status: 502, body: { error: serviceError(connected.body, 'Não foi possível iniciar a conexão WhatsApp.') } };
    if (connected.body?.status === 'connected') return { status: 200, body: { ok: true, configurado: true, conectado: true, instancia: config.session } };

    if (phone) {
      const number = normalizarNumero(phone);
      if (!number) return { status: 400, body: { error: 'Informe um telefone válido com DDD e código do país.' } };
      const pairing = await request(config, `${sessionPath(config)}/pair`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ phone: number })
      });
      if (!pairing.response.ok) return { status: pairing.response.status === 504 ? 504 : 502, body: { error: serviceError(pairing.body, 'Não foi possível gerar o código de pareamento.') } };
      return { status: 200, body: { ok: true, configurado: true, conectado: false, instancia: config.session, pairingCode: pairing.body?.code || '', estado: 'pairing' } };
    }

    // Return the current QR if ready. Otherwise the client polls statusWhatsApp,
    // which fetches the QR as soon as whatsmeow emits it.
    const status = await statusWhatsApp();
    return { status: 200, body: { ok: true, ...status } };
  } catch {
    return { status: 502, body: { error: 'Não foi possível consultar o serviço WhatsApp Go.' } };
  }
}

function serviceError(body, fallback) {
  return body?.error?.message || body?.message || body?.error || fallback;
}

export async function desconectarWhatsApp(mode) {
  const config = ownConfig();
  if (!configured(config)) return { status: 503, body: { error: 'O serviço WhatsApp Go ainda não foi configurado no servidor.' } };
  if (!['logout', 'apagar'].includes(mode)) return { status: 400, body: { error: 'modo deve ser "logout" ou "apagar"' } };
  try {
    const session = await readSession(config);
    if (session.response.status === 404 && mode === 'apagar') return { status: 200, body: { ok: true, modo: mode } };
    if (!session.response.ok) return { status: 502, body: { error: serviceError(session.body, 'Não foi possível localizar a sessão WhatsApp.') } };
    const path = sessionPath(config) + (mode === 'logout' ? '/disconnect' : '');
    const { response, body } = await request(config, path, { method: mode === 'logout' ? 'POST' : 'DELETE' });
    if (!response.ok && !(mode === 'apagar' && response.status === 404)) return { status: 502, body: { error: serviceError(body, 'Não foi possível desconectar a conta WhatsApp.') } };
    return { status: 200, body: { ok: true, modo: mode } };
  } catch {
    return { status: 502, body: { error: 'Não foi possível consultar o serviço WhatsApp Go.' } };
  }
}

export function normalizarNumero(telefone) {
  let number = String(telefone || '').replace(/\D/g, '');
  if (number.startsWith('55') && number.length >= 12) return number;
  if (number.length >= 10 && number.length <= 11) return `55${number}`;
  return number.length >= 10 ? number : null;
}

export async function enviarWhatsapp({ telefone, texto, documento_url, documento_nome }) {
  const number = normalizarNumero(telefone);
  if (!number) return { enviado: false, motivo: 'telefone invalido' };
  if (!texto && !documento_url) return { enviado: false, motivo: 'mensagem vazia' };
  const config = ownConfig();
  if (!configured(config)) return { enviado: false, motivo: 'nao configurado' };
  try {
    const session = await readSession(config);
    if (!session.response.ok || session.body?.status !== 'connected') return { enviado: false, motivo: 'whatsapp desconectado' };
    const endpoint = `${config.url}${sessionPath(config)}/messages`;
    if (documento_url) {
      const document = await fetch(documento_url);
      if (!document.ok) throw new Error('download do documento falhou');
      const form = new FormData();
      form.append('to', `${number}@s.whatsapp.net`);
      form.append('caption', texto || '');
      form.append('file', new Blob([await document.arrayBuffer()], { type: 'application/pdf' }), documento_nome || 'Inovar.pdf');
      const uploaded = await fetch(endpoint, { method: 'POST', headers: { Authorization: `Bearer ${config.token}` }, body: form });
      if (uploaded.ok) return { enviado: true, motivo: 'ok (sistema Go, anexo)' };
      const fallback = `${texto || ''}\n\n📎 Documento: ${documento_url}`;
      const sent = await request(config, `${sessionPath(config)}/messages`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ to: `${number}@s.whatsapp.net`, text: fallback })
      });
      return sent.response.ok ? { enviado: true, motivo: 'ok (sistema Go, link)' } : { enviado: false, motivo: 'falha no envio pelo sistema Go' };
    }
    const sent = await request(config, `${sessionPath(config)}/messages`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ to: `${number}@s.whatsapp.net`, text: texto })
    });
    return sent.response.ok ? { enviado: true, motivo: 'ok (sistema Go)' } : { enviado: false, motivo: 'falha no envio pelo sistema Go' };
  } catch {
    return { enviado: false, motivo: 'falha ao comunicar com o sistema WhatsApp Go' };
  }
}
