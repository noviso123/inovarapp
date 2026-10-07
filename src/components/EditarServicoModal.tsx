import React, { useState } from 'react';
import { X, Save, Wrench, Loader2 } from 'lucide-react';
import { format, parseISO } from 'date-fns';
import { montarCatalogo, TipoCatalogo } from '../services/catalogo';

interface EditarServicoModalProps {
  catalogo?: TipoCatalogo[];
  servico: {
    id: string;
    tipo: string;
    data: string;
    valor: number;
    cliente: string;
    aparelho: string;
    observacoes?: string;
  };
  onClose: () => void;
  onSave: (id: string, dados: { tipo: string; data: string; valor: number; observacoes: string }) => void;
}

// Edição completa de um serviço/agendamento — salva no banco e sincroniza
export const EditarServicoModal: React.FC<EditarServicoModalProps> = ({ catalogo, servico, onClose, onSave }) => {
  const [tipo, setTipo] = useState(servico.tipo);
  const [data, setData] = useState(servico.data);
  const [valor, setValor] = useState(servico.valor);
  const [observacoes, setObservacoes] = useState(servico.observacoes || '');
  const [salvando, setSalvando] = useState(false);

  // Opções do catálogo atual + tipo atual do registro (mesmo que tenha sido removido) + Outro
  const todosTipos = (() => {
    const lista = (catalogo && catalogo.length > 0 ? catalogo : montarCatalogo()).map((t) => t.nome);
    if (servico.tipo && !lista.includes(servico.tipo)) lista.unshift(servico.tipo);
    if (!lista.includes('Outro')) lista.push('Outro');
    return lista;
  })();

  const inputCls = 'w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs';

  const salvar = (e: React.FormEvent) => {
    e.preventDefault();
    setSalvando(true);
    onSave(servico.id, { tipo, data, valor: Number(valor), observacoes });
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-xs overflow-y-auto">
      <div className="bg-white text-slate-900 rounded-2xl w-full max-w-md shadow-2xl border border-slate-200 overflow-hidden my-6">
        <div className="bg-slate-900 p-4 text-white flex items-center justify-between">
          <div className="flex items-center gap-2">
            <div className="p-2 bg-amber-500/20 text-amber-400 rounded-xl">
              <Wrench className="w-5 h-5" />
            </div>
            <div>
              <h3 className="font-bold text-sm">Editar Serviço</h3>
              <p className="text-[11px] text-slate-400">{servico.cliente} • {servico.aparelho}</p>
            </div>
          </div>
          <button onClick={onClose} className="p-1.5 text-slate-400 hover:text-white rounded-lg">
            <X className="w-5 h-5" />
          </button>
        </div>

        <form onSubmit={salvar} className="p-5 space-y-3 text-xs">
          <div>
            <label className="block font-semibold text-slate-700 mb-1">Tipo de Serviço</label>
            <select value={tipo} onChange={e => setTipo(e.target.value)} className={inputCls}>
              {todosTipos.map((t) => <option key={t} value={t}>{t}</option>)}
            </select>
          </div>

          <div className="grid grid-cols-2 gap-2">
            <div>
              <label className="block font-semibold text-slate-700 mb-1">Data</label>
              <input type="date" required value={data} onChange={e => setData(e.target.value)} className={inputCls} />
            </div>
            <div>
              <label className="block font-semibold text-slate-700 mb-1">Valor (R$)</label>
              <input type="number" min="0" step="10" value={valor} onChange={e => setValor(Number(e.target.value))} className={inputCls + ' font-bold'} />
            </div>
          </div>

          <div>
            <label className="block font-semibold text-slate-700 mb-1">Observações</label>
            <textarea rows={2} value={observacoes} onChange={e => setObservacoes(e.target.value)} className={inputCls} />
          </div>

          {data !== servico.data && (
            <div className="p-2 bg-amber-50 border border-amber-200 rounded-lg text-[11px] text-amber-700">
              A data mudou de {servico.data ? format(parseISO(servico.data + 'T00:00:00'), 'dd/MM/yyyy') : '(sem data)'} para {data ? format(parseISO(data + 'T00:00:00'), 'dd/MM/yyyy') : '(sem data)'} — o cliente não será notificado automaticamente desta edição.
            </div>
          )}

          <div className="pt-2 flex gap-2">
            <button type="button" onClick={onClose} className="flex-1 py-2 px-4 bg-slate-100 text-slate-700 rounded-xl font-semibold">
              Cancelar
            </button>
            <button
              type="submit"
              disabled={salvando}
              className="flex-1 py-2 px-4 bg-amber-600 hover:bg-amber-500 text-white rounded-xl font-bold flex items-center justify-center gap-1.5 disabled:opacity-50"
            >
              {salvando ? <Loader2 className="w-4 h-4 animate-spin" /> : <Save className="w-4 h-4" />}
              <span>Salvar no Banco</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
