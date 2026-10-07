import React, { useMemo } from 'react';
import { Client, Appliance, MaintenanceRecord } from '../types';
import { Calendar, Phone, XCircle, CalendarClock, MapPin, CheckCircle2, RefreshCw } from 'lucide-react';
import { format, parseISO } from 'date-fns';

interface AgendaTabProps {
  maintenances: MaintenanceRecord[];
  clients: Client[];
  appointments: any[];
  onUpdateStatus: (serviceId: string, status: string) => void;
  onReschedule: (serviceId: string, novaData: string) => void;
  onFinalizar?: (serviceId: string) => void;
  calendarConnected?: boolean;
  calendarSyncing?: boolean;
  calendarLastSync?: string | null;
  onGoogleCalendarSync?: () => void;
  onEditServico?: (item: { id: string; tipo: string; data: string; valor: number; cliente: string; aparelho: string; observacoes?: string }) => void;
  onReabrirOS?: (serviceId: string, novaData: string) => void;
  onExcluirServico?: (serviceId: string) => void;
}

// Agenda consolidada: serviços AGENDADOS + agendamentos da tabela appointments.
// Tudo lido do banco — pode visualizar e alterar de qualquer plataforma.
export const AgendaTab: React.FC<AgendaTabProps> = ({
  maintenances,
  clients,
  appointments,
  onUpdateStatus,
  onReschedule,
  onFinalizar,
  calendarConnected = false,
  calendarSyncing = false,
  calendarLastSync,
  onGoogleCalendarSync,
  onEditServico,
  onReabrirOS,
  onExcluirServico
}) => {
  const itens = useMemo(() => {
    const agendados = maintenances
      .filter((m) => m.status === 'agendado' || m.status === 'em_andamento' || m.status === 'concluido')
      .map((m) => {
        const client = clients.find((c) => c.id === m.clientId);
        const appliance = client?.appliances?.find((a) => a.id === m.applianceId);
        const appt = appointments.find((x: any) => x.service_id === m.id);
        return {
          id: m.id,
          cliente: client?.name || 'Cliente',
          telefone: client?.phone || '',
          cidade: client?.city || '',
          aparelho: appliance ? `${appliance.brand} ${appliance.capacityBtu} BTUs (${appliance.room})` : 'Aparelho',
          servico: m.serviceType,
          valor: m.price,
          data: m.scheduledDate || m.date,
          origem: appt ? 'agenda' : 'servico',
          status: m.status
          ,dataInicio: m.startedAt
          ,dataConclusao: m.completedAt || m.completionDate
          ,dataCancelamento: m.cancelledAt
        };
      })
      .sort((a, b) => a.data.localeCompare(b.data));
    return agendados;
  }, [maintenances, clients, appointments]);

  const safeDate = (value: string, pattern: string) => { const date = parseISO(value || ''); return Number.isNaN(date.getTime()) ? '—' : format(date, pattern); };
  const hoje = format(new Date(), 'yyyy-MM-dd');
  // Em andamento continua visível (senão o serviço some da agenda ao clicar "Iniciar")
  const futuros = itens.filter((i) => i.data >= hoje && i.status !== 'concluido');
  const passadosPendentes = itens.filter((i) => i.data < hoje && i.status !== 'concluido');
  const concluidosRecentes = itens.filter((i) => i.status === 'concluido').slice(-8).reverse();

  const Card = ({ i }: { i: ReturnType<() => any> }) => (
    <div className="bg-white border border-slate-200 rounded-2xl p-4 shadow-sm hover:shadow-md transition-all">
      <div className="flex flex-col sm:flex-row sm:items-start justify-between gap-3">
        <div className="flex items-start gap-3 min-w-0">
          <div className="w-[4.75rem] min-h-14 px-2 rounded-2xl bg-sky-50 border border-sky-100 flex flex-col items-center justify-center shrink-0">
            <span className="text-[9px] font-bold text-sky-600 uppercase leading-none">{safeDate(i.data, 'MMM')}</span>
            <span className="text-lg font-extrabold text-slate-800 leading-none">{safeDate(i.data, 'dd')}</span>
          </div>
          <div className="min-w-0">
            <div className="flex items-center gap-2 flex-wrap">
              <span className="font-bold text-sm text-slate-800">{i.cliente}</span>
              <span className={`px-2 py-0.5 rounded-full text-[10px] font-bold ${i.status === 'agendado' ? 'bg-blue-50 text-blue-700 border border-blue-100' : 'bg-emerald-50 text-emerald-700 border border-emerald-100'}`}>
                {i.status === 'agendado' ? 'Agendado' : i.status === 'em_andamento' ? 'Em andamento' : 'Concluído'}
              </span>
            </div>
            <p className="text-xs text-slate-500 mt-0.5 flex items-center gap-1">
              <CalendarClock className="w-3 h-3" /> {i.servico} • {i.aparelho}
            </p>
            <p className="text-[11px] text-slate-400 mt-0.5 flex items-center gap-1">
              {i.telefone && <><Phone className="w-3 h-3" /> {i.telefone}</>}
              {i.cidade && <><MapPin className="w-3 h-3 ml-1" /> {i.cidade}</>}
            </p>
            <p className="text-[10px] text-slate-400 mt-1">
              Agendamento: {safeDate(i.data, 'dd/MM/yyyy')}
              {i.dataInicio && ` • Início: ${safeDate(i.dataInicio, 'dd/MM/yyyy')}`}
              {i.dataConclusao && ` • Conclusão: ${safeDate(i.dataConclusao, 'dd/MM/yyyy')}`}
              {i.dataCancelamento && ` • Cancelamento: ${safeDate(i.dataCancelamento, 'dd/MM/yyyy')}`}
            </p>
          </div>
        </div>

        <div className="flex flex-col items-end gap-2 shrink-0">
          <span className="font-extrabold text-emerald-600">
            {i.valor > 0 ? `R$ ${i.valor.toFixed(2)}` : '—'}
          </span>
          {onExcluirServico && (
            <button
              onClick={() => { if (confirm('Excluir este serviço permanentemente do banco?')) onExcluirServico(i.id); }}
              title="Excluir serviço"
              className="p-1.5 bg-red-50 hover:bg-red-100 text-red-500 rounded-lg border border-red-100 transition-colors"
            >
              <XCircle className="w-4 h-4" />
            </button>
          )}
          {i.status === 'concluido' && onReabrirOS && (
            <div className="flex items-center gap-1.5">
              <input
                key={i.id + i.data}
                type="date"
                defaultValue={i.data}
                onChange={(e) => { if (e.target.value && e.target.value !== i.data) onReabrirOS(i.id, e.target.value); }}
                className="p-1.5 bg-slate-50 border border-slate-200 rounded-lg text-[10px] text-slate-600"
                title="Nova data"
              />
              <button
                onClick={() => onReabrirOS(i.id, i.data)}
                title="Reabrir como agendado"
                className="px-2 py-1.5 bg-blue-50 hover:bg-blue-100 text-blue-600 rounded-lg border border-blue-100 text-[10px] font-bold transition-colors"
              >
                Reagendar
              </button>
            </div>
          )}
          {(i.status === 'agendado' || i.status === 'em_andamento') && (
            <div className="flex items-center gap-1.5">
              <input
                key={i.id + i.data}
                type="date"
                defaultValue={i.data}
                onChange={(e) => { if (e.target.value && e.target.value !== i.data) onReschedule(i.id, e.target.value); }}
                className="p-1.5 bg-slate-50 border border-slate-200 rounded-lg text-[10px] text-slate-600"
                title="Alterar data"
              />
              {i.status === 'agendado' && <button
                onClick={() => onUpdateStatus(i.id, 'EM_ANDAMENTO')}
                title="Iniciar serviço"
                className="px-2 py-1.5 bg-purple-50 hover:bg-purple-100 text-purple-700 rounded-lg border border-purple-100 text-[10px] font-bold transition-colors"
              >
                Iniciar
              </button>}
              {i.status === 'em_andamento' && <button
                onClick={() => (onFinalizar ? onFinalizar(i.id) : onUpdateStatus(i.id, 'CONCLUIDO'))}
                title="Finalizar serviço (checklist, valor e garantia na mesma OS)"
                className="px-2 py-1.5 bg-emerald-50 hover:bg-emerald-100 text-emerald-600 rounded-lg border border-emerald-100 text-[10px] font-bold transition-colors"
              >
                Finalizar
              </button>}
              <button
                onClick={() => { if (confirm('Cancelar este agendamento?')) onUpdateStatus(i.id, 'CANCELADO'); }}
                title="Cancelar agendamento"
                className="p-1.5 bg-red-50 hover:bg-red-100 text-red-500 rounded-lg border border-red-100 transition-colors"
              >
                <XCircle className="w-4 h-4" />
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );

  return (
    <div className="operational-screen space-y-4">
      <div className="bg-white rounded-2xl border border-slate-200 shadow-sm p-4 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <div className="p-1.5 bg-blue-100 text-blue-700 rounded-lg">
            <Calendar className="w-4 h-4" />
          </div>
          <div>
            <h3 className="font-bold text-sm text-slate-800">Agenda de Atendimentos</h3>
            <p className="text-[11px] text-slate-500">Alterne datas, conclua ou cancele — de qualquer plataforma</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={onGoogleCalendarSync}
            disabled={calendarSyncing || !onGoogleCalendarSync}
            title={calendarConnected
              ? `Google Agenda conectado${calendarLastSync ? ` — última atualização: ${safeDate(calendarLastSync, 'dd/MM HH:mm')}` : ''}. Toque para atualizar agora.`
              : 'Conectar ao Google Agenda. Depois da primeira autorização, os agendamentos são atualizados automaticamente.'}
            className="min-h-9 px-3 py-1.5 bg-emerald-50 hover:bg-emerald-100 disabled:opacity-60 text-emerald-800 rounded-full text-[10px] font-extrabold border border-emerald-200 transition-colors inline-flex items-center gap-1.5"
          >
            {calendarSyncing ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : calendarConnected ? <CheckCircle2 className="w-3.5 h-3.5" /> : <Calendar className="w-3.5 h-3.5" />}
            {calendarSyncing ? 'Atualizando…' : calendarConnected ? 'Google Agenda sincronizado' : 'Conectar Google Agenda'}
          </button>
          <span className="text-xs font-bold text-white bg-blue-600 px-2.5 py-1 rounded-full">
            {futuros.length} próximos
          </span>
        </div>
      </div>

      {passadosPendentes.length > 0 && (
        <div>
          <h4 className="text-[11px] font-extrabold text-amber-600 uppercase tracking-wide px-1 mb-2">
            ⚠️ Atrasados (pendente de execução)
          </h4>
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-3 items-start">
            {passadosPendentes.map((i) => <Card key={i.id} i={i} />)}
          </div>
        </div>
      )}

      <div>
        <h4 className="text-[11px] font-extrabold text-slate-500 uppercase tracking-wide px-1 mb-2">
          Próximos agendamentos
        </h4>
        {futuros.length === 0 ? (
          <div className="bg-white p-8 rounded-2xl border border-slate-200 text-center text-slate-400 text-xs">
            Nenhum agendamento futuro. Use "Agendar Retorno" ou aprove um orçamento para criar um.
          </div>
        ) : (
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-3 items-start">
            {futuros.map((i) => <Card key={i.id} i={i} />)}
          </div>
        )}
      </div>

      {concluidosRecentes.length > 0 && (
        <div>
          <h4 className="text-[11px] font-extrabold text-emerald-600 uppercase tracking-wide px-1 mb-2">
            Concluídos recentemente
          </h4>
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-3 items-start">
            {concluidosRecentes.map((i) => <Card key={i.id} i={i} />)}
          </div>
        </div>
      )}
    </div>
  );
};
