import React, { useState } from 'react';
import { format, addDays } from 'date-fns';
import { X, CalendarClock, Loader2 } from 'lucide-react';

interface AgendarChamadoModalProps {
  chamado: { id: string; cliente: string; servico: string; aparelho: string };
  onClose: () => void | Promise<void>;
  onSave: (serviceId: string, data: string, hora: string, observacoes: string) => void;
}

// Agendamento completo a partir do chamado do cliente: data + hora + obs.
// Salva no banco (service AGENDADO + appointment) e avisa o cliente no WhatsApp.
export const AgendarChamadoModal: React.FC<AgendarChamadoModalProps> = ({ chamado, onClose, onSave }) => {
  const amanha = format(addDays(new Date(), 1), 'yyyy-MM-dd');
  const [data, setData] = useState(amanha);
  const [hora, setHora] = useState('09:00');
  const [obs, setObs] = useState('');
  const [salvando, setSalvando] = useState(false);

  const inputCls = 'w-full p-2 bg-white border border-slate-300 rounded-lg text-xs';

  const salvar = async (e: React.FormEvent) => {
    e.preventDefault();
    if (salvando) return;
    setSalvando(true);
    try { await onSave(chamado.id, data, hora, obs); } finally { setSalvando(false); }
  };

  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-xs overflow-y-auto">
      <div className="bg-white text-slate-900 rounded-2xl w-full max-w-md shadow-2xl border border-slate-200 overflow-hidden my-6">
        <div className="bg-slate-900 p-4 text-white flex items-center justify-between">
          <div className="flex items-center gap-2">
            <div className="p-2 bg-blue-500/20 text-blue-400 rounded-xl">
              <CalendarClock className="w-5 h-5" />
            </div>
            <div>
              <h3 className="font-bold text-sm">Agendar Atendimento</h3>
              <p className="text-[11px] text-slate-400 truncate">{chamado.cliente} • {chamado.servico}</p>
            </div>
          </div>
          <button onClick={onClose} className="p-1.5 text-slate-400 hover:text-white rounded-lg">
            <X className="w-5 h-5" />
          </button>
        </div>

        <form
          onSubmit={salvar}
          className="p-5 space-y-3 text-xs"
        >
          <div className="p-2.5 bg-slate-100 rounded-xl text-[11px] text-slate-600">
            Aparelho: <b>{chamado.aparelho || 'não informado'}</b>
          </div>

          <div className="grid grid-cols-2 gap-2">
            <div>
              <label className="block font-semibold text-slate-700 mb-1">Data do Atendimento *</label>
              <input type="date" required value={data} onChange={e => setData(e.target.value)} className={inputCls} />
            </div>
            <div>
              <label className="block font-semibold text-slate-700 mb-1">Horário *</label>
              <input type="time" required value={hora} onChange={e => setHora(e.target.value)} className={inputCls} />
            </div>
          </div>

          <div>
            <label className="block font-semibold text-slate-700 mb-1">Observações do agendamento</label>
            <textarea rows={2} value={obs} onChange={e => setObs(e.target.value)} placeholder="Ex: confirmar chegada 15 min antes..." className={inputCls} />
          </div>

          <div className="p-2.5 bg-blue-50 border border-blue-200 rounded-xl text-[11px] text-blue-700 flex items-center gap-2">
            <CalendarClock className="w-4 h-4 shrink-0" />
            <span>Ao salvar: a confirmação entra na fila do WhatsApp e o serviço entra na Agenda. Se a conexão estiver temporariamente indisponível, o envio será tentado novamente.</span>
          </div>

          <div className="pt-1 flex gap-2">
            <button type="button" onClick={onClose} className="flex-1 py-2.5 px-4 bg-slate-100 text-slate-700 rounded-xl font-semibold">
              Cancelar
            </button>
            <button
              type="submit"
              disabled={salvando}
              className="flex-1 py-2.5 px-4 bg-blue-600 hover:bg-blue-500 text-white rounded-xl font-bold flex items-center justify-center gap-1.5 disabled:opacity-50"
            >
              {salvando ? <Loader2 className="w-4 h-4 animate-spin" /> : <CalendarClock className="w-4 h-4" />}
              <span>Confirmar Agendamento</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
