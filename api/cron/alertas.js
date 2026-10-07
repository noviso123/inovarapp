// CRON DIÁRIO — Alertas automáticos de manutenção preventiva da Inovar.
// Agenda-se pelo vercel.json (todos os dias às 09:00, horário UTC na Vercel).
// Proteção: a Vercel envia automaticamente o header
//   authorization: Bearer ${CRON_SECRET}
// O que faz:
//   1. Lê clientes, aparelhos e histórico de serviços (service_history + services).
//   2. Para cada aparelho, calcula o retorno recomendado:
//      última limpeza de ar/manutenção CONCLUÍDA + ALERTA_MESES (padrão 6).
//   3. Se o retorno venceu (e não está atrasado há mais de 180 dias — evita
//      importunar clientes antigos), dispara o alerta para o cliente:
//      - WhatsApp Go próprio  -> se WHATSAPP_OWN_URL/TOKEN definidos
//      - E-mail (Resend)       -> se EMAIL_API_KEY/EMAIL_FROM definidos
//   4. Devolve um resumo do disparo.
// Sem canais configurados, roda em modo relatório (não quebra nada).

import { enviarWhatsapp, canaisConfigurados } from '../_whatsapp.js';
import { lerConfigServidor, emailConfigFrom } from '../_config.js';
import { sendPushToUser } from '../_push.js';

const SUPABASE_URL = process.env.VITE_SUPABASE_URL || 'https://ycpswioserctavijhnre.supabase.co';
const SRK = process.env.SUPABASE_SERVICE_ROLE_KEY;
const ANON = process.env.VITE_SUPABASE_ANON_KEY;

const json = (res, status, body) => {
  res.statusCode = status;
  res.setHeader('Content-Type', 'application/json; charset=utf-8');
  res.end(JSON.stringify(body));
};

async function serviceGet(path) {
  const r = await fetch(SUPABASE_URL + path, {
    headers: { apikey: SRK, Authorization: `Bearer ${SRK}` }
  });
  return r.json();
}

// ---- Central de Mensagens: usa o texto salvo pelo admin (mensagensWhats) ou o padrão ----
function aplicarVars(texto, vars) {
  return String(texto || '').replace(/\{\{(\w+)\}\}/g, (_, k) => (vars[k] === undefined || vars[k] === null ? '' : String(vars[k])));
}
async function textoLembrete(chave, padrao, vars) {
  try {
    const cfgRaw = await lerConfigServidor();
    const salvo = cfgRaw && cfgRaw.mensagensWhats && cfgRaw.mensagensWhats[chave];
    return aplicarVars(salvo && salvo.trim() ? salvo : padrao, vars);
  } catch {
    return aplicarVars(padrao, vars);
  }
}

async function enviarEmail(para, assunto, html) {
  const cfg = emailConfigFrom(await lerConfigServidor());
  const apiKey = cfg.apiKey;
  const from = cfg.from;
  if (!apiKey || !from) return { enviado: false, motivo: 'nao configurado' };
  if (!para || !/.+@.+\..+/.test(para)) return { enviado: false, motivo: 'e-mail invalido' };
  const r = await fetch('https://api.resend.com/emails', {
    method: 'POST',
    headers: { Authorization: `Bearer ${apiKey}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ from, to: [para], subject: assunto, html })
  });
  return { enviado: r.ok, motivo: r.ok ? 'ok' : 'falha envio' };
}

const fmtData = (iso) => iso ? iso.split('T')[0].split('-').reverse().join('/') : '';

// Nome legivel do servico: descricao do banco se for texto simples
function descricaoLegivel(svc) {
  if (!svc) return '';
  const d = (svc.descricao || '').trim();
  if (d.startsWith('{') || d.startsWith('[')) return '';
  return d;
}

export default async function handler(req, res) {
  const secret = process.env.CRON_SECRET;
  const auth = req.headers.authorization || '';
  if (secret && auth !== `Bearer ${secret}`) {
    return json(res, 401, { error: 'Não autorizado' });
  }

  try {
    const meses = Number(process.env.ALERTA_MESES || 6);
    const maxAtrasoDias = Number(process.env.ALERTA_MAX_ATRASO_DIAS || 180);
    const hoje = new Date();

    const [clientes, perfis, aparelhos, historico, servicos] = await Promise.all([
      serviceGet('/rest/v1/customers?select=id,nome,whatsapp,profile_id&ativo=eq.true'),
      serviceGet('/rest/v1/profiles?select=id,email'),
      serviceGet('/rest/v1/air_conditioners?select=id,cliente_id,marca,modelo,ambiente,ultima_manutencao'),
      serviceGet(`/rest/v1/service_history?select=cliente_id,aparelho_id,data,descricao&order=data.desc&limit=2000`),
      serviceGet(`/rest/v1/services?status=eq.CONCLUIDO&select=cliente_id,aparelho_id,data_agendamento&order=data_agendamento.desc&limit=2000`)
    ]);

    // e-mail do cliente vem do perfil vinculado
    const emailsPorProfile = new Map((Array.isArray(perfis) ? perfis : []).map((p) => [p.id, p.email]));
    (Array.isArray(clientes) ? clientes : []).forEach((c) => {
      if (c.profile_id && emailsPorProfile.has(c.profile_id)) c.email = emailsPorProfile.get(c.profile_id);
    });

    // último atendimento concluído por aparelho:
    // considera service_history, serviços CONCLUIDOS e a última manutenção
    // registrada na ficha do aparelho (a mais recente entre elas).
    const ultimo = new Map();
    const considera = (clienteId, aparelhoId, dataISO) => {
      if (!dataISO || !clienteId || !aparelhoId) return;
      const d = dataISO.split('T')[0];
      const key = clienteId + '|' + aparelhoId;
      const atual = ultimo.get(key);
      if (!atual || d > atual) ultimo.set(key, d);
    };
    (Array.isArray(historico) ? historico : []).forEach((h) => considera(h.cliente_id, h.aparelho_id, h.data));
    (Array.isArray(servicos) ? servicos : []).forEach((s) => considera(s.cliente_id, s.aparelho_id, s.data_agendamento));
    (Array.isArray(aparelhos) ? aparelhos : []).forEach((a) => considera(a.cliente_id, a.id, a.ultima_manutencao));

    const clientesMap = new Map((Array.isArray(clientes) ? clientes : []).map((c) => [c.id, c]));
    const alertas = [];

    for (const [key, dataUltimo] of ultimo.entries()) {
      const [clienteId, aparelhoId] = key.split('|');
      const cliente = clientesMap.get(clienteId);
      if (!cliente) continue;
      const aparelho = (aparelhos || []).find((a) => a.id === aparelhoId);
      if (!aparelho) continue;

      const retornoPrevisto = new Date(dataUltimo + 'T00:00:00');
      retornoPrevisto.setMonth(retornoPrevisto.getMonth() + meses);
      const diasParaVencer = Math.round((retornoPrevisto - hoje) / 86400000);

      // alerta quando venceu (ou vence em até 5 dias) e não está atrasado demais
      if ((diasParaVencer <= 0 || diasParaVencer <= 5) && diasParaVencer >= -maxAtrasoDias) {
        alertas.push({ cliente, aparelho, dataUltimo, retornoPrevisto: retornoPrevisto.toISOString().slice(0, 10), diasParaVencer });
      }
    }

    const resultados = [];
    // Reenvio "de tempo em tempo": lê intervalo configurável (dias) e o log de enviados
    const cfgCiclo = await lerConfigServidor();
    const intervaloDias = Math.max(1, Number(cfgCiclo?.lembrete_intervalo_dias) || 7);
    const logLembretes = (cfgCiclo && cfgCiclo.lembretes_log) || {};
    let logAlterado = false;
    for (const a of alertas) {
      const nome = a.cliente.nome.split(' ')[0];
      const eq = `${a.aparelho.marca} ${a.aparelho.modelo || ''} ${a.aparelho.btus || ''} BTUs (${a.aparelho.ambiente || 'aparelho'})`.trim();
      // já avisado dentro do intervalo definido? pula (evita spam diário)
      const chaveLog = `${a.cliente.id || a.cliente.nome}|${a.aparelho.id || eq}`;
      const ultimoEnvio = logLembretes[chaveLog];
      if (ultimoEnvio && (Date.now() - new Date(ultimoEnvio).getTime()) < intervaloDias * 86400000) {
        resultados.push({ cliente: a.cliente.nome, aparelho: eq, pulado: true, motivo: `lembrado em ${ultimoEnvio.slice(0, 10)} (intervalo ${intervaloDias}d)` });
        continue;
      }
      logLembretes[chaveLog] = new Date().toISOString();
      logAlterado = true;
      const atraso = a.diasParaVencer < 0 ? `está em atraso há ${Math.abs(a.diasParaVencer)} dias` : 'vence nos próximos dias';
      // texto salvo pelo admin na Central de Mensagens (ou o padrão abaixo)
      const texto = await textoLembrete('lembrete_ciclo_vencido',
        `Olá, {{cliente}}! Tudo bem? 😊\n\nAqui é a *Inovar Refrigeração*.\n\n❄️ Pela nossa ficha técnica, a manutenção/limpeza de ar do seu *{{equipamento}}* realizada em {{data_ultima}} já {{situacao}} (ciclo recomendado de {{meses}} meses).\n\nManter o ciclo evita fungos, bactérias, mau cheiro e maior consumo de energia. Posso agendar sua visita? 🗓️\n\nInovar Refrigeração`,
        { cliente: nome, equipamento: eq, data_ultima: fmtData(a.dataUltimo), situacao: atraso, meses }
      );

      const canais = {};
      if (a.cliente.whatsapp) canais.whatsapp = await enviarWhatsapp({ telefone: a.cliente.whatsapp, texto });
      if (a.cliente.email) {
        canais.email = await enviarEmail(
          a.cliente.email,
          '❄️ Inovar: hora da manutenção preventiva do seu ar-condicionado',
          `<p>Olá, <b>${nome}</b>!</p><p>A manutenção/limpeza de ar do seu <b>${eq}</b> (realizada em ${fmtData(a.dataUltimo)}) ${atraso} — o ciclo recomendado é de ${meses} meses.</p><p>Manter o ciclo evita fungos, bactérias, mau cheiro e consumo elevado de energia.</p><p>Responda este e-mail ou chame no WhatsApp para agendar sua visita! 🗓️</p><p><b>Inovar — Refrigeração</b></p>`
        );
      }
      if (a.cliente.profile_id) canais.push = await sendPushToUser(a.cliente.profile_id, {
        title: 'Manutenção preventiva Inovar',
        body: `Seu ${eq} ${atraso}. Toque para solicitar o atendimento.`,
        url: '/'
      });
      resultados.push({ cliente: a.cliente.nome, aparelho: eq, retorno: a.retornoPrevisto, canais });
    }

    // ===== LEMBRETES DE AGENDAMENTO (1 dia antes do atendimento) =====
    const amanhaISO = new Date(hoje.getTime() + 86400000).toISOString().slice(0, 10);
    const agendadosAmanha = await serviceGet(`/rest/v1/services?status=eq.AGENDADO&data_agendamento=eq.${amanhaISO}&select=id,cliente_id,tipo,data_agendamento,hora_agendamento,valor`);
    const lembretesAgenda = [];
    for (const svc of (Array.isArray(agendadosAmanha) ? agendadosAmanha : [])) {
      const cliente = clientesMap.get(svc.cliente_id);
      if (!cliente) continue;
      const aparelho = (Array.isArray(aparelhos) ? aparelhos : []).find((a) => a.id === svc.aparelho_id);
      const eq = aparelho ? `${aparelho.marca} ${aparelho.modelo || ''} ${aparelho.btus || ''} BTUs — ${aparelho.ambiente || 'ambiente não informado'}`.replace(/\s+/g, ' ').trim() : 'equipamento não informado no cadastro';
      const hora = svc.hora_agendamento || '';
      const nomesAmigaveis = {
        LIMPEZA: 'Limpeza de Ar',
        INSTALACAO: 'Instalação',
        MANUTENCAO_PREVENTIVA: 'Manutenção Preventiva',
        MANUTENCAO_CORRETIVA: 'Manutenção Corretiva',
        RECARGA_GAS: 'Recarga de Gás',
        AVALIACAO: 'Avaliação Técnica',
        OUTRO: 'Serviço'
      };
      let nomeServico = descricaoLegivel(svc) || nomesAmigaveis[svc.tipo] || svc.tipo || 'Serviço';
      // texto salvo pelo admin na Central de Mensagens (ou o padrão abaixo)
      const texto = await textoLembrete('lembrete_vespera',
        `Olá, {{cliente}}! Tudo bem? 😊\n\n*Inovar Refrigeração* passando para lembrar:\n\n📅 Seu atendimento está *AGENDADO PARA AMANHÃ* ({{data}}{{hora}}).\n❄️ Serviço: {{servico}}\n🏠 Equipamento: {{equipamento}}\n\nQualquer imprevisto, é só nos chamar por aqui! Estamos à disposição. ❄️`,
        {
          cliente: cliente.nome.split(' ')[0],
          data: fmtData(svc.data_agendamento),
          hora: hora ? ' às ' + hora : '',
          servico: nomeServico,
          equipamento: eq
        }
      );
      const canais = {};
      if (cliente.whatsapp) canais.whatsapp = await enviarWhatsapp({ telefone: cliente.whatsapp, texto });
      if (cliente.email) canais.email = await enviarEmail(cliente.email, '⏰ Lembrete InovarApp: seu atendimento é amanhã!', `<p>Olá, <b>${cliente.nome.split(' ')[0]}</b>!</p><p>Seu atendimento está agendado para <b>amanhã, ${fmtData(svc.data_agendamento)}${hora ? ' às ' + hora : ''}</b>.</p><p>❄️ Serviço: ${svc.tipo ? svc.tipo.replace('_', ' ') : ''} — ${eq}</p><p>Qualquer imprevisto, é só nos chamar! <b>Inovar Refrigeração</b></p>`);
      if (cliente.profile_id) canais.push = await sendPushToUser(cliente.profile_id, {
        title: 'Atendimento agendado para amanhã',
        body: `${nomeServico}${hora ? ` às ${hora}` : ''}. Toque para conferir os detalhes.`,
        url: '/'
      });
      lembretesAgenda.push({ cliente: cliente.nome, data: svc.data_agendamento, hora, canais });
    }

    // ===== PUSH 1 HORA ANTES PARA QUEM TEM O APP INSTALADO =====
    // O cron roda de hora em hora; a janela de 30 minutos evita perda por
    // pequenos atrasos de execução e o log impede notificações repetidas.
    const agoraBR = new Date(new Date().toLocaleString('en-US', { timeZone: 'America/Sao_Paulo' }));
    const dataHojeBR = `${agoraBR.getFullYear()}-${String(agoraBR.getMonth() + 1).padStart(2, '0')}-${String(agoraBR.getDate()).padStart(2, '0')}`;
    const proximosHoje = await serviceGet(`/rest/v1/services?status=eq.AGENDADO&data_agendamento=eq.${dataHojeBR}&select=id,cliente_id,tipo,descricao,hora_agendamento,data_agendamento`);
    const logUmaHora = (cfgCiclo && cfgCiclo.lembretes_uma_hora_log) || {};
    const lembretesUmaHora = [];
    for (const svc of (Array.isArray(proximosHoje) ? proximosHoje : [])) {
      if (!svc.hora_agendamento || logUmaHora[svc.id]) continue;
      const [hh, mm] = String(svc.hora_agendamento).slice(0, 5).split(':').map(Number);
      if (!Number.isFinite(hh) || !Number.isFinite(mm)) continue;
      const horario = new Date(agoraBR); horario.setHours(hh, mm, 0, 0);
      const faltam = Math.round((horario.getTime() - agoraBR.getTime()) / 60000);
      if (faltam < 30 || faltam > 90) continue;
      const cliente = clientesMap.get(svc.cliente_id);
      if (!cliente?.profile_id) continue;
      const nomeServico = descricaoLegivel(svc) || ({ LIMPEZA: 'Limpeza de Ar', INSTALACAO: 'Instalação', MANUTENCAO_PREVENTIVA: 'Manutenção Preventiva', MANUTENCAO_CORRETIVA: 'Manutenção Corretiva', RECARGA_GAS: 'Recarga de Gás', AVALIACAO: 'Avaliação Técnica' }[svc.tipo] || 'Atendimento');
      const push = await sendPushToUser(cliente.profile_id, { title: 'Seu atendimento é em aproximadamente 1 hora', body: `${nomeServico} às ${svc.hora_agendamento}. Toque para ver os detalhes.`, url: '/', tag: `agenda-1h-${svc.id}` });
      logUmaHora[svc.id] = new Date().toISOString();
      lembretesUmaHora.push({ cliente: cliente.nome, hora: svc.hora_agendamento, push });
    }

    // Salva ambos os logs em uma única gravação, preservando todas as outras
    // configurações do técnico.
    if (logAlterado || lembretesUmaHora.length) {
      try {
        const SRKc = process.env.SUPABASE_SERVICE_ROLE_KEY;
        await fetch(`${SUPABASE_URL}/storage/v1/object/documentos-inovar/config/tecnico.json`, {
          method: 'POST',
          headers: { apikey: SRKc, Authorization: `Bearer ${SRKc}`, 'Content-Type': 'application/json', 'x-upsert': 'true' },
          body: JSON.stringify({ ...(cfgCiclo || {}), lembretes_log: logLembretes, lembretes_uma_hora_log: logUmaHora })
        });
      } catch { /* best-effort */ }
    }

    return json(res, 200, {
      ok: true,
      executadoEm: new Date().toISOString(),
      ciclosVencidos: alertas.length,
      lembretesAgenda: lembretesAgenda.length,
      lembretesUmaHora: lembretesUmaHora.length,
      whatsappConfigurado: (await canaisConfigurados()).proprio,
      // cobre credenciais salvas no app (Gmail) e/ou nas variáveis de ambiente (Resend)
      emailConfigurado: !!(emailConfigFrom(cfgCiclo || {}).apiKey || (process.env.EMAIL_API_KEY && process.env.EMAIL_FROM)),
      disparosAgenda: lembretesAgenda
    });
  } catch (e) {
    return json(res, 500, { error: 'Falha no cron de alertas', detalhe: String(e).slice(0, 300) });
  }
}
