import React, { useState } from 'react';
import { Calendar, Phone, CheckCircle2, Clock, Wrench, User, Trash2, AlertTriangle, Calculator, Play, XCircle } from 'lucide-react';
import { format, parseISO } from 'date-fns';
import { normalizarNomeServico } from '../services/catalogo';

interface SolicitacoesTabProps {
  onAgendar?: (chamado: { id: string; cliente: string; servico: string; aparelho: string }) => void;
  onExcluirServico?: (serviceId: string) => void;
  onGerarOrcamento?: (client: any) => void;
  services: any[];
  onUpdateStatus: (serviceId: string, status: string) => void;
  onOpenChecklist: (customer: any, appliance: any) => void;
}

// "Hoje" por render/componente — se ficasse congelado no módulo, a classificação
// de atrasados errava quando o app ficasse aberto passando da meia-noite.
const hojeISO = () => format(new Date(), 'yyyy-MM-dd');

// Prioridade de atendimento: atrasado > caiu hoje > agendados futuros > pendentes sem data > concluídos
function chavePrioridade(s: any): number {
  const data = (s.data_agendamento || '').slice(0, 10);
  if (s.status === 'CONCLUIDO') return 9;
  if (s.status === 'CANCELADO') return 9.5;
  if (data && data < hojeISO()) return 0; // atrasado
  if (data === hojeISO()) return 1; // caiu hoje e ainda não foi atendido
  if (s.status === 'AGENDADO' && data) return 2; // agendados futuros, por data
  if (s.status === 'AGENDADO') return 3;
  return 4; // pendentes sem data
}

export const SolicitacoesTab: React.FC<SolicitacoesTabProps> = ({
  services,
  onUpdateStatus,
  onAgendar,
  onGerarOrcamento,
  onExcluirServico,
  onOpenChecklist
}) => {
  const [filter, setFilter] = useState<'TODOS' | 'PENDENTE' | 'AGENDADO' | 'CONCLUIDO'>('TODOS');

  const filtered = services
    .filter((s) => {
      if (s.status === 'CANCELADO' && filter !== 'TODOS') return false;
      if (filter === 'TODOS') return s.status !== 'CANCELADO';
      return s.status === filter;
    })
    .slice()
    .sort((a, b) => {
      const ca = chavePrioridade(a);
      const cb = chavePrioridade(b);
      if (ca !== cb) return ca - cb;
      // dentro do mesmo nível, mais antigo/mais próximo primeiro
      const da = (a.data_agendamento || '9999-12-31').slice(0, 10);
      const db = (b.data_agendamento || '9999-12-31').slice(0, 10);
      return da.localeCompare(db);
    });

  const atrasadosCount = services.filter(
    (s) => (s.data_agendamento || '').slice(0, 10) < hojeISO() && (s.status === 'PENDENTE' || s.status === 'AGENDADO')
  ).length;

  const getWhatsAppLink = (cust: any, serv: any) => {
    if (!cust?.whatsapp) return '#';
    const cleanPhone = cust.whatsapp.replace(/\D/g, '');
    const phoneWithDDI = cleanPhone.startsWith('55') ? cleanPhone : `55${cleanPhone}`;
    const text = encodeURIComponent(
      `Olá ${cust.nome}, aqui é da Inovar Refrigeração! Vi sua solicitação no app para o serviço de ${normalizarNomeServico(serv.descricao || serv.tipo).replace('_', ' ')}. Vamos confirmar o horário de atendimento?`
    );
    return `https://wa.me/${phoneWithDDI}?text=${text}`;
  };

  return (
    <div className="space-y-4">
      {/* Filter Tabs */}
      <div className="flex items-center justify-between gap-2 overflow-x-auto pb-1">
        <div className="flex items-center gap-1.5 bg-white p-1 rounded-xl border border-slate-200 shadow-sm text-xs">
          {(['TODOS', 'PENDENTE', 'AGENDADO', 'CONCLUIDO'] as const).map((f) => (
            <button
              key={f}
              onClick={() => setFilter(f)}
              className={`px-3 py-1.5 rounded-lg font-bold transition-all ${
                filter === f
                  ? 'bg-inovar-yellow text-inovar-navy shadow'
                  : 'text-slate-600 hover:text-slate-900 hover:bg-slate-100'
              }`}
            >
              {f === 'TODOS' ? 'Todas' : f === 'PENDENTE' ? 'Pendentes' : f === 'AGENDADO' ? 'Agendadas' : 'Concluídas'}
            </button>
          ))}
        </div>
        {atrasadosCount > 0 && (
          <span className="px-2.5 py-1 rounded-lg bg-red-500/15 border border-red-500/40 text-red-300 text-[11px] font-extrabold flex items-center gap-1.5 shrink-0">
            <AlertTriangle className="w-3.5 h-3.5" />
            {atrasadosCount} atrasado(s)
          </span>
        )}
      </div>

      {filtered.length === 0 ? (
        <div className="p-10 text-center bg-white border border-slate-200 rounded-2xl text-slate-500 shadow-sm">
          <Clock className="w-8 h-8 text-slate-400 mx-auto mb-2" />
          <h4 className="text-sm font-bold text-slate-700">Nenhuma solicitação encontrada</h4>
          <p className="text-xs text-slate-500 mt-1">
            Quando os clientes solicitarem atendimentos pelo app ou pelo site da Inovar, as ordens aparecerão aqui para você confirmar e atender.
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-3 items-start">
          {filtered.map((serv) => {
            const cust = serv.customers || {};
            const app = serv.air_conditioners || {};
            const dataServ = (serv.data_agendamento || '').slice(0, 10);
            const atrasado = dataServ && dataServ < hojeISO() && (serv.status === 'PENDENTE' || serv.status === 'AGENDADO');

            return (
              <div
                key={serv.id}
                className={`bg-white rounded-2xl p-4 shadow-sm flex flex-col gap-3 transition-all border ${
                  atrasado ? 'border-red-400' : 'border-slate-200 hover:border-sky-300'
                }`}
              >
                <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
                  <div>
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className="text-sm font-extrabold text-slate-800">
                        {cust.nome || 'Cliente não identificado'}
                      </span>
                      {atrasado && (
                        <span className="px-2 py-0.5 rounded-full text-[10px] font-bold border bg-red-500/20 border-red-500/50 text-red-300 flex items-center gap-1">
                          <AlertTriangle className="w-3 h-3" /> ATRASADO
                        </span>
                      )}
                      <span
                        className={`px-2 py-0.5 rounded-full text-[10px] font-bold border ${
                          serv.status === 'PENDENTE'
                            ? 'bg-amber-50 border-amber-200 text-amber-800'
                            : serv.status === 'AGENDADO'
                            ? 'bg-blue-50 border-blue-200 text-blue-800'
                            : serv.status === 'CONCLUIDO'
                            ? 'bg-emerald-50 border-emerald-200 text-emerald-800'
                            : 'bg-slate-100 border-slate-200 text-slate-700'
                        }`}
                      >
                        {serv.status}
                      </span>
                    </div>

                    <p className="text-xs text-slate-500 mt-0.5 flex items-center gap-2">
                      {cust.whatsapp && <span>Tel: {cust.whatsapp}</span>}
                      {cust.bairro && <span>• {cust.bairro}, {cust.cidade || 'ES'}</span>}
                    </p>
                  </div>

                  {/* WhatsApp button */}
                  {cust.whatsapp && (
                    <div className="flex items-center gap-2 w-full sm:w-auto sm:justify-end">
                      {onExcluirServico && (
                        <button
                          onClick={() => { if (confirm('Excluir este serviço permanentemente do banco?')) onExcluirServico(serv.id); }}
                          title="Excluir serviço"
                          className="p-2 text-slate-400 hover:text-red-600 rounded-xl hover:bg-red-50 transition-colors"
                        >
                          <Trash2 className="w-4 h-4" />
                        </button>
                      )}
                      <a
                        href={getWhatsAppLink(cust, serv)}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="inline-flex flex-1 sm:flex-none justify-center items-center gap-1.5 px-3 py-2 bg-emerald-600 hover:bg-emerald-500 text-white rounded-xl text-xs font-bold shadow transition-transform active:scale-95"
                      >
                        <Phone className="w-3.5 h-3.5" />
                        <span>WhatsApp</span>
                      </a>
                    </div>
                  )}
                </div>

                {/* Details box */}
                <div className="bg-slate-50 p-3 rounded-xl border border-slate-200 text-xs space-y-1.5">
                  <div className="flex items-start justify-between gap-3">
                    <span className="text-slate-500">Serviço:</span>
                    <span className="font-bold text-sky-700">{normalizarNomeServico(serv.descricao || serv.tipo).replace('_', ' ')}</span>
                  </div>

                  {app.marca && (
                    <div className="flex items-start justify-between gap-3">
                      <span className="text-slate-500">Aparelho:</span>
                      <span className="font-medium text-slate-700 text-right break-words">
                        {app.marca} {app.modelo || ''} ({app.btus ? `${app.btus} BTUs` : 'Split'}) - {app.ambiente || 'Ambiente'}
                      </span>
                    </div>
                  )}

                  {serv.problema && (
                    <div className="pt-1 border-t border-slate-200">
                      <span className="text-slate-500 block text-[11px]">Observações / Sintomas:</span>
                      <p className="text-slate-700 italic mt-0.5">"{serv.problema}"</p>
                    </div>
                  )}

                  {serv.data_agendamento && (
                    <div className="flex items-start justify-between gap-3 pt-1 border-t border-slate-200">
                      <span className="text-slate-500">Data Preferencial:</span>
                      <span className="font-bold text-slate-800">
                        {format(parseISO(serv.data_agendamento), 'dd/MM/yyyy')}
                      </span>
                    </div>
                  )}
                  {serv.data_inicio && <div className="flex items-center justify-between"><span className="text-slate-500">Iniciado em:</span><span className="font-medium text-purple-700">{format(parseISO(serv.data_inicio), 'dd/MM/yyyy')}</span></div>}
                  {serv.data_conclusao && <div className="flex items-center justify-between"><span className="text-slate-500">Concluído em:</span><span className="font-medium text-emerald-700">{format(parseISO(serv.data_conclusao), 'dd/MM/yyyy')}</span></div>}
                  {serv.data_cancelamento && <div className="flex items-center justify-between"><span className="text-slate-500">Cancelado em:</span><span className="font-medium text-red-700">{format(parseISO(serv.data_cancelamento), 'dd/MM/yyyy')}</span></div>}
                </div>

                {/* Actions */}
                <div className="grid grid-cols-1 min-[420px]:grid-cols-2 sm:flex sm:justify-end gap-2 pt-1">
                  {(serv.status === 'PENDENTE' || serv.status === 'AGENDADO') && onOpenChecklist && app.id && (
                    <button
                      onClick={() => { onUpdateStatus(serv.id, 'EM_ANDAMENTO'); onOpenChecklist(cust, app); }}
                      title="Iniciar execução de campo com checklist e OS"
                      className="px-3 py-1.5 bg-sky-600 hover:bg-sky-500 text-white rounded-lg text-xs font-bold shadow flex items-center gap-1.5 transition-transform active:scale-95"
                    >
                      <Play className="w-3.5 h-3.5" />
                      Iniciar Serviço
                    </button>
                  )}
                  {(serv.status === 'PENDENTE' || serv.status === 'AGENDADO') && onGerarOrcamento && (
                    <button
                      onClick={() => onGerarOrcamento({ ...cust, id: serv.cliente_id })}
                      title="Criar proposta comercial para este cliente"
                      className="px-3 py-1.5 bg-inovar-yellow hover:brightness-105 text-inovar-navy rounded-lg text-xs font-extrabold shadow flex items-center gap-1.5 transition-transform active:scale-95"
                    >
                      <Calculator className="w-3.5 h-3.5" />
                      Gerar Orçamento
                    </button>
                  )}
                  {serv.status === 'PENDENTE' && (
                    <button
                      onClick={() => onAgendar?.({ id: serv.id, cliente: cust.nome || 'Cliente', servico: serv.tipo, aparelho: (app.marca || '') + ' ' + (app.modelo || '') + ' (' + (app.ambiente || '') + ')' })}
                      className="px-3 py-1.5 bg-blue-600 hover:bg-blue-500 text-white rounded-lg text-xs font-bold shadow"
                    >
                      Agendar Serviço
                    </button>
                  )}

                  {(serv.status === 'AGENDADO' || serv.status === 'EM_ANDAMENTO') && (
                    <button
                      onClick={() => onUpdateStatus(serv.id, 'CONCLUIDO')}
                      className="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 text-white rounded-lg text-xs font-bold shadow"
                    >
                      Concluir Serviço
                    </button>
                  )}
                  {(serv.status === 'PENDENTE' || serv.status === 'AGENDADO' || serv.status === 'EM_ANDAMENTO') && (
                    <button
                      onClick={() => { if (confirm('Cancelar este serviço?')) onUpdateStatus(serv.id, 'CANCELADO'); }}
                      className="px-3 py-1.5 bg-red-600 hover:bg-red-500 text-white rounded-lg text-xs font-bold shadow flex items-center gap-1.5"
                    >
                      <XCircle className="w-3.5 h-3.5" />
                      Cancelar Serviço
                    </button>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};
