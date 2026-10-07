// E-mail da InovarApp — Gmail SMTP (conta própria, sem domínio, sem Resend).
// acao: 'enviar' | 'disparo' (todos os clientes com e-mail) | 'testar'
import { json, authCaller } from './_supabase.js';
import { enviarEmailAutomatico, gmailConfigurado } from './_email.js';

const SUPABASE_URL = process.env.VITE_SUPABASE_URL || 'https://ycpswioserctavijhnre.supabase.co';
const SRK = process.env.SUPABASE_SERVICE_ROLE_KEY;

const TEMPLATE_TESTE = (nome) => `
<div style="font-family:sans-serif;max-width:560px;margin:auto;border:1px solid #e2e8f0;border-radius:12px;overflow:hidden">
  <div style="background:#0B2D4E;padding:20px 24px"><span style="color:#FFC61E;font-weight:800;font-size:20px">❄️ InovarApp</span></div>
  <div style="padding:24px">
    <h2 style="color:#0B2D4E;margin-top:0">Olá, ${nome}!</h2>
    <p>Este é um <b>e-mail de teste do sistema de envios</b> da Inovar Refrigeração.</p>
    <p>Por este canal você receberá: <b>alertas de manutenção preventiva</b>, <b>orçamentos</b> e <b>comunicados</b>.</p>
    <p style="color:#64748b;font-size:13px">Se recebeu este e-mail no spam, marque como "não é spam" para receber os próximos normalmente.</p>
  </div>
  <div style="background:#f8fafc;padding:14px 24px;color:#94a3b8;font-size:12px">Inovar Refrigeração — enviado automaticamente pelo InovarApp</div>
</div>`;

async function clientesComEmail() {
  const r = await fetch(`${SUPABASE_URL}/rest/v1/customers?select=id,nome,profile_id,profiles!customers_profile_id_fkey(email)`, {
    headers: { apikey: SRK, Authorization: `Bearer ${SRK}` }
  });
  const clientes = await r.json().catch(() => []);
  return (Array.isArray(clientes) ? clientes : [])
    .filter((c) => c.profiles?.email)
    .map((c) => ({ nome: c.nome, email: c.profiles.email }));
}

export default async function handler(req, res) {
  if (req.method !== 'POST') return json(res, 405, { error: 'Método não permitido' });

  const { acao } = req.body || {};
  if (!['enviar', 'disparo', 'testar'].includes(acao)) return json(res, 400, { error: 'Ação inválida' });

  const caller = await authCaller(req);
  if (!caller) return json(res, 401, { error: 'Não autenticado' });
  if (caller.role === 'CLIENTE') return json(res, 403, { error: 'Somente a equipe Inovar dispara e-mails' });

  if (!(await gmailConfigurado())) {
    return json(res, 200, { configurado: false, mensagem: 'Configure o Gmail (usuário + senha de app) nas Configurações.' });
  }

  if (acao === 'testar') {
    const destinatarioTeste = String(process.env.EMAIL_TEST_TO || '').trim();
    if (!destinatarioTeste) return json(res, 200, { configurado: false, mensagem: 'Configure EMAIL_TEST_TO para o teste de envio.' });
    const r = await enviarEmailAutomatico(destinatarioTeste, 'InovarApp — canal de e-mail ativo (Gmail)!', TEMPLATE_TESTE('equipe Inovar'));
    return json(res, r.enviado ? { ok: true, canal: 'gmail' } : { ok: false, error: r.motivo });
  }

  if (acao === 'enviar') {
    const { para, assunto, html } = req.body || {};
    if (!para || !assunto || !html) return json(res, 400, { error: 'Informe para, assunto e html' });
    const r = await enviarEmailAutomatico(para, assunto, html);
    if (!r.enviado) return json(res, 502, { error: 'Falha no envio de e-mail', detalhe: r.motivo });
    return json(res, 200, { ok: true, id: r.id, canal: 'gmail' });
  }

  // disparo para todos os clientes com e-mail
  const { assunto, html, tipo } = req.body || {};
  const ehTeste = tipo === 'teste' || (!assunto && !html);
  const assuntoFinal = assunto || 'InovarApp — e-mail de teste';
  const htmlFinal = html || null;

  const destinatarios = await clientesComEmail();
  const resultados = [];
  for (const d of destinatarios) {
    const corpo = htmlFinal || TEMPLATE_TESTE((d.nome || 'Cliente').split(' ')[0]);
    const r = await enviarEmailAutomatico(d.email, assuntoFinal, corpo);
    resultados.push({ cliente: d.nome, email: d.email, ok: r.enviado, motivo: (r.motivo || '').slice(0, 120) });
  }

  return json(res, 200, {
    ok: true,
    canal: 'gmail',
    tipo: ehTeste ? 'teste' : 'personalizado',
    totalClientesComEmail: destinatarios.length,
    enviados: resultados.filter((x) => x.ok).length,
    falhas: resultados.filter((x) => !x.ok).length,
    resultados
  });
}
