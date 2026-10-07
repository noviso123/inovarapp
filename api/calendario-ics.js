// Feed de calendário da InovarApp (ICS) — sincroniza os agendamentos com o
// calendário de celulares e computadores (Google Agenda, Apple/iOS, Outlook e
// Android) por ASSINATURA: cadastra a URL uma vez e o calendário atualiza
// sozinho periodicamente. Não precisa de login do Google.
// Uso: /api/calendario-ics?token=<token salvo em config/tecnico.json>
import { SUPABASE_URL, json, queryAsService } from './_supabase.js';

const SRK = process.env.SUPABASE_SERVICE_ROLE_KEY;

async function lerConfig() {
  const r = await fetch(`${SUPABASE_URL}/storage/v1/object/documentos-inovar/config/tecnico.json`, {
    headers: { apikey: SRK, Authorization: `Bearer ${SRK}` }
  });
  if (!r.ok) return null;
  try { return await r.json(); } catch { return null; }
}

const esc = (s) => String(s || '')
  .replace(/\\/g, '\\\\').replace(/;/g, '\\;').replace(/,/g, '\\,')
  .replace(/\r?\n/g, '\\n');
const fmt = (d) => d.toISOString().replace(/[-:]/g, '').replace(/\.\d{3}/, '');

export default async function handler(req, res) {
  if (req.method !== 'GET') return json(res, 405, { error: 'Método não permitido' });

  const token = String(req.query.token || '');
  const config = await lerConfig();
  if (!config || !config.calendario_token || !token || token !== config.calendario_token) {
    return json(res, 404, { error: 'Feed de calendário não encontrado ou token inválido' });
  }

  const { data: servicos } = await queryAsService(
    `/rest/v1/services?status=in.("AGENDADO","EM_ANDAMENTO")&data_agendamento=not.is.null&select=id,tipo,descricao,status,data_agendamento,hora_agendamento,valor,observacoes,cliente_id&order=data_agendamento.asc&limit=500`,
    { method: 'GET' }
  );
  const { data: clientes } = await queryAsService(
    `/rest/v1/customers?select=id,nome,endereco,bairro,cidade,whatsapp`,
    { method: 'GET' }
  );
  const mapaClientes = new Map((Array.isArray(clientes) ? clientes : []).map((c) => [c.id, c]));

  const agora = fmt(new Date());
  const linhas = [
    'BEGIN:VCALENDAR',
    'VERSION:2.0',
    'PRODID:-//Inovar Refrigeracao//InovarApp//PT-BR',
    'CALSCALE:GREGORIAN',
    'METHOD:PUBLISH',
    'X-WR-CALNAME:Inovar Refrigeração — Agenda',
    'X-WR-TIMEZONE:America/Sao_Paulo',
    'REFRESH-INTERVAL;VALUE=DURATION:PT2H',
    'X-PUBLISHED-TTL:PT2H'
  ];

  for (const s of (Array.isArray(servicos) ? servicos : [])) {
    const cl = mapaClientes.get(s.cliente_id) || {};
    const dataStr = String(s.data_agendamento || '').slice(0, 10);
    if (!dataStr) continue;
    const [ano, mes, dia] = dataStr.split('-').map(Number);
    const [h, m] = String(s.hora_agendamento || '09:00').split(':').map(Number);
    const inicio = new Date(Date.UTC(ano, (mes || 1) - 1, dia || 1, (Number.isFinite(h) ? h : 9) + 3, m || 0)); // BRT
    const fim = new Date(inicio.getTime() + 2 * 60 * 60 * 1000);
    const local = [cl.endereco, cl.bairro, cl.cidade].filter(Boolean).join(', ');
    linhas.push(
      'BEGIN:VEVENT',
      `UID:inovar-${s.id}@inovarapp`,
      `DTSTAMP:${agora}`,
      `DTSTART:${fmt(inicio)}`,
      `DTEND:${fmt(fim)}`,
      `SUMMARY:${esc(String(s.descricao || s.tipo || 'Atendimento').replace(/_/g, ' '))} — ${esc(cl.nome || 'Cliente')}`,
      `DESCRIPTION:${esc(`Serviço Inovar Refrigeração (${s.status}). Valor: R$ ${Number(s.valor || 0).toFixed(2)}. Contato: ${cl.whatsapp || ''}`)}`,
      `LOCATION:${esc(local)}`,
      'END:VEVENT'
    );
  }
  linhas.push('END:VCALENDAR');

  res.statusCode = 200;
  res.setHeader('Content-Type', 'text/calendar; charset=utf-8');
  res.setHeader('Cache-Control', 'no-store, max-age=0');
  res.setHeader('Content-Disposition', 'inline; filename="inovar-agenda.ics"');
  res.end(linhas.join('\r\n'));
}
