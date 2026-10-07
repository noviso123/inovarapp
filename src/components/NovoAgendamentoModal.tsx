import React, { useState } from 'react';
import { Client, Appliance, MaintenanceRecord, TechnicianProfile } from '../types';
import { montarCatalogo, TipoCatalogo } from '../services/catalogo';
import { X, Calendar, Save, Clock } from 'lucide-react';
import { format, addMonths } from 'date-fns';

interface NovoAgendamentoModalProps {
  clients: Client[];
  profile: TechnicianProfile;
  catalogo?: TipoCatalogo[];
  initialClient?: Client;
  initialAppliance?: Appliance;
  onClose: () => void;
  onSave: (record: MaintenanceRecord) => void | Promise<void>;
}

export const NovoAgendamentoModal: React.FC<NovoAgendamentoModalProps> = ({
  clients,
  profile,
  catalogo,
  initialClient,
  initialAppliance,
  onClose,
  onSave
}) => {
  const [selectedClientId, setSelectedClientId] = useState(initialClient?.id || (clients[0]?.id ?? ''));
  const currentClient = clients.find(c => c.id === selectedClientId) || clients[0];
  const [selectedApplianceId, setSelectedApplianceId] = useState(
    initialAppliance?.id || (currentClient?.appliances[0]?.id ?? '')
  );

  const [date, setDate] = useState(format(new Date(), 'yyyy-MM-dd'));
  const [time, setTime] = useState('09:00');
  const [returnDate, setReturnDate] = useState(format(addMonths(new Date(), 6), 'yyyy-MM-dd'));
  const [serviceType, setServiceType] = useState<MaintenanceRecord['serviceType']>('Limpeza de Ar');
  const [price, setPrice] = useState(profile.defaultPrice || 250);
  const [notes, setNotes] = useState('');

  const [saving, setSaving] = useState(false);
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (saving || !currentClient || !selectedApplianceId) return;
    setSaving(true);

    const record: MaintenanceRecord = {
      id: 'm_' + Math.random().toString(36).substring(2, 9),
      clientId: currentClient.id,
      applianceId: selectedApplianceId,
      date,
      scheduledDate: date,
      scheduledTime: time,
      returnDate,
      serviceType,
      price: Number(price),
      paymentMethod: 'PIX',
      warrantyDays: profile.defaultWarrantyDays || 90,
      notes,
      status: 'agendado'
    };

    try { await onSave(record); } finally { setSaving(false); }
  };

  const podeSalvar = !!currentClient && !!selectedApplianceId && currentClient.appliances.length > 0;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-xs overflow-y-auto">
      <div className="bg-white text-slate-900 rounded-2xl w-full max-w-md shadow-2xl border border-slate-200 overflow-hidden my-6">
        <div className="bg-slate-900 p-4 text-white flex items-center justify-between">
          <div className="flex items-center gap-2">
            <div className="p-2 bg-sky-500/20 text-sky-400 rounded-xl">
              <Calendar className="w-5 h-5" />
            </div>
            <div>
              <h3 className="font-bold text-sm">Agendar Retorno / Manutenção</h3>
              <p className="text-[11px] text-slate-400">Defina o cliente e a data prevista de retorno</p>
            </div>
          </div>
          <button onClick={onClose} className="p-1.5 text-slate-400 hover:text-white rounded-lg">
            <X className="w-5 h-5" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-5 space-y-3.5 text-xs">
          {/* Client select */}
          <div>
            <label className="block font-semibold text-slate-700 mb-1">Cliente</label>
            <select
              value={selectedClientId}
              onChange={e => {
                setSelectedClientId(e.target.value);
                const c = clients.find(cl => cl.id === e.target.value);
                if (c && c.appliances.length > 0) {
                  setSelectedApplianceId(c.appliances[0].id);
                }
              }}
              className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs font-medium"
            >
              {clients.map(c => (
                <option key={c.id} value={c.id}>{c.name}</option>
              ))}
            </select>
          </div>

          {/* Appliance select */}
          <div>
            <label className="block font-semibold text-slate-700 mb-1">Aparelho do Cliente</label>
            <select
              value={selectedApplianceId}
              onChange={e => setSelectedApplianceId(e.target.value)}
              className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
            >
              {currentClient?.appliances.map(a => (
                <option key={a.id} value={a.id}>
                  {a.brand} {a.capacityBtu} BTUs ({a.room})
                </option>
              ))}
            </select>
          </div>

          <div className="grid grid-cols-3 gap-2">
            <div>
              <label className="block font-semibold text-slate-700 mb-1">Data do Serviço</label>
              <input
                type="date"
                required
                value={date}
                onChange={e => setDate(e.target.value)}
                className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
              />
            </div>

            <div>
              <label className="block font-semibold text-slate-700 mb-1">Horário</label>
              <input type="time" required value={time} onChange={e => setTime(e.target.value)} className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs" />
            </div>

            <div>
              <label className="block font-semibold text-slate-700 mb-1">Data do Retorno</label>
              <input
                type="date"
                required
                value={returnDate}
                onChange={e => setReturnDate(e.target.value)}
                className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs font-bold text-sky-700"
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-2">
            <div>
              <label className="block font-semibold text-slate-700 mb-1">Tipo de Serviço</label>
              <select
                value={serviceType}
                onChange={e => {
                  setServiceType(e.target.value as any);
                  const def = (catalogo && catalogo.length > 0 ? catalogo : montarCatalogo()).find((t) => t.nome === e.target.value);
                  if (def && def.preco > 0) setPrice(def.preco);
                }}
                className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
              >
                {(catalogo && catalogo.length > 0 ? catalogo : montarCatalogo()).map((t) => (
                  <option key={t.key} value={t.nome}>{t.nome}</option>
                ))}
              </select>
            </div>

            <div>
              <label className="block font-semibold text-slate-700 mb-1">Valor Estimado (R$)</label>
              <input
                type="number"
                value={price}
                onChange={e => setPrice(Number(e.target.value))}
                className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs font-bold"
              />
            </div>
          </div>

          <div>
            <label className="block font-semibold text-slate-700 mb-1">Anotações do Agendamento</label>
            <textarea
              rows={2}
              value={notes}
              onChange={e => setNotes(e.target.value)}
              placeholder="Ex: Ligar antes para confirmar se o cliente estará em casa..."
              className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
            />
          </div>

          <div className="pt-2 flex gap-2">
            <button
              type="button"
              onClick={onClose}
              className="flex-1 py-2 px-4 bg-slate-100 text-slate-700 rounded-xl font-semibold"
            >
              Cancelar
            </button>
            <button
              type="submit"
              disabled={!podeSalvar || saving}
              className="flex-1 py-2 px-4 bg-sky-600 hover:bg-sky-500 text-white rounded-xl font-bold flex items-center justify-center gap-1.5 disabled:opacity-40 disabled:cursor-not-allowed"
            >
              <Save className="w-4 h-4" />
              <span>Salvar Agendamento</span>
            </button>
          </div>
          {!podeSalvar && (
            <p className="text-[11px] text-amber-600 bg-amber-50 border border-amber-200 rounded-lg p-2">
              Este cliente ainda não possui aparelhos cadastrados. Cadastre um aparelho primeiro.
            </p>
          )}
        </form>
      </div>
    </div>
  );
};
