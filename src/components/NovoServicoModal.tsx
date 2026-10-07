import React, { useState } from 'react';
import { Client } from '../types';
import { montarCatalogo, TipoCatalogo } from '../services/catalogo';
import { X, Save, Wrench, Loader2 } from 'lucide-react';

interface NovoServicoModalProps {
  clients: Client[];
  catalogo?: TipoCatalogo[];
  onClose: () => void;
  onSave: (dados: { clientId: string; applianceId: string; tipo: string; data: string; valor: number; status: string; observacoes: string }) => void;
}

// Cadastro DIRETO de serviço (sem checklist) — fica no banco e no fluxo
export const NovoServicoModal: React.FC<NovoServicoModalProps> = ({ clients, catalogo, onClose, onSave }) => {
  const amanha = new Date(Date.now() + 86400000).toISOString().slice(0, 10);
  const [clientId, setClientId] = useState(clients[0]?.id || '');
  const cliente = clients.find((c) => c.id === clientId);
  const [applianceId, setApplianceId] = useState(cliente?.appliances?.[0]?.id || '');
  const [tipo, setTipo] = useState('Limpeza de Ar');
  const [data, setData] = useState(amanha);
  const [valor, setValor] = useState(250);
  const [status, setStatus] = useState('AGENDADO');
  const [observacoes, setObservacoes] = useState('');
  const [salvando, setSalvando] = useState(false);

  const catalogoLista = catalogo && catalogo.length > 0 ? catalogo : montarCatalogo();
  const todosTipos = catalogoLista.map((t) => t.nome);

  const trocarTipo = (novoTipo: string) => {
    setTipo(novoTipo);
    const def = catalogoLista.find((t) => t.nome === novoTipo);
    if (def && def.preco > 0) setValor(def.preco);
  };

  const inputCls = 'w-full min-h-11 p-2.5 bg-white border border-slate-300 rounded-xl text-sm text-slate-800';

  const trocarCliente = (id: string) => {
    setClientId(id);
    const c = clients.find((x) => x.id === id);
    setApplianceId(c?.appliances?.[0]?.id || '');
  };

  const salvar = (e: React.FormEvent) => {
    e.preventDefault();
    if (!cliente || !applianceId) return;
    setSalvando(true);
    onSave({ clientId: cliente.id, applianceId, tipo, data, valor: Number(valor), status, observacoes });
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-xs overflow-y-auto">
      <div className="bg-white text-slate-900 rounded-2xl w-full max-w-md max-h-[calc(100dvh-1rem)] flex flex-col shadow-2xl border border-slate-200 overflow-hidden my-2 sm:my-6">
        <div className="bg-sky-50 border-b border-sky-100 p-4 text-slate-900 flex items-center justify-between shrink-0">
          <div className="flex items-center gap-2">
            <div className="p-2 bg-sky-100 text-sky-700 rounded-xl border border-sky-200">
              <Wrench className="w-5 h-5" />
            </div>
            <div>
              <h3 className="font-bold text-sm text-slate-900">Cadastrar Serviço</h3>
              <p className="text-[11px] text-slate-600">Registro direto — sem checklist</p>
            </div>
          </div>
          <button onClick={onClose} className="p-2 text-slate-500 hover:text-slate-900 hover:bg-white rounded-lg">
            <X className="w-5 h-5" />
          </button>
        </div>

        <form onSubmit={salvar} className="p-4 sm:p-5 space-y-3 text-xs overflow-y-auto overscroll-contain">
          <div>
            <label className="block font-semibold text-slate-700 mb-1">Cliente *</label>
            <select value={clientId} onChange={(e) => trocarCliente(e.target.value)} className={inputCls}>
              {clients.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>
          </div>

          <div>
            <label className="block font-semibold text-slate-700 mb-1">Aparelho * (obrigatório)</label>
            {cliente?.appliances && cliente.appliances.length > 0 ? (
              <select value={applianceId} onChange={(e) => setApplianceId(e.target.value)} className={inputCls}>
                {cliente.appliances.map((a) => (
                  <option key={a.id} value={a.id}>{a.brand} {a.model || ''} {a.capacityBtu} BTUs ({a.room})</option>
                ))}
              </select>
            ) : (
              <p className="p-2 bg-amber-50 border border-amber-200 rounded-lg text-[11px] text-amber-700">
                Este cliente não tem aparelhos. Cadastre o aparelho dele primeiro.
              </p>
            )}
          </div>

          <div>
            <label className="block font-semibold text-slate-700 mb-1">Tipo de Serviço *</label>
            <select value={tipo} onChange={(e) => trocarTipo(e.target.value)} className={inputCls}>
              {todosTipos.map((t) => <option key={t} value={t}>{t}</option>)}
            </select>
          </div>

          <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
            <div className="col-span-2 sm:col-span-1">
              <label className="block font-semibold text-slate-700 mb-1">Data *</label>
              <input type="date" required value={data} onChange={(e) => setData(e.target.value)} className={inputCls} />
            </div>
            <div>
              <label className="block font-semibold text-slate-700 mb-1">Valor (R$)</label>
              <input type="number" min="0" step="10" value={valor} onChange={(e) => setValor(Number(e.target.value))} className={inputCls + ' font-bold'} />
            </div>
            <div>
              <label className="block font-semibold text-slate-700 mb-1">Status *</label>
              <select value={status} onChange={(e) => setStatus(e.target.value)} className={inputCls}>
                <option value="AGENDADO">Agendado</option>
                <option value="CONCLUIDO">Concluído</option>
                <option value="PENDENTE">Pendente</option>
              </select>
            </div>
          </div>

          <div>
            <label className="block font-semibold text-slate-700 mb-1">Observações</label>
            <textarea rows={2} value={observacoes} onChange={(e) => setObservacoes(e.target.value)} className={inputCls} placeholder="Ex: levar escada, confirmar horário..." />
          </div>

          <div className="pt-2 flex gap-2 shrink-0">
            <button type="button" onClick={onClose} className="flex-1 min-h-11 py-2 px-3 bg-slate-100 text-slate-700 rounded-xl font-semibold">
              Cancelar
            </button>
            <button
              type="submit"
              disabled={salvando || !applianceId}
              className="flex-1 min-h-11 py-2 px-3 bg-sky-600 hover:bg-sky-500 text-white rounded-xl font-bold flex items-center justify-center gap-1.5 disabled:opacity-50"
            >
              {salvando ? <Loader2 className="w-4 h-4 animate-spin" /> : <Save className="w-4 h-4" />}
              <span>Cadastrar Serviço</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
