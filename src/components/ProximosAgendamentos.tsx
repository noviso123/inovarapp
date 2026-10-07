import React from 'react';
import { Calendar, ChevronRight, Clock, Download, ExternalLink, XCircle } from 'lucide-react';
import { Client, Appliance, MaintenanceRecord } from '../types';
import { format, parseISO } from 'date-fns';
import { baixarConviteICS, linkGoogleAgenda, linkOutlookAgenda } from '../services/calendario';

interface AppointmentEvent {
  client: Client;
  appliance: Appliance;
  maintenance: MaintenanceRecord;
  scheduledDate: string;
}

interface Props {
  events: AppointmentEvent[];
  onSelectEvent: (client: Client, appliance: Appliance, maintenance: MaintenanceRecord) => void;
  onIniciar?: (serviceId: string) => void;
  onFinalizar?: (serviceId: string) => void;
  onCancelar?: (serviceId: string) => void;
}

export const ProximosAgendamentos: React.FC<Props> = ({ events, onSelectEvent, onIniciar, onFinalizar, onCancelar }) => {
  const sorted = [...events]
    .filter((event) => (event.maintenance.status === 'agendado' || event.maintenance.status === 'em_andamento') && !!event.scheduledDate)
    .sort((a, b) => a.scheduledDate.localeCompare(b.scheduledDate));

  return (
    <div className="bg-white text-slate-900 rounded-2xl shadow-xl border border-slate-200 overflow-hidden">
      <div className="bg-blue-50 px-4 py-3.5 border-b border-blue-100 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <div className="p-1.5 bg-blue-100 text-blue-700 rounded-lg"><Calendar className="w-4 h-4" /></div>
          <div>
            <h3 className="font-bold text-slate-800 text-sm">Próximos agendamentos</h3>
            <p className="text-[10px] text-blue-700">Serviços já marcados para atendimento</p>
          </div>
        </div>
        <span className="text-xs font-semibold text-blue-700 bg-blue-100 px-2 py-0.5 rounded-full">{sorted.length}</span>
      </div>
      <div className="divide-y divide-slate-100">
        {sorted.length === 0 ? (
          <div className="p-6 text-center text-slate-400 text-xs">Nenhum agendamento futuro cadastrado.</div>
        ) : sorted.map((item) => {
          let dateLabel = item.scheduledDate;
          try { dateLabel = format(parseISO(item.scheduledDate), 'dd/MM/yyyy'); } catch { /* mantém original */ }
          const evento = {
            id: item.maintenance.id,
            titulo: `${String(item.maintenance.serviceType || 'Atendimento')} — ${item.client.name}`,
            descricao: `Agendamento Inovar Refrigeração. Telefone: ${item.client.phone || ''}`,
            local: item.client.address || item.client.city || '',
            data: item.scheduledDate.slice(0, 10),
            hora: item.maintenance.scheduledTime || '09:00',
            duracaoMin: 120
          };
          return (
            <div key={item.maintenance.id} className="p-3.5 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 hover:bg-blue-50/50 transition-colors group">
              <button onClick={() => onSelectEvent(item.client, item.appliance, item.maintenance)} className="min-w-0 flex-1 flex items-center gap-3 text-left">
              <div className="flex items-center gap-3">
                <div className="w-[4.75rem] min-h-14 px-2 rounded-xl bg-blue-50 border border-blue-100 flex flex-col items-center justify-center shrink-0">
                  <Calendar className="w-3.5 h-3.5 text-blue-600 mb-0.5" />
                  <span className="text-[10px] font-extrabold text-blue-800">{dateLabel.slice(0, 5)}</span>
                </div>
                <div>
                  <h4 className="font-bold text-slate-900 text-sm group-hover:text-blue-700">{item.client.name}</h4>
                  <p className="text-xs text-slate-500 mt-0.5">{item.appliance.type} {item.appliance.capacityBtu} BTUs • {item.appliance.room}</p>
                  <p className="text-[10px] text-blue-600 font-semibold mt-0.5 flex items-center gap-1"><Clock className="w-3 h-3" /> {dateLabel} {item.maintenance.scheduledTime || ''}</p>
                </div>
              </div>
                <ChevronRight className="w-4 h-4 text-slate-400 group-hover:text-blue-600" />
              </button>
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
                </div>
                <div className="flex items-center gap-1">
                  <button title="Baixar convite para celular/computador (.ics)" onClick={() => baixarConviteICS(evento)} className="p-1.5 rounded-lg bg-slate-100 hover:bg-slate-200 text-slate-600" aria-label="Baixar convite ICS"><Download className="w-3.5 h-3.5" /></button>
                  <a title="Adicionar ao Google Agenda" href={linkGoogleAgenda(evento)} target="_blank" rel="noopener noreferrer" className="p-1.5 rounded-lg bg-blue-100 hover:bg-blue-200 text-blue-700" aria-label="Google Agenda"><Calendar className="w-3.5 h-3.5" /></a>
                  <a title="Adicionar ao Outlook" href={linkOutlookAgenda(evento)} target="_blank" rel="noopener noreferrer" className="p-1.5 rounded-lg bg-violet-100 hover:bg-violet-200 text-violet-700" aria-label="Outlook"><ExternalLink className="w-3.5 h-3.5" /></a>
                  {onCancelar && (
                    <button
                      title="Cancelar este atendimento"
                      onClick={() => { if (confirm('Cancelar este atendimento? O cliente será avisado no WhatsApp.')) onCancelar(item.maintenance.id); }}
                      className="p-1.5 rounded-lg bg-red-50 hover:bg-red-100 text-red-500 border border-red-100 transition-colors"
                      aria-label="Cancelar atendimento"
                    >
                      <XCircle className="w-3.5 h-3.5" />
                    </button>
                  )}
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
};
