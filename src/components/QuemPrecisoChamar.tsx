import React, { useEffect, useState } from 'react';
import { Client, Appliance, MaintenanceRecord, TechnicianProfile, ReturnStatus, BudgetEstimate } from '../types';
import { WhatsAppService } from '../services/whatsapp';
import { Phone, MessageCircle, ChevronRight, AlertCircle, Clock, Calendar, CheckCircle2, Search, Wrench, Shield, Sparkles, FileText, History, Calculator, XCircle } from 'lucide-react';
import { format, parseISO } from 'date-fns';

interface QueueItem {
  client: Client;
  appliance: Appliance;
  lastMaintenance?: MaintenanceRecord;
  scheduledMaintenance?: MaintenanceRecord;
  returnStatus: ReturnStatus;
  returnDate: string;
  historico?: MaintenanceRecord[];
  orcamento?: BudgetEstimate; // orçamento que colocou o cliente na fila
  deOrcamento?: boolean; // item gerado a partir de orçamento (sem OS ainda)
  chamados?: any[]; // solicitações PENDENTES deste cliente+aparelho
  emExecucao?: any[]; // serviços EM_ANDAMENTO agora
}

interface QuemPrecisoChamarProps {
  queueItems: QueueItem[];
  chamadosContent?: React.ReactNode;
  historicoContent?: React.ReactNode;
  chamadosCount?: number;
  historicoCount?: number;
  profile: TechnicianProfile;
  maintenances?: MaintenanceRecord[];
  onOpenChecklist: (client: Client, appliance: Appliance) => void;
  onOpenFicha: (client: Client, appliance: Appliance) => void;
  onOpenAgendamento: (client: Client, appliance: Appliance) => void;
  onMarkContacted?: (clientId: string, applianceId: string) => void;
  onGerarOrcamento?: (client: Client) => void;
  onAbrirOS?: (client: Client, appliance: Appliance, record: MaintenanceRecord) => void;
  onRegistrarHistorico?: (client: Client, appliance: Appliance) => void;
  onUpdateStatus?: (serviceId: string, status: string) => void;
  onFinalizarServico?: (serviceId: string) => void;
  onAgendarOrcamento?: (budget: BudgetEstimate) => void;
  onAtenderChamado?: (serviceId: string) => void;
  servicesLive?: any[];
  budgets?: BudgetEstimate[];
  onAbrirOrcamento?: (budget: BudgetEstimate) => void;
  activeFilter?: 'todos' | 'chamados' | 'atrasado' | 'esta_semana' | 'em_breve' | 'sem_historico';
  onFilterChange?: (filter: 'todos' | 'chamados' | 'atrasado' | 'esta_semana' | 'em_breve' | 'sem_historico') => void;
  sectionRequest?: 'fila' | 'chamados' | 'historico';
}

export const QuemPrecisoChamar: React.FC<QuemPrecisoChamarProps> = ({
  queueItems,
  chamadosContent, historicoContent, chamadosCount = 0, historicoCount = 0,
  profile,
  maintenances,
  onOpenChecklist,
  onOpenFicha,
  onOpenAgendamento,
  onMarkContacted,
  onGerarOrcamento,
  onAbrirOS,
  onRegistrarHistorico,
  servicesLive,
  budgets,
  onAbrirOrcamento,
  onUpdateStatus,
  onFinalizarServico,
  onAgendarOrcamento,
  onAtenderChamado,
  activeFilter,
  onFilterChange,
  sectionRequest
}) => {
  const [internalFilter, setInternalFilter] = useState<'todos' | 'chamados' | 'atrasado' | 'esta_semana' | 'em_breve' | 'sem_historico'>('todos');
  const filter = activeFilter !== undefined ? activeFilter : internalFilter;
  const setFilter = (f: 'todos' | 'chamados' | 'atrasado' | 'esta_semana' | 'em_breve' | 'sem_historico') => {
    setInternalFilter(f);
    if (onFilterChange) onFilterChange(f);
  };
  const [section, setSection] = useState<'fila' | 'chamados' | 'historico'>('fila');
  useEffect(() => {
    if (sectionRequest) setSection(sectionRequest);
  }, [sectionRequest]);
  const [search, setSearch] = useState('');
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const selectedItem = queueItems.find(item => `${item.client.id}:${item.appliance.id}` === selectedKey) || null;
  const setSelectedItem = (item: QueueItem | null) => setSelectedKey(item ? `${item.client.id}:${item.appliance.id}` : null);

  const safeFormat = (dateStr: string | undefined | null, pattern: string, fallback = '—'): string => {
    if (!dateStr) return fallback;
    const d = parseISO(dateStr);
    return !isNaN(d.getTime()) ? format(d, pattern) : fallback;
  };

  const filteredItems = queueItems.filter(item => {
    // "Todos" mostra a operação do dia: retornos, chamados, execução e orçamentos ativos;
    // "1º Atendimento" (sem histórico) fica no filtro próprio
    const temChamado = (item.chamados?.length || 0) > 0;
    const emExec = (item.emExecucao?.length || 0) > 0;
    if (filter === 'todos' && item.returnStatus === 'sem_historico' && !temChamado && !emExec) return false;
    if (filter === 'chamados' && !temChamado) return false;
    if (filter !== 'todos' && filter !== 'chamados' && item.returnStatus !== filter) return false;
    if (search.trim()) {
      const q = search.toLowerCase();
      const matchName = String(item.client.name || '').toLowerCase().includes(q);
      const matchRoom = String(item.appliance.room || '').toLowerCase().includes(q);
      const matchBrand = String(item.appliance.brand || '').toLowerCase().includes(q);
      const matchBtu = String(item.appliance.capacityBtu || '').toLowerCase().includes(q);
      const matchPhone = String(item.client.phone || '').includes(q);
      return matchName || matchRoom || matchBrand || matchBtu || matchPhone;
    }
    return true;
  }).sort((a, b) => {
    const aScheduled = a.scheduledMaintenance ? 0 : a.lastMaintenance ? 1 : 2;
    const bScheduled = b.scheduledMaintenance ? 0 : b.lastMaintenance ? 1 : 2;
    if (aScheduled !== bScheduled) return aScheduled - bScheduled;
    return (a.returnDate || '9999-12-31').localeCompare(b.returnDate || '9999-12-31');
  });

  const countTodos = queueItems.filter(i => i.returnStatus !== 'sem_historico' || (i.chamados?.length || 0) > 0 || (i.emExecucao?.length || 0) > 0).length;
  const countChamados = queueItems.filter(i => (i.chamados?.length || 0) > 0).length;
  const countAtrasados = queueItems.filter(i => i.returnStatus === 'atrasado').length;
  const countEstaSemana = queueItems.filter(i => i.returnStatus === 'esta_semana').length;
  const countEmBreve = queueItems.filter(i => i.returnStatus === 'em_breve').length;
  const countSemHistorico = queueItems.filter(i => i.returnStatus === 'sem_historico').length;

  // Histórico completo do aparelho selecionado (fila já traz ordenado do mais recente)
  const historicoSelecionado = selectedItem
    ? (selectedItem.historico || (maintenances || [])
        .filter((m) => m.applianceId === selectedItem.appliance.id)
        .sort((a, b) => new Date(b.date).getTime() - new Date(a.date).getTime()))
    : [];

  const handleWhatsApp = (item: QueueItem) => {
    let msg = '';
    const firstName = item.client.name.split(' ')[0];
    const techName = profile.name || 'Inovar Refrigeração';
    const appDesc = `${item.appliance.type || 'Ar-condicionado'} ${item.appliance.capacityBtu ? item.appliance.capacityBtu + ' BTUs' : ''} (${item.appliance.room || 'ambiente'})`.trim();

    if (item.scheduledMaintenance) {
      const dataFmt = safeFormat(item.scheduledMaintenance.scheduledDate || item.scheduledMaintenance.date, 'dd/MM/yyyy');
      const horaFmt = item.scheduledMaintenance.scheduledTime ? ` às ${item.scheduledMaintenance.scheduledTime}` : '';
      msg = `Olá, *${firstName}*! Tudo bem? 😊\n\nAqui é o *${techName}*.\n\nPassando para confirmar o nosso atendimento agendado para o dia *${dataFmt}*${horaFmt}, referente ao seu *${appDesc}*.\n\nQualquer dúvida ou ajuste de horário, estou à disposição! ❄️🔧`;
    } else if ((item.chamados?.length || 0) > 0) {
      const desc = item.chamados![0].descricao || item.chamados![0].tipo || 'atendimento';
      msg = `Olá, *${firstName}*! Tudo bem? 😊\n\nAqui é o *${techName}*.\n\nRecebi sua solicitação de *${desc}* para o seu *${appDesc}*. Gostaria de agendar o melhor dia e horário para realizar o atendimento.\n\nQual período fica melhor para você: *manhã* ou *tarde*? ❄️🔧`;
    } else if (item.orcamento && item.orcamento.status === 'pendente') {
      msg = `Olá, *${firstName}*! Tudo bem? 😊\n\nAqui é o *${techName}*.\n\nEstou entrando em contato para saber se você conseguiu avaliar a proposta de orçamento #${item.orcamento.numero || ''} (R$ ${(item.orcamento.finalValue || 0).toFixed(2)}) para o seu ar-condicionado.\n\nFico à disposição se tiver alguma dúvida ou quiser aprovar para agendarmos o serviço! ❄️📄`;
    } else if (item.returnStatus === 'sem_historico') {
      msg = `Olá, *${firstName}*! Tudo bem? 😊\n\nAqui é o *${techName}* da *Inovar Refrigeração*.\n\nVi que temos o cadastro do seu ar-condicionado (*${appDesc}*). Para garantir ar puro, economia de energia e máximo rendimento, que tal agendarmos uma revisão ou limpeza de ar preventiva?\n\nQual dia fica melhor para você? ❄️🔧`;
    } else {
      msg = WhatsAppService.generateReturnMessage(item.client, item.appliance, item.lastMaintenance, profile);
    }

    WhatsAppService.openWhatsApp(item.client.phone, msg);
    if (onMarkContacted) {
      onMarkContacted(item.client.id, item.appliance.id);
    }
  };

  const handleCall = (item: QueueItem) => {
    if (onMarkContacted) {
      onMarkContacted(item.client.id, item.appliance.id);
    }
    window.location.href = `tel:${item.client.phone}`;
  };

  return (
    <div id="fila-de-retornos" className="bg-white text-slate-900 rounded-2xl shadow-xl border border-slate-200 overflow-hidden">
      {/* Card Header */}
      <div className="bg-slate-50 p-4 border-b border-slate-200">
        <div className="flex flex-wrap items-center justify-between gap-3 mb-3">
          <div className="flex items-center gap-2">
            <div className="p-2 bg-blue-100 text-blue-700 rounded-xl">
              <Phone className="w-5 h-5" />
            </div>
            <div>
              <h2 className="text-base font-bold text-slate-800">Central de Atendimento</h2>
              <p className="text-xs text-slate-500">Retornos, chamados e ordens de serviço em um único lugar</p>
            </div>
          </div>
          <span className="text-xs font-bold px-2.5 py-1 bg-blue-50 text-blue-700 border border-blue-200 rounded-full">
            {filteredItems.length} na fila
          </span>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-2 mb-4" role="tablist" aria-label="Seções da Fila de Retorno">
          {([['fila', 'Retornos', queueItems.length], ['chamados', 'Chamados', chamadosCount], ['historico', 'Ordens de Serviço', historicoCount]] as const).map(([id, label, count]) => (
            <button key={id} type="button" role="tab" aria-selected={section === id} aria-controls={'fila-painel-' + id} id={'fila-aba-' + id}
              onClick={() => setSection(id)}
              className={'px-4 py-3 rounded-xl text-sm font-bold flex items-center justify-between gap-2 transition-colors ' + (section === id ? 'bg-sky-600 text-white shadow-sm' : 'bg-white border border-slate-300 text-slate-700 hover:bg-blue-50')}>
              {label}<span className="rounded-full bg-black/10 px-2 py-0.5 text-xs">{count}</span>
            </button>
          ))}
        </div>
        {section === 'fila' && <>
        <div className="relative mb-3">
          <Search className="w-4 h-4 text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            placeholder="Buscar por cliente, aparelho ou cômodo..."
            value={search}
            onChange={e => setSearch(e.target.value)}
            className="w-full pl-9 pr-3 py-2 bg-white border border-slate-300 rounded-xl text-xs text-slate-800 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent transition-all"
          />
        </div>

        <div className="flex gap-1.5 overflow-x-auto pb-1 text-xs [scrollbar-width:none] [&::-webkit-scrollbar]:hidden -mx-1 px-1">
          <button
            onClick={() => setFilter('todos')}
            className={`px-3 py-1.5 rounded-lg font-semibold transition-all shrink-0 whitespace-nowrap ${
              filter === 'todos' ? 'bg-slate-800 text-white shadow-sm' : 'bg-white text-slate-600 border border-slate-200 hover:bg-slate-100'
            }`}
          >
            Todos ({countTodos})
          </button>
          <button
            onClick={() => setFilter('chamados')}
            className={`px-3 py-1.5 rounded-lg font-semibold transition-all shrink-0 flex items-center gap-1.5 whitespace-nowrap ${
              filter === 'chamados' ? 'bg-amber-600 text-white shadow-sm' : 'bg-amber-50 text-amber-800 border border-amber-200 hover:bg-amber-100'
            }`}
            title="Solicitações de clientes aguardando agendamento"
          >
            <AlertCircle className="w-3.5 h-3.5" />
            Chamados ({countChamados})
          </button>
          <button
            onClick={() => setFilter('atrasado')}
            className={`px-3 py-1.5 rounded-lg font-semibold transition-all shrink-0 flex items-center gap-1.5 whitespace-nowrap ${
              filter === 'atrasado' ? 'bg-red-600 text-white shadow-sm' : 'bg-red-50 text-red-700 border border-red-200 hover:bg-red-100'
            }`}
          >
            <span className="w-2 h-2 rounded-full bg-red-500" aria-hidden="true"></span>
            Atrasado ({countAtrasados})
          </button>
          <button
            onClick={() => setFilter('esta_semana')}
            className={`px-3 py-1.5 rounded-lg font-semibold transition-all shrink-0 flex items-center gap-1.5 whitespace-nowrap ${
              filter === 'esta_semana' ? 'bg-amber-600 text-white shadow-sm' : 'bg-amber-50 text-amber-800 border border-amber-200 hover:bg-amber-100'
            }`}
          >
            <Clock className="w-3.5 h-3.5" />
            Esta Semana ({countEstaSemana})
          </button>
          <button
            onClick={() => setFilter('em_breve')}
            className={`px-3 py-1.5 rounded-lg font-semibold transition-all shrink-0 flex items-center gap-1.5 whitespace-nowrap ${
              filter === 'em_breve' ? 'bg-blue-600 text-white shadow-sm' : 'bg-blue-50 text-blue-700 border border-blue-200 hover:bg-blue-100'
            }`}
          >
            <Calendar className="w-3.5 h-3.5" />
            Em Breve ({countEmBreve})
          </button>
          <button
            onClick={() => setFilter('sem_historico')}
            className={`px-3 py-1.5 rounded-lg font-semibold transition-all shrink-0 flex items-center gap-1.5 whitespace-nowrap ${
              filter === 'sem_historico' ? 'bg-violet-600 text-white shadow-sm' : 'bg-violet-50 text-violet-700 border border-violet-200 hover:bg-violet-100'
            }`}
            title="Aparelhos que nunca receberam nenhum serviço — cadastre o histórico anterior para entrar no ciclo"
          >
            <Search className="w-3.5 h-3.5" />
            Sem Histórico ({countSemHistorico})
          </button>
        </div>
        </>}
      </div>

      {section === 'chamados' && <div role="tabpanel" id="fila-painel-chamados" aria-labelledby="fila-aba-chamados" className="p-4 bg-slate-50"><h3 className="text-slate-800 font-bold mb-3">Chamados dos clientes</h3>{chamadosContent}</div>}
      {section === 'historico' && <div role="tabpanel" id="fila-painel-historico" aria-labelledby="fila-aba-historico" className="p-2 sm:p-4 bg-slate-50">{historicoContent}</div>}
      {/* Lista */}
      {section === 'fila' && <div role="tabpanel" id="fila-painel-fila" aria-labelledby="fila-aba-fila" className="divide-y divide-slate-100 max-h-[70vh] overflow-y-auto">
        {filteredItems.length === 0 ? (
          <div className="p-8 text-center">
            <CheckCircle2 className="w-12 h-12 text-emerald-500 mx-auto mb-2 opacity-80" />
            <h4 className="font-semibold text-slate-700 text-sm">Nenhum retorno pendente!</h4>
            <p className="text-xs text-slate-400 mt-1">Todos os seus clientes estão em dia ou não correspondem ao filtro.</p>
          </div>
        ) : (
          filteredItems.map((item, itemIndex) => {
            const isSelected = selectedItem?.client.id === item.client.id && selectedItem?.appliance.id === item.appliance.id;
            const section = item.scheduledMaintenance ? 'agendamentos' : item.lastMaintenance ? 'retornos' : 'sem_historico';
            const previous = filteredItems[itemIndex - 1];
            const previousSection = previous ? (previous.scheduledMaintenance ? 'agendamentos' : previous.lastMaintenance ? 'retornos' : 'sem_historico') : null;

            // dados ao vivo: agendamento e orçamentos do cliente
            const ags = (servicesLive || []).filter(
              (s: any) => s.cliente_id === item.client.id && s.aparelho_id === item.appliance.id && s.status === 'AGENDADO'
            );
            const emExecucao = (servicesLive || []).filter(
              (s: any) => s.cliente_id === item.client.id && s.aparelho_id === item.appliance.id && s.status === 'EM_ANDAMENTO'
            );
            const orcamentosCliente = (budgets || []).filter((b) => b.clientId === item.client.id);

            return (
              <React.Fragment key={`${item.client.id}-${item.appliance.id}`}>
              {section !== previousSection && (
                <div className="px-4 py-2 bg-slate-100 border-y border-slate-200 text-[10px] font-extrabold uppercase tracking-wider text-slate-500">
                  {section === 'agendamentos' ? 'Próximos agendamentos' : section === 'retornos' ? 'Próximos retornos pelo histórico' : 'Aparelhos sem histórico'}
                </div>
              )}
              <div
                className={`transition-colors duration-150 ${isSelected ? 'bg-blue-50/70' : 'hover:bg-slate-50/80'}`}
              >
                {/* Linha do cliente */}
                <div
                  onClick={() => setSelectedItem(isSelected ? null : item)}
                  className="p-3.5 flex flex-col sm:flex-row sm:items-center sm:justify-between cursor-pointer select-none gap-2"
                >
                  <div className="flex items-center gap-3 min-w-0 flex-1">
                    <div
                      className={`w-9 h-9 rounded-full flex items-center justify-center shrink-0 shadow-sm ${
                        item.returnStatus === 'atrasado'
                          ? 'bg-red-100 text-red-600'
                          : item.returnStatus === 'esta_semana'
                          ? 'bg-amber-100 text-amber-700'
                          : item.returnStatus === 'sem_historico'
                          ? 'bg-violet-100 text-violet-700'
                          : 'bg-sky-100 text-sky-700'
                      }`}
                    >
                      <Phone className="w-4 h-4" />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-1.5 flex-wrap">
                        <span className="font-bold text-slate-900 text-sm truncate">{item.client.name}</span>
                        {(item.chamados?.length || 0) > 0 && (
                          <span title="Chamado(s) aguardando agendamento" className="px-1.5 py-0.5 bg-amber-100 text-amber-700 rounded text-[9px] font-bold border border-amber-200">
                            🔔 CHAMADO {(item.chamados || []).length > 1 ? `×${item.chamados!.length}` : ''}
                          </span>
                        )}
                        {(item.emExecucao?.length || 0) > 0 && (
                          <span title="Serviço em execução agora" className="px-1.5 py-0.5 bg-violet-100 text-violet-700 rounded text-[9px] font-bold border border-violet-200">
                            🔧 EM ANDAMENTO
                          </span>
                        )}
                        {item.scheduledMaintenance && (
                          <span title="Atendimento agendado" className="px-1.5 py-0.5 bg-blue-100 text-blue-700 rounded text-[9px] font-bold border border-blue-200">
                            📅 AGENDADO
                          </span>
                        )}
                        {item.orcamento && (
                          <span title={item.orcamento.status === 'aprovado' ? 'Orçamento aprovado' : 'Orçamento aguardando resposta'} className={`px-1.5 py-0.5 rounded text-[9px] font-bold border ${item.orcamento.status === 'aprovado' ? 'bg-emerald-100 text-emerald-700 border-emerald-200' : 'bg-sky-100 text-sky-700 border-sky-200'}`}>
                            📄 {item.orcamento.status === 'aprovado' ? 'APROVADO' : 'ORÇ.'}
                          </span>
                        )}
                        {item.lastMaintenance && (
                          <span title="Ordem de Serviço já gerada" className="px-1.5 py-0.5 bg-slate-100 text-slate-600 rounded text-[9px] font-bold border border-slate-200">
                            Ordem de Serviço ✓
                          </span>
                        )}
                        {item.lastMaintenance?.contactedAt && (
                          <span title={`Último contato registrado em ${item.lastMaintenance.contactedAt}`} className="px-1.5 py-0.5 bg-emerald-50 text-emerald-700 rounded text-[9px] font-bold border border-emerald-200">
                            💬 Contatado ({item.lastMaintenance.contactedAt})
                          </span>
                        )}
                      </div>
                      {item.orcamento ? (
                        <p className="text-xs text-slate-500 truncate mt-0.5">
                          <span className="font-medium text-slate-700">📄 {item.orcamento.items?.[0]?.description || item.orcamento.applianceDesc || 'Orçamento'}</span>
                          <span> • R$ {(item.orcamento.finalValue || 0).toFixed(2)}</span>
                        </p>
                      ) : (
                        <p className="text-xs text-slate-500 truncate mt-0.5">
                          <span className="font-medium text-slate-700">{item.appliance.brand} {item.appliance.capacityBtu} BTUs</span>
                          <span> • </span>
                          <span>{item.appliance.room}</span>
                        </p>
                      )}
                      {(item.chamados?.length || 0) > 0 && !item.returnDate && (
                        <p className="text-[10px] text-amber-700 font-semibold mt-0.5">
                          🔔 Chamado aguardando agendamento — toque em "Atender chamado"
                        </p>
                      )}
                      {(item.emExecucao?.length || 0) > 0 && (
                        <p className="text-[10px] text-violet-700 font-semibold mt-0.5">
                          🔧 Serviço em execução agora — finalize ao concluir o trabalho
                        </p>
                      )}
                      {item.orcamento ? (
                        <p className="text-[10px] font-semibold mt-0.5">
                          <span className={item.orcamento.status === 'aprovado' ? 'text-emerald-600' : 'text-amber-600'}>
                            {item.orcamento.status === 'aprovado' ? '✅ Aprovado — agendar serviço' : '⏳ Orçamento aguardando resposta'}
                          </span>
                          {item.returnDate && (
                            <span className="text-sky-700"> • {item.orcamento.status === 'pendente' ? 'vence em ' : ''}{safeFormat(item.returnDate, 'dd/MM/yyyy')}</span>
                          )}
                        </p>
                      ) : item.returnDate && (
                        <p className="text-[10px] text-sky-700 font-semibold mt-0.5">
                          {item.scheduledMaintenance ? 'Próximo agendamento' : 'Próximo retorno'}: {safeFormat(item.returnDate, 'dd/MM/yyyy')}
                        </p>
                      )}
                    </div>
                  </div>

                  <div className="flex items-center shrink-0 self-end sm:self-auto">
                    {item.returnStatus === 'atrasado' && (
                      <span className="px-2.5 py-1 bg-red-500 text-white rounded-md text-[10px] font-bold tracking-wide flex items-center gap-1 uppercase shadow-sm whitespace-nowrap">
                        ATRASADO <ChevronRight className="w-3 h-3 shrink-0" />
                      </span>
                    )}
                    {item.returnStatus === 'esta_semana' && (
                      <span className="px-2.5 py-1 bg-amber-400 text-amber-950 rounded-md text-[10px] font-bold tracking-wide flex items-center gap-1 uppercase shadow-sm whitespace-nowrap">
                        ESTA SEMANA <ChevronRight className="w-3 h-3 shrink-0" />
                      </span>
                    )}
                    {item.returnStatus === 'em_breve' && (
                      <span className="px-2.5 py-1 bg-sky-500 text-white rounded-md text-[10px] font-bold tracking-wide flex items-center gap-1 uppercase shadow-sm whitespace-nowrap">
                        EM BREVE <ChevronRight className="w-3 h-3 shrink-0" />
                      </span>
                    )}
                    {(item.emExecucao?.length || 0) > 0 ? (
                      <span className="px-2.5 py-1 bg-violet-500 text-white rounded-md text-[10px] font-bold tracking-wide flex items-center gap-1 uppercase shadow-sm whitespace-nowrap">
                        🔧 AGORA
                      </span>
                    ) : item.returnStatus === 'sem_historico' && (
                      <span className="px-2.5 py-1 bg-violet-100 text-violet-800 border border-violet-300 rounded-md text-[10px] font-bold tracking-wide flex items-center gap-1 uppercase whitespace-nowrap">
                        SEM HISTÓRICO <ChevronRight className="w-3 h-3 shrink-0" />
                      </span>
                    )}
                    {item.returnStatus === 'em_dia' && (
                      <span className="px-2.5 py-1 bg-emerald-100 text-emerald-800 rounded-md text-[10px] font-bold tracking-wide flex items-center gap-1 uppercase whitespace-nowrap">
                        EM DIA <CheckCircle2 className="w-3 h-3 shrink-0" />
                      </span>
                    )}
                  </div>
                </div>

                {/* AÇÕES RÁPIDAS — sempre visíveis na primeira página */}
                <div
                  className="px-3.5 pb-2.5 -mt-0.5 grid grid-cols-2 sm:flex sm:items-center gap-1.5"
                  onClick={(e) => e.stopPropagation()}
                >
                  {/* Contextuais primeiro: em execução / chamado / orçamento aprovado */}
                  {(item.emExecucao?.length || 0) > 0 && onFinalizarServico && (
                    <button
                      onClick={() => onFinalizarServico(item.emExecucao![0].id)}
                      title="Finalizar o serviço em execução (checklist, valor e garantia)"
                      className="px-2 py-1.5 bg-emerald-600 hover:bg-emerald-500 text-white rounded-lg text-[10px] font-bold flex items-center gap-1 transition-all active:scale-95 shadow-sm"
                    >
                      ✓ Finalizar
                    </button>
                  )}
                  {(item.emExecucao?.length || 0) > 0 && onUpdateStatus && (
                    <button
                      onClick={() => { if (confirm('Cancelar este serviço?')) onUpdateStatus(item.emExecucao![0].id, 'CANCELADO'); }}
                      title="Cancelar serviço em execução"
                      className="p-1.5 bg-red-50 hover:bg-red-100 text-red-500 rounded-lg border border-red-100 transition-colors"
                    >
                      <XCircle className="w-3.5 h-3.5" />
                    </button>
                  )}
                  {(item.chamados?.length || 0) > 0 && onAtenderChamado && (
                    <button
                      onClick={() => onAtenderChamado(item.chamados![0].id)}
                      title="Agendar data e hora do chamado do cliente"
                      className="px-2 py-1.5 bg-amber-500 hover:bg-amber-400 text-white rounded-lg text-[10px] font-bold flex items-center gap-1 transition-all active:scale-95 shadow-sm"
                    >
                      🔔 Atender chamado
                    </button>
                  )}
                  {onGerarOrcamento && !item.orcamento && (
                    <button
                      onClick={() => onGerarOrcamento(item.client)}
                      title="Gerar orçamento para este cliente"
                      className="min-h-10 px-2.5 py-2 bg-inovar-yellow hover:brightness-105 text-inovar-navy rounded-lg text-[10px] font-extrabold flex items-center justify-center gap-1 transition-all active:scale-95 shadow-sm"
                    >
                      <Calculator className="w-3 h-3" />
                      Orçamento
                    </button>
                  )}
                  {item.orcamento && onAbrirOrcamento && (
                    <button
                      onClick={() => onAbrirOrcamento(item.orcamento!)}
                      title="Abrir a proposta deste cliente"
                      className="min-h-10 px-2.5 py-2 bg-white hover:bg-slate-100 border border-slate-300 text-slate-700 rounded-lg text-[10px] font-bold flex items-center justify-center gap-1 transition-all active:scale-95"
                    >
                      <FileText className="w-3 h-3" />
                      Orçamento
                    </button>
                  )}
                  <button
                    onClick={() => item.orcamento?.status === 'aprovado' && onAgendarOrcamento
                      ? onAgendarOrcamento(item.orcamento)
                      : onOpenAgendamento(item.client, item.appliance)}
                    title={item.orcamento?.status === 'aprovado' ? 'Agendar o serviço do orçamento aprovado' : 'Agendar retorno'}
                    className="min-h-10 px-2.5 py-2 bg-white hover:bg-slate-100 border border-slate-300 text-slate-700 rounded-lg text-[10px] font-semibold flex items-center justify-center gap-1 transition-all active:scale-95"
                  >
                    <Calendar className="w-3 h-3 text-slate-500" />
                    Agendar
                  </button>
                  <button
                    onClick={() => onOpenChecklist(item.client, item.appliance)}
                    title="Abrir Ordem de Serviço"
                    className="min-h-10 px-2.5 py-2 bg-sky-600 hover:bg-sky-500 text-white rounded-lg text-[10px] font-bold flex items-center justify-center gap-1 transition-all active:scale-95 shadow-sm"
                  >
                    <Wrench className="w-3 h-3" />
                    Ordem de Serviço
                  </button>
                  {item.client.phone && (
                    <>
                      <button
                        onClick={() => handleWhatsApp(item)}
                        title="WhatsApp com mensagem pronta (marca contato feito)"
                        className="min-h-10 px-2.5 py-2 bg-emerald-600 hover:bg-emerald-500 text-white rounded-lg text-[10px] font-bold flex items-center justify-center gap-1 transition-all active:scale-95 shadow-sm"
                      >
                        <MessageCircle className="w-3 h-3" />
                        WhatsApp
                      </button>
                      <button
                        onClick={() => handleCall(item)}
                        title="Ligar para o cliente"
                        className="min-h-10 px-2.5 py-2 bg-blue-600 hover:bg-blue-500 text-white rounded-lg text-[10px] font-bold flex items-center justify-center gap-1 transition-all active:scale-95 shadow-sm"
                      >
                        <Phone className="w-3 h-3" />
                        Ligar
                      </button>
                    </>
                  )}
                </div>

                {/* Drawer expandido */}
                {isSelected && (
                  <div className="px-4 pb-4 pt-1 bg-blue-50/50 border-t border-blue-100 text-xs">
                    {/* Faixa do orçamento que colocou o cliente na fila */}
                    {item.orcamento && (
                      <div className="mb-3 p-3 bg-amber-50 border border-amber-200 rounded-xl flex items-center justify-between gap-2 flex-wrap">
                        <div className="min-w-0">
                          <p className="text-[11px] font-extrabold text-amber-800">
                            {item.orcamento.status === 'aprovado' ? '✅ Orçamento aprovado' : '⏳ Orçamento aguardando resposta'} — R$ {(item.orcamento.finalValue || 0).toFixed(2)}
                          </p>
                          <p className="text-[10px] text-slate-500 truncate">
                            {item.orcamento.numero ? '#' + item.orcamento.numero + ' • ' : ''}
                            {item.orcamento.items?.[0]?.description || item.orcamento.applianceDesc || ''}
                            {item.orcamento.validUntil ? ` • válido até ${safeFormat(item.orcamento.validUntil, 'dd/MM/yyyy')}` : ''}
                          </p>
                        </div>
                        <div className="flex items-center gap-1.5 shrink-0" onClick={(e) => e.stopPropagation()}>
                          {onAbrirOrcamento && (
                            <button
                              onClick={() => onAbrirOrcamento(item.orcamento!)}
                              className="px-2 py-1.5 bg-white hover:bg-slate-100 border border-slate-300 text-slate-700 rounded-lg text-[10px] font-bold transition-colors"
                            >
                              Ver proposta
                            </button>
                          )}
                          {item.orcamento.status === 'aprovado' && onAgendarOrcamento && (
                            <button
                              onClick={() => onAgendarOrcamento(item.orcamento!)}
                              title="Agendar o serviço deste orçamento aprovado"
                              className="px-2 py-1.5 bg-sky-600 hover:bg-sky-500 text-white rounded-lg text-[10px] font-extrabold transition-colors"
                            >
                              📅 Agendar
                            </button>
                          )}
                        </div>
                      </div>
                    )}

                    {/* Chamados pendentes de agendamento */}
                    {(item.chamados?.length || 0) > 0 && (
                      <div className="bg-white rounded-xl border border-amber-200 divide-y divide-amber-100 mb-3 overflow-hidden">
                        <div className="px-3 py-2 bg-amber-50 flex items-center justify-between">
                          <span className="text-[10px] font-extrabold text-amber-700 uppercase tracking-wide">🔔 Chamado(s) aguardando agendamento</span>
                          <span className="text-[10px] font-bold text-amber-700">{item.chamados!.length}</span>
                        </div>
                        {item.chamados!.map((s: any) => (
                          <div key={s.id} className="px-3 py-2 flex items-center justify-between gap-2">
                            <span className="text-[11px] text-slate-600 truncate">
                              {(s.descricao || s.tipo || '').replace(/_/g, ' ')}
                              {s.data_solicitacao ? ` • pedido em ${safeFormat(String(s.data_solicitacao).slice(0, 10), 'dd/MM')}` : ''}
                            </span>
                            <div className="flex items-center gap-1 shrink-0" onClick={(e) => e.stopPropagation()}>
                              {onAtenderChamado && (
                                <button
                                  onClick={() => onAtenderChamado(s.id)}
                                  title="Agendar data e hora deste chamado"
                                  className="px-2 py-1 bg-amber-500 hover:bg-amber-400 text-white rounded-lg text-[10px] font-bold transition-colors"
                                >
                                  📅 Agendar
                                </button>
                              )}
                              {onUpdateStatus && (
                                <button
                                  onClick={() => onUpdateStatus(s.id, 'EM_ANDAMENTO')}
                                  title="Iniciar execução agora"
                                  className="px-2 py-1 bg-purple-50 hover:bg-purple-100 text-purple-700 rounded-lg text-[10px] font-bold border border-purple-100 transition-colors"
                                >
                                  ▶
                                </button>
                              )}
                              {onUpdateStatus && (
                                <button
                                  onClick={() => { if (confirm('Cancelar este chamado?')) onUpdateStatus(s.id, 'CANCELADO'); }}
                                  title="Cancelar chamado"
                                  className="px-2 py-1 bg-red-50 hover:bg-red-100 text-red-600 rounded-lg text-[10px] font-bold border border-red-100 transition-colors"
                                >
                                  ✕
                                </button>
                              )}
                            </div>
                          </div>
                        ))}
                      </div>
                    )}

                    {/* Ficha do Aparelho (contato e ações já ficam nos botões da fila) */}
                    <button
                      onClick={() => onOpenFicha(item.client, item.appliance)}
                      className="w-full mb-3 p-3 bg-white hover:bg-slate-50 border border-slate-300 rounded-xl flex items-center justify-between gap-2 transition-colors"
                    >
                      <span className="flex items-center gap-2">
                        <Wrench className="w-4 h-4 text-blue-600" />
                        <span className="font-bold text-slate-800 text-xs">Ficha do Aparelho</span>
                      </span>
                      <span className="text-[10px] text-slate-400">especificações, observações e edições</span>
                    </button>

                    {/* ATENDIMENTO EM TEMPO REAL */}
                    <div className="bg-white rounded-xl border border-slate-200 divide-y divide-slate-100">
                      <div className="px-3 py-2 flex items-center justify-between">
                        <span className="text-[10px] font-extrabold text-slate-500 uppercase tracking-wide">Atendimento em tempo real</span>
                        {ags.length > 0 && (
                          <span className="text-[10px] font-bold text-blue-600 bg-blue-50 px-2 py-0.5 rounded-full border border-blue-100">
                            AGENDADO: {ags[0].data_agendamento ? safeFormat(String(ags[0].data_agendamento).slice(0, 10), 'dd/MM') : '—'}
                            {ags[0].hora_agendamento ? ' ' + ags[0].hora_agendamento : ''}
                          </span>
                        )}
                      </div>

                      {/* Botões de status dos serviços abertos deste cliente+aparelho */}
                      {(emExecucao.length > 0 || ags.length > 0) && (onUpdateStatus || onFinalizarServico) && (
                        <div className="px-3 py-2 space-y-1.5">
                          {emExecucao.map((s: any) => (
                            <div key={s.id} className="flex items-center justify-between gap-2">
                              <span className="text-[10px] font-bold text-violet-600">🔧 Em andamento agora</span>
                              <div className="flex items-center gap-1" onClick={(e) => e.stopPropagation()}>
                                {onFinalizarServico && (
                                  <button
                                    onClick={() => onFinalizarServico(s.id)}
                                    title="Finalizar: checklist, valor e garantia nesta mesma OS"
                                    className="px-2 py-1 bg-emerald-50 hover:bg-emerald-100 text-emerald-700 rounded-lg text-[10px] font-bold border border-emerald-100 transition-colors"
                                  >
                                    ✓ Finalizar
                                  </button>
                                )}
                                {onUpdateStatus && (
                                  <button
                                    onClick={() => { if (confirm('Cancelar este serviço?')) onUpdateStatus(s.id, 'CANCELADO'); }}
                                    title="Cancelar serviço"
                                    className="px-2 py-1 bg-red-50 hover:bg-red-100 text-red-600 rounded-lg text-[10px] font-bold border border-red-100 transition-colors"
                                  >
                                    ✕
                                  </button>
                                )}
                              </div>
                            </div>
                          ))}
                          {ags.map((s: any) => (
                            <div key={s.id} className="flex items-center justify-between gap-2">
                              <span className="text-[10px] text-slate-500 truncate">
                                📅 {(s.descricao || s.tipo || '').replace(/_/g, ' ')}
                              </span>
                              <div className="flex items-center gap-1 shrink-0" onClick={(e) => e.stopPropagation()}>
                                {onUpdateStatus && (
                                  <button
                                    onClick={() => onUpdateStatus(s.id, 'EM_ANDAMENTO')}
                                    title="Iniciar execução agora"
                                    className="px-2 py-1 bg-purple-50 hover:bg-purple-100 text-purple-700 rounded-lg text-[10px] font-bold border border-purple-100 transition-colors"
                                  >
                                    ▶ Iniciar
                                  </button>
                                )}
                                {onFinalizarServico && (
                                  <button
                                    onClick={() => onFinalizarServico(s.id)}
                                    title="Finalizar: checklist, valor e garantia nesta mesma OS"
                                    className="px-2 py-1 bg-emerald-50 hover:bg-emerald-100 text-emerald-700 rounded-lg text-[10px] font-bold border border-emerald-100 transition-colors"
                                  >
                                    ✓ Finalizar
                                  </button>
                                )}
                                {onUpdateStatus && (
                                  <button
                                    onClick={() => { if (confirm('Cancelar este serviço?')) onUpdateStatus(s.id, 'CANCELADO'); }}
                                    title="Cancelar serviço"
                                    className="px-2 py-1 bg-red-50 hover:bg-red-100 text-red-600 rounded-lg text-[10px] font-bold border border-red-100 transition-colors"
                                  >
                                    ✕
                                  </button>
                                )}
                              </div>
                            </div>
                          ))}
                        </div>
                      )}
                      <div className="px-3 py-2 space-y-1.5 text-[11px]">
                        <div className="flex items-center justify-between">
                          <span className="text-slate-500">📄 Orçamentos: <b className="text-slate-700">{orcamentosCliente.length}</b></span>
                          {orcamentosCliente.length > 0 && onAbrirOrcamento && (
                            <button onClick={() => onAbrirOrcamento(orcamentosCliente[0])} className="font-bold text-sky-600 hover:text-sky-700">
                              Abrir último ↗
                            </button>
                          )}
                        </div>
                        <div className="flex items-center justify-between">
                          <span className="text-slate-500">🛠️ Ordens de Serviço no histórico: <b className="text-slate-700">{historicoSelecionado.length}</b></span>
                          {historicoSelecionado.length > 0 && onAbrirOS && (
                            <button onClick={() => onAbrirOS(item.client, item.appliance, historicoSelecionado[0])} className="font-bold text-emerald-600 hover:text-emerald-700">
                              Abrir última ↗
                            </button>
                          )}
                        </div>
                      </div>
                    </div>

                    {/* 4. HISTÓRICO DO APARELHO */}
                    <div className="bg-white rounded-xl border border-slate-200 divide-y divide-slate-100">
                      <div className="px-3 py-2 flex items-center justify-between">
                        <span className="text-[10px] font-extrabold text-slate-500 uppercase tracking-wide flex items-center gap-1">
                          <History className="w-3.5 h-3.5 text-slate-400" /> Histórico do aparelho
                        </span>
                        <span className="text-[10px] font-bold text-slate-600 bg-slate-100 px-2 py-0.5 rounded-full border border-slate-200">
                          {historicoSelecionado.length} {historicoSelecionado.length === 1 ? 'Ordem de Serviço' : 'Ordens de Serviço'}
                        </span>
                      </div>
                      {historicoSelecionado.length === 0 ? (
                        <div className="px-3 py-3">
                          <p className="text-[11px] text-slate-400 mb-2">
                            Nenhuma Ordem de Serviço ainda — este aparelho nunca foi atendido pelo aplicativo.
                          </p>
                          {onRegistrarHistorico && (
                            <button
                              onClick={() => onRegistrarHistorico(item.client, item.appliance)}
                              className="w-full py-2 bg-violet-600 hover:bg-violet-500 text-white rounded-lg text-[11px] font-extrabold flex items-center justify-center gap-1.5 transition-colors active:scale-95"
                            >
                              <History className="w-3.5 h-3.5" />
                              Registrar histórico anterior (serviço já feito)
                            </button>
                          )}
                        </div>
                      ) : (
                        <div className="divide-y divide-slate-50">
                          {historicoSelecionado.map((rec) => (
                            <button
                              key={rec.id}
                              onClick={() => onAbrirOS && onAbrirOS(item.client, item.appliance, rec)}
                              className="w-full px-3 py-2 flex items-center justify-between gap-2 text-[11px] hover:bg-slate-50 transition-colors text-left"
                            >
                              <span className="min-w-0">
                                <span className="font-bold text-slate-700 block truncate">{rec.serviceType}</span>
                                <span className="text-slate-400">
                                  {safeFormat(rec.date, 'dd/MM/yyyy')} • Garantia {rec.warrantyDays}d
                                  {rec.contactedAt ? ' • contato feito' : ''}
                                </span>
                              </span>
                              <span className="flex items-center gap-1.5 shrink-0">
                                <span className="font-bold text-slate-600">R$ {(rec.price || 0).toFixed(2)}</span>
                                <ChevronRight className="w-3.5 h-3.5 text-slate-400" />
                              </span>
                            </button>
                          ))}
                        </div>
                      )}
                    </div>
                  </div>
                )}
              </div>
              </React.Fragment>
            );
          })
        )}
      </div>}
    </div>
  );
};
