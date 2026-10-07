// Central de Mensagens do WhatsApp — catálogo único das mensagens automáticas.
// O admin pode editar cada mensagem (salva no perfil via /api/configuracoes,
// campo mensagensWhats). Placeholders {{assim}} são substituídos no envio.
// NOTA: os textos padrão também existem como fallback em api/cron/alertas.js
// e api/contas.js — ao alterar um padrão aqui, mantenha os fallbacks em sincronia.

export interface ModeloMensagem {
  chave: string;
  titulo: string;
  quando: string;
  placeholders: string[];
  padrao: string;
}

const APP = 'https://inovarapp.vercel.app';

export const MODELOS_MENSAGENS: ModeloMensagem[] = [
  {
    chave: 'boas_vindas_autocadastro',
    titulo: 'Boas-vindas (auto-cadastro)',
    quando: 'Cliente cria a própria conta no app',
    placeholders: ['{{cliente}}', '{{email}}', '{{empresa}}', '{{app}}'],
    padrao:
      '🎉 *Bem-vindo(a) à {{empresa}}*, {{cliente}}!\n\n' +
      'Sua conta no *InovarApp* foi criada com o e-mail:\n' +
      '📧 {{email}}\n\n' +
      '📲 *Acesse:* {{app}}\n\n' +
      'No app você acompanha seus aparelhos, solicita atendimentos, recebe orçamentos com assinatura e consulta suas garantias! ❄️'
  },
  {
    chave: 'boas_vindas_tecnico',
    titulo: 'Boas-vindas (conta criada pelo técnico)',
    quando: 'Técnico cadastra cliente com acesso ao app',
    placeholders: ['{{cliente}}', '{{email}}', '{{senha}}', '{{empresa}}', '{{app}}'],
    padrao:
      '🎉 *Bem-vindo(a) à {{empresa}}*, {{cliente}}!\n\n' +
      'Sua conta de acesso ao *InovarApp* foi criada:\n' +
      '📧 *Login:* {{email}}\n' +
      '🔑 *Senha:* {{senha}}\n\n' +
      '📲 *Acesse aqui:* {{app}}\n\n' +
      '*Como entrar:*\n' +
      '1. Abra o link acima\n' +
      '2. Toque em *Entrar no App*\n' +
      '3. Use o login e a senha acima\n\n' +
      '💡 No app você acompanha seus aparelhos, orçamentos, agendamentos e garantias. Recomendamos trocar a senha em *Meus Dados → Alterar Senha*.\n\n' +
      '*{{empresa}}* ❄️'
  },
  {
    chave: 'orcamento_criado',
    titulo: 'Orçamento criado (com PDF anexo)',
    quando: 'Técnico gera um orçamento',
    placeholders: ['{{cliente}}', '{{empresa}}', '{{numero}}', '{{valor}}', '{{app}}'],
    padrao:
      'Olá, {{cliente}}! Tudo bem? 😊\n\n' +
      'Aqui é a *{{empresa}}*.\n\n' +
      'Segue a sua *Proposta/Orçamento nº {{numero}}*\n' +
      '💰 *Valor total: {{valor}}*\n\n' +
      '📄 A proposta completa em PDF está anexada nesta mensagem.\n\n' +
      'Qualquer dúvida estou à disposição! ❄️🤝\n\n' +
      '📲 Acompanhe tudo pelo app: {{app}}'
  },
  {
    chave: 'os_concluida',
    titulo: 'OS concluída (comprovante + garantia em PDF)',
    quando: 'Serviço de campo é concluído',
    placeholders: ['{{cliente}}', '{{os}}', '{{servico}}', '{{data}}', '{{garantia}}', '{{empresa}}', '{{app}}'],
    padrao:
      'Olá, {{cliente}}! ✅\n\n' +
      'Sua *Ordem de Serviço {{os}}* foi concluída com sucesso!\n\n' +
      '❄️ Serviço: {{servico}}\n' +
      '📅 Data: {{data}}\n' +
      '🛡️ *Garantia: {{garantia}}*\n\n' +
      '📄 A OS completa em PDF (comprovante e garantia) está anexada.\n\n' +
      'Obrigado pela confiança! *{{empresa}}* ❄️\n\n' +
      '📲 Acesse seu portal: {{app}}'
  },
  {
    chave: 'agendamento_criado',
    titulo: 'Agendamento criado (retorno pelo painel)',
    quando: 'Técnico agenda um retorno',
    placeholders: ['{{cliente}}', '{{data}}', '{{servico}}', '{{empresa}}'],
    padrao:
      'Olá, {{cliente}}! 📅 Seu atendimento ficou *AGENDADO* para *{{data}}* — {{servico}}. Qualquer imprevisto, é só nos chamar! *{{empresa}}* ❄️'
  },
  {
    chave: 'chamado_agendado',
    titulo: 'Chamado agendado (data e hora)',
    quando: 'Técnico agenda um chamado do cliente',
    placeholders: ['{{cliente}}', '{{servico}}', '{{data}}', '{{hora}}', '{{empresa}}', '{{app}}'],
    padrao:
      '📅 *Atendimento AGENDADO!*\n\nOlá, {{cliente}}! Seu {{servico}} foi agendado para *{{data}} às {{hora}}*.\n\n' +
      '📲 Acompanhe tudo pelo app: {{app}}\n\n*{{empresa}}* ❄️'
  },
  {
    chave: 'orcamento_agendado',
    titulo: 'Orçamento aprovado → agendado',
    quando: 'Serviço do orçamento aprovado é agendado',
    placeholders: ['{{cliente}}', '{{data}}', '{{hora}}', '{{valor}}', '{{empresa}}', '{{app}}'],
    padrao:
      '📅 *Atendimento AGENDADO!*\n\nOlá, {{cliente}}! Seu serviço do orçamento aprovado foi agendado para *{{data}} às {{hora}}* (valor {{valor}}).\n\n' +
      '📲 Acompanhe pelo app: {{app}}\n\n*{{empresa}}* ❄️'
  },
  {
    chave: 'servico_registrado',
    titulo: 'Serviço registrado (cadastro direto)',
    quando: 'Técnico cadastra um serviço sem checklist',
    placeholders: ['{{cliente}}', '{{servico}}', '{{data}}', '{{empresa}}'],
    padrao:
      '📅 *Serviço registrado!* Olá, {{cliente}}! Seu {{servico}} ficou agendado para *{{data}}*. *{{empresa}}* ❄️'
  },
  {
    chave: 'status_agendado',
    titulo: 'Status: AGENDADO',
    quando: 'Status do serviço muda para agendado',
    placeholders: ['{{cliente}}', '{{servico}}', '{{empresa}}'],
    padrao: '📅 Seu serviço ({{servico}}) foi *AGENDADO* pela {{empresa}}. Em breve confirmaremos a data exata! ❄️'
  },
  {
    chave: 'status_em_andamento',
    titulo: 'Status: EM ANDAMENTO',
    quando: 'Técnico está a caminho / executando',
    placeholders: ['{{cliente}}', '{{servico}}', '{{empresa}}'],
    padrao: '🔧 *Técnico a caminho / em execução!* Seu serviço ({{servico}}) está em andamento agora. ❄️'
  },
  {
    chave: 'status_concluido',
    titulo: 'Status: CONCLUÍDO',
    quando: 'Serviço marcado como concluído',
    placeholders: ['{{cliente}}', '{{servico}}', '{{empresa}}'],
    padrao: '✅ Seu serviço ({{servico}}) foi *CONCLUÍDO* com garantia ativa. Obrigado pela confiança! ❄️'
  },
  {
    chave: 'status_cancelado',
    titulo: 'Status: CANCELADO',
    quando: 'Serviço cancelado',
    placeholders: ['{{cliente}}', '{{servico}}', '{{empresa}}'],
    padrao: '⚠️ Seu serviço ({{servico}}) foi cancelado. Fale conosco para reagendarmos! ❄️'
  },
  {
    chave: 'os_status_atualizado',
    titulo: 'OS atualizada (edição da OS)',
    quando: 'Técnico edita a OS e conclui por lá',
    placeholders: ['{{cliente}}', '{{status}}', '{{empresa}}'],
    padrao: '✅ Seu serviço foi atualizado: *{{status}}* — novo status no app. *{{empresa}}* ❄️'
  },
  {
    chave: 'reagendado',
    titulo: 'Atendimento reagendado',
    quando: 'Data do atendimento é alterada',
    placeholders: ['{{cliente}}', '{{servico}}', '{{data}}', '{{empresa}}'],
    padrao:
      '📅 *Atendimento REAGENDADO*\n\nOlá, {{cliente}}! Seu atendimento ({{servico}}) foi reagendado para *{{data}}*.\n\nQualquer dúvida, é só nos chamar! *{{empresa}}* ❄️'
  },
  {
    chave: 'orcamento_aprovado_cliente',
    titulo: 'Orçamento aprovado (confirmação ao cliente)',
    quando: 'Cliente aprova a proposta com assinatura',
    placeholders: ['{{cliente}}', '{{empresa}}', '{{numero}}', '{{valor}}', '{{app}}'],
    padrao:
      '✅ *Recebemos sua aprovação*, {{cliente}}!\n\n' +
      'Proposta nº {{numero}} ({{valor}}) confirmada com assinatura digital.\n\n' +
      'Nossa equipe vai entrar em contato para agendar o serviço. Qualquer dúvida, chame por aqui! ❄️\n\n' +
      '*{{empresa}}*'
  },
  {
    chave: 'orcamento_aprovado_tecnico',
    titulo: 'Aviso: orçamento aprovado (para a empresa)',
    quando: 'Cliente aprova — avisa o técnico/empresa',
    placeholders: ['{{cliente}}', '{{numero}}', '{{empresa}}'],
    padrao:
      '✅ *Orçamento aprovado e assinado!*\n\n' +
      'O cliente *{{cliente}}* aprovou a proposta (assinatura registrada). Hora de agendar o serviço!\n\n' +
      '— InovarApp'
  },
  {
    chave: 'orcamento_recusado_tecnico',
    titulo: 'Aviso: orçamento recusado (para a empresa)',
    quando: 'Cliente recusa a proposta',
    placeholders: ['{{cliente}}', '{{numero}}', '{{empresa}}'],
    padrao:
      '⚠️ *Orçamento recusado.*\n\n' +
      'O cliente *{{cliente}}* recusou a proposta.\n\n' +
      '— InovarApp'
  },
  {
    chave: 'lembrete_ciclo_vencido',
    titulo: 'Lembrete: ciclo de manutenção vencido',
    quando: 'Cron diário 09h — clientes com ciclo em atraso/vencendo',
    placeholders: ['{{cliente}}', '{{empresa}}', '{{equipamento}}', '{{data_ultima}}', '{{situacao}}', '{{meses}}'],
    padrao:
      'Olá, {{cliente}}! Tudo bem? 😊\n\n' +
      'Aqui é a *{{empresa}}*.\n\n' +
      '❄️ Pela nossa ficha técnica, a manutenção/limpeza de ar do seu *{{equipamento}}* realizada em {{data_ultima}} já {{situacao}} (ciclo recomendado de {{meses}} meses).\n\n' +
      'Manter o ciclo evita fungos, bactérias, mau cheiro e maior consumo de energia. Posso agendar sua visita? 🗓️\n\n' +
      '{{empresa}}'
  },
  {
    chave: 'lembrete_vespera',
    titulo: 'Lembrete: atendimento é amanhã',
    quando: 'Cron diário 09h — véspera do atendimento',
    placeholders: ['{{cliente}}', '{{empresa}}', '{{data}}', '{{hora}}', '{{servico}}', '{{equipamento}}'],
    padrao:
      'Olá, {{cliente}}! Tudo bem? 😊\n\n' +
      '*{{empresa}}* passando para lembrar:\n\n' +
      '📅 Seu atendimento está *AGENDADO PARA AMANHÃ* ({{data}}{{hora}}).\n' +
      '❄️ Serviço: {{servico}}\n' +
      '🏠 Equipamento: {{equipamento}}\n\n' +
      'Qualquer imprevisto, é só nos chamar por aqui! Estamos à disposição. ❄️'
  }
];

export function textoMensagem(salvas: Record<string, string> | undefined | null, chave: string): string {
  if (salvas && salvas[chave] && salvas[chave].trim()) return salvas[chave];
  return MODELOS_MENSAGENS.find((m) => m.chave === chave)?.padrao || '';
}

export function aplicarPlaceholders(texto: string, vars: Record<string, string | number | undefined | null>): string {
  return (texto || '').replace(/\{\{(\w+)\}\}/g, (_, k: string) => {
    const v = vars[k];
    return v === undefined || v === null ? '' : String(v);
  });
}

export const URL_APP = APP;
