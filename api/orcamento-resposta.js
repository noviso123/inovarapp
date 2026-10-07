// Aprovação/recusa de orçamento com assinatura digital.
// Chamadores aceitos: o cliente dono do orçamento (via vínculo customers.profile_id)
// ou um ADMIN/TECNICO. Grava o status e a assinatura (PNG em base64) no campo
// descricao (JSON) da tabela budgets, preservando os demais dados.

import { json, authCaller, queryAsUser, queryAsService } from './_supabase.js';
import { enviarWhatsapp, canaisConfigurados } from './_whatsapp.js';
import { lerConfigServidor } from './_config.js';

// Aviso automático (WhatsApp) com textos da Central de Mensagens (mensagensWhats) ou padrão
function aplicarVarsResp(texto, vars) {
  return String(texto || '').replace(/\{\{(\w+)\}\}/g, (_, k) => (vars[k] === undefined || vars[k] === null ? '' : String(vars[k])));
}
async function msgResp(chave, padrao, vars) {
  try {
    const cfgRaw = await lerConfigServidor();
    const salvo = cfgRaw && cfgRaw.mensagensWhats && cfgRaw.mensagensWhats[chave];
    return aplicarVarsResp(salvo && salvo.trim() ? salvo : padrao, vars);
  } catch {
    return aplicarVarsResp(padrao, vars);
  }
}

export default async function handler(req, res) {
  if (req.method !== 'POST') return json(res, 405, { error: 'Método não permitido' });

  const caller = await authCaller(req);
  if (!caller) return json(res, 401, { error: 'Não autenticado' });

  const { orcamento_id, acao, assinatura } = req.body || {};
  if (!orcamento_id) return json(res, 400, { error: 'Orçamento não informado' });
  const acaoUpper = String(acao || 'APROVAR').toUpperCase();
  if (!['APROVAR', 'RECUSAR'].includes(acaoUpper)) {
    return json(res, 400, { error: 'Ação inválida' });
  }
  if (assinatura && assinatura.length > 400000) {
    return json(res, 413, { error: 'Assinatura muito grande' });
  }

  // Dados do orçamento (leitura com o JWT do chamador — RLS filtra:
  // cliente só alcança o próprio orçamento; admin/tecnico alcançam todos)
  const leitura = await queryAsUser(
    caller.token,
    `/rest/v1/budgets?id=eq.${orcamento_id}&select=id,cliente_id,descricao,status`
  );
  let rows = leitura.data;

  // Depois de respondido, algumas políticas RLS ocultam o orçamento do
  // cliente. Reconsultamos por service role apenas para decidir se ele é o
  // mesmo dono; assim a resposta repetida continua idempotente (409), sem
  // revelar qualquer orçamento de outro cliente.
  if ((!Array.isArray(rows) || rows.length === 0) && caller.role === 'CLIENTE') {
    const { data: mine } = await queryAsUser(
      caller.token,
      `/rest/v1/customers?profile_id=eq.${caller.userId}&select=id`
    );
    const meuCustomerId = Array.isArray(mine) && mine[0]?.id;
    const historico = await queryAsService(
      `/rest/v1/budgets?id=eq.${orcamento_id}&select=id,cliente_id,descricao,status`,
      { method: 'GET' }
    );
    if (historico.status === 200 && Array.isArray(historico.data) && historico.data[0]?.cliente_id === meuCustomerId) {
      rows = historico.data;
    }
  }
  if (leitura.status !== 200 || !Array.isArray(rows) || rows.length === 0) {
    return json(res, 404, { error: 'Orçamento não encontrado para este usuário' });
  }
  const orcamento = rows[0];

  // Se o chamador não é admin/tecnico, precisa ser o cliente dono
  if (caller.role === 'CLIENTE') {
    const { data: mine } = await queryAsUser(
      caller.token,
      `/rest/v1/customers?profile_id=eq.${caller.userId}&select=id`
    );
    const meuCustomerId = Array.isArray(mine) && mine[0]?.id;
    if (!meuCustomerId || meuCustomerId !== orcamento.cliente_id) {
      return json(res, 403, { error: 'Este orçamento não pertence ao seu cadastro' });
    }
  }

  // Regra de negocio: cliente aprova SOMENTE com assinatura; tecnico aprova administrativamente
  if (caller.role === 'CLIENTE' && acaoUpper === 'APROVAR' && !assinatura) {
    return json(res, 400, { error: 'Assinatura digital obrigatoria para aprovacao pelo cliente' });
  }

  if (orcamento.status === 'APROVADO' || orcamento.status === 'RECUSADO') {
    return json(res, 409, { error: `Este orçamento já foi ${orcamento.status.toLowerCase()}` });
  }

  // Mescla a assinatura no JSON de descricao
  let meta = {};
  try { meta = JSON.parse(orcamento.descricao || '{}'); } catch { meta = {}; }
  meta.assinatura = assinatura || null;
  meta.assinatura_em = assinatura ? new Date().toISOString() : null;
  meta.resposta_cliente = acaoUpper;

  const novoStatus = acaoUpper === 'APROVAR' ? 'APROVADO' : 'RECUSADO';
  const { status: upStatus } = await queryAsService(
    `/rest/v1/budgets?id=eq.${orcamento_id}`,
    {
      method: 'PATCH',
      body: { status: novoStatus, descricao: JSON.stringify(meta) }
    }
  );
  if (upStatus !== 200 && upStatus !== 204) {
    return json(res, 500, { error: 'Falha ao atualizar o orçamento' });
  }

  // Aviso automático pelo WhatsApp (textos editáveis na Central de Mensagens)
  try {
    const canais = await canaisConfigurados();
    if (canais.proprio) {
      const cfg = await lerConfigServidor();
      const empresa = cfg?.businessName || 'Inovar Refrigeração';
      const nomeCliente = meta.clientName || 'Cliente';
      const numero = meta.numero || '';
      const valor = meta.totalValue != null ? 'R$ ' + Number(meta.totalValue).toFixed(2) : '';
      const vars = { cliente: nomeCliente, numero, valor, empresa };

      // 1) avisa a empresa (aprovação/recusa)
      const tecnicoNum = cfg?.phone;
      if (tecnicoNum) {
        const msg = acaoUpper === 'APROVAR'
          ? await msgResp('orcamento_aprovado_tecnico',
              '✅ *Orçamento aprovado e assinado!*\n\nO cliente *{{cliente}}* aprovou a proposta (assinatura registrada). Hora de agendar o serviço!\n\n— InovarApp', vars)
          : await msgResp('orcamento_recusado_tecnico',
              '⚠️ *Orçamento recusado.*\n\nO cliente *{{cliente}}* recusou a proposta.\n\n— InovarApp', vars);
        enviarWhatsapp({ telefone: tecnicoNum, texto: msg }).catch(() => {});
      }

      // 2) confirma a aprovação ao próprio cliente
      if (acaoUpper === 'APROVAR') {
        const cli = await queryAsService(`/rest/v1/customers?id=eq.${orcamento.cliente_id}&select=whatsapp`, { method: 'GET' });
        const zapCliente = Array.isArray(cli?.data) && cli.data[0]?.whatsapp;
        if (zapCliente) {
          const msgCli = await msgResp('orcamento_aprovado_cliente',
            '✅ *Recebemos sua aprovação*, {{cliente}}!\n\nProposta nº {{numero}} ({{valor}}) confirmada com assinatura digital.\n\nNossa equipe vai entrar em contato para agendar o serviço. Qualquer dúvida, chame por aqui! ❄️\n\n*{{empresa}}*', vars);
          enviarWhatsapp({ telefone: zapCliente, texto: msgCli }).catch(() => {});
        }
      }
    }
  } catch { /* não bloqueia a resposta */ }

  return json(res, 200, {
    ok: true,
    status: novoStatus,
    assinado_em: meta.assinatura_em,
    mensagem: acaoUpper === 'APROVAR'
      ? 'Orçamento aprovado e assinado! A Inovar foi notificada.'
      : 'Orçamento recusado. A Inovar foi notificada.'
  });
}
