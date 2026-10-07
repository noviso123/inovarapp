// Contrato legado do frontend, atendido exclusivamente pelo sistema Go próprio.
import { json, authCaller } from './_supabase.js';
import { enviarWhatsapp, statusWhatsApp, conectarWhatsApp, desconectarWhatsApp } from './_whatsapp.js';

export default async function handler(req, res) {
  if (req.method !== 'POST') return json(res, 405, { error: 'Método não permitido' });
  const { acao, telefone, texto, documento_url, documento_nome, modo } = req.body || {};
  if (!['enviar', 'status', 'conectar', 'desconectar'].includes(acao)) {
    return json(res, 400, { error: 'Ação inválida' });
  }
  const caller = await authCaller(req);
  if (!caller) return json(res, 401, { error: 'Não autenticado' });
  if (caller.role === 'CLIENTE') return json(res, 403, { error: 'Somente a equipe Inovar' });

  if (acao === 'status') return json(res, 200, await statusWhatsApp());
  if (acao === 'conectar') {
    const result = await conectarWhatsApp(telefone);
    return json(res, result.status, result.body);
  }
  if (acao === 'desconectar') {
    const result = await desconectarWhatsApp(modo);
    return json(res, result.status, result.body);
  }
  if (!telefone || (!texto && !documento_url)) {
    return json(res, 400, { error: 'Informe telefone e mensagem ou documento.' });
  }
  const result = await enviarWhatsapp({ telefone, texto, documento_url, documento_nome });
  if (!result.enviado) return json(res, 502, { error: 'Falha no envio WhatsApp', detalhe: result.motivo });
  return json(res, 200, { ok: true, configurado: true, provedor: 'whatsapp-go-proprio' });
}
