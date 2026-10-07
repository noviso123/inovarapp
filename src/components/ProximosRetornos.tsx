import React from 'react';
import { Client, Appliance, MaintenanceRecord } from '../types';
import { Calendar, ChevronRight, MessageSquare, Wrench, XCircle } from 'lucide-react';
import { format, parseISO } from 'date-fns';
import { ptBR } from 'date-fns/locale';

interface ReturnEvent {
  client: Client;
  appliance: Appliance;
  maintenance: MaintenanceRecord;
  returnDate: string;
}

interface ProximosRetornosProps {
  events: ReturnEvent[];
  onSelectEvent: (client: Client, appliance: Appliance, maintenance: MaintenanceRecord) => void;
  onOpenChecklist: (client: Client, appliance: Appliance) => void;
  onIniciar?: (serviceId: string) => void;
  onFinalizar?: (serviceId: string) => void;
  onCancelar?: (serviceId: string) => void;
}

export const ProximosRetornos: React.FC<ProximosRetornosProps> = ({
  events,
  onSelectEvent,
  onOpenChecklist,
  onIniciar,
  onFinalizar,
  onCancelar
}) => {
  // Mostra retornos calculados de serviços concluídos e próximos agendamentos.
  // Data de referência: retorno calculado, senão a data do agendamento/serviço
  // (sem isso o sort recebia NaN para itens em andamento, cujo retorno é vazio).
  const dataRef = (e: ReturnEvent) => e.returnDate || e.maintenance.scheduledDate || e.maintenance.date || '9999-12-31';
  const sortedEvents = [...events].filter((e) => {
    const isReturn = e.maintenance.status === 'concluido' && !!e.returnDate;
    const isAppointment = e.maintenance.status === 'agendado' && !!(e.maintenance.scheduledDate || e.returnDate);
    const isRunning = e.maintenance.status === 'em_andamento';
    return isReturn || isAppointment || isRunning;
  }).sort((a, b) => dataRef(a).localeCompare(dataRef(b)));

  return (
    <div className="bg-white text-slate-900 rounded-2xl shadow-xl border border-slate-200 overflow-hidden">
      {/* Header matching phone mockup */}
      <div className="bg-slate-50 px-4 py-3.5 border-b border-slate-200 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <div className="p-1.5 bg-sky-100 text-sky-700 rounded-lg">
            <Calendar className="w-4 h-4" />
          </div>
          <h3 className="font-bold text-slate-800 text-sm">
            Próximos retornos
          </h3>
        </div>
        <span className="text-xs font-semibold text-slate-500 bg-slate-200/70 px-2 py-0.5 rounded-full">
          {sortedEvents.length} retornos
        </span>
      </div>

      {/* List matching screenshot format */}
      <div className="divide-y divide-slate-100">
        {sortedEvents.length === 0 ? (
          <div className="p-6 text-center text-slate-400 text-xs">
            Nenhum retorno preventivo pendente.
          </div>
        ) : (
          sortedEvents.map(item => {
            const ref = item.returnDate || item.maintenance.scheduledDate || item.maintenance.date || '';
            const dayMonth = ref && !isNaN(parseISO(ref).getTime()) ? format(parseISO(ref), 'dd/MM/yyyy') : '—';

            const baseConclusao = item.maintenance.completionDate || item.maintenance.date || '';
            const dtConclusaoFmt = baseConclusao && !isNaN(parseISO(baseConclusao).getTime()) ? format(parseISO(baseConclusao), 'dd/MM/yyyy') : '';

            return (
              <div
                key={item.maintenance.id}
                onClick={() => onSelectEvent(item.client, item.appliance, item.maintenance)}
                className="p-3.5 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 hover:bg-slate-50 transition-colors cursor-pointer group"
              >
                <div className="flex items-center gap-3">
                  {/* Calendar Badge exactly as in the ad */}
                  <div className="w-[4.75rem] min-h-14 px-2 rounded-xl bg-slate-100 border border-slate-200 flex flex-col items-center justify-center shrink-0 group-hover:border-sky-300 group-hover:bg-sky-50 transition-colors">
                    <Calendar className="w-3.5 h-3.5 text-sky-600 mb-0.5" />
                    <span className="text-[10px] font-extrabold text-slate-800 leading-none">
                      {dayMonth}
                    </span>
                  </div>

                  {/* Customer info & Appliance specs */}
                  <div>
                    <h4 className="font-bold text-slate-900 text-sm group-hover:text-sky-700 transition-colors">
                      {item.client.name}
                    </h4>
                    <p className="text-xs text-slate-500 mt-0.5">
                      {item.appliance.type} {item.appliance.capacityBtu} BTUs • {item.appliance.room}
                    </p>
                    <p className="text-[10px] text-sky-600 font-semibold mt-0.5">
                      {item.maintenance.status === 'agendado' ? 'Próximo agendamento' : item.maintenance.status === 'em_andamento' ? 'Em andamento agora' : (dtConclusaoFmt ? `Retorno após ${dtConclusaoFmt}` : 'Retorno preventivo')}
                    </p>
                  </div>
                </div>

                {/* Ações rápidas + seta */}
                <div className="flex items-center justify-between sm:flex-col sm:items-end gap-1.5 shrink-0" onClick={(e) => e.stopPropagation()}>
                  <div className="flex items-center gap-1.5">
                    {item.maintenance.status === 'agendado' && onIniciar && (
                      <button
                        onClick={() => onIniciar(item.maintenance.id)}
                        title="Iniciar o atendimento agora (marca Em Andamento)"
                        className="px-2.5 py-1.5 bg-purple-50 hover:bg-purple-100 text-purple-700 border border-purple-100 rounded-lg text-[10px] font-extrabold transition-colors"
                      >
                        ▶ Iniciar
                      </button>
                    )}
                    {item.maintenance.status === 'em_andamento' && onFinalizar && (
                      <button
                        onClick={() => onFinalizar(item.maintenance.id)}
                        title="Finalizar o atendimento (checklist, valor e garantia nesta mesma OS)"
                        className="px-2.5 py-1.5 bg-emerald-50 hover:bg-emerald-100 text-emerald-700 border border-emerald-100 rounded-lg text-[10px] font-extrabold transition-colors"
                      >
                        ✓ Finalizar
                      </button>
                    )}
                    {(item.maintenance.status === 'agendado' || item.maintenance.status === 'em_andamento') && onOpenChecklist && (
                      <button
                        onClick={() => onOpenChecklist(item.client, item.appliance)}
                        title="Abrir checklist de campo deste aparelho"
                        className="p-1.5 rounded-lg bg-slate-100 hover:bg-slate-200 text-slate-600 transition-colors"
                        aria-label="Checklist"
                      >
                        <Wrench className="w-3.5 h-3.5" />
                      </button>
                    )}
                    {(item.maintenance.status === 'agendado' || item.maintenance.status === 'em_andamento') && onCancelar && (
                      <button
                        onClick={() => { if (confirm('Cancelar este atendimento? O cliente será avisado no WhatsApp.')) onCancelar(item.maintenance.id); }}
                        title="Cancelar este atendimento"
                        className="p-1.5 rounded-lg bg-red-50 hover:bg-red-100 text-red-500 border border-red-100 transition-colors"
                        aria-label="Cancelar atendimento"
                      >
                        <XCircle className="w-3.5 h-3.5" />
                      </button>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-slate-400 hidden sm:inline">
                      R$ {item.maintenance.price?.toFixed(0) || '0'}
                    </span>
                    <ChevronRight className="w-4 h-4 text-slate-400 group-hover:text-sky-600 group-hover:translate-x-0.5 transition-all" />
                  </div>
                </div>
              </div>
            );
          })
        )}
      </div>
    </div>
  );
};
