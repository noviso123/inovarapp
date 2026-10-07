import React, { useState } from 'react';
import { Client, Appliance, TechnicianProfile } from '../types';
import { TipoCatalogo } from '../services/catalogo';
import { X, History, Save } from 'lucide-react';
import { format, subMonths } from 'date-fns';

interface NovoHistoricoModalProps {
  clients: Client[];
  catalogo: TipoCatalogo[];
  profile: TechnicianProfile;
  /** Cliente/aparelho fixado (ficha ou fila). Opcional: sem ele, a modal mostra seletores. */
  target?: { client: Client; appliance: Appliance };
  onClose: () => void;
  onSalvar: (
    client: Client,
    appliance: Appliance,
    dados: { data: string; tipo: string; valor: number; observacoes?: string; atualizarRetorno: boolean }
  ) => void;
}

// Registro RETROATIVO de serviço (histórico anterior à adoção do app):
// cria OS concluída no passado e inicia o ciclo de retorno a partir da data informada.
export const NovoHistoricoModal: React.FC<NovoHistoricoModalProps> = ({
  clients,
  catalogo,
  profile,
  target,
  onClose,
  onSalvar
}) => {
  const [clientId, setClientId] = useState(target?.client.id || clients[0]?.id || '');
  const cliente = clients.find((c) => c.id === clientId);
  const [applianceId, setApplianceId] = useState(target?.appliance.id || cliente?.appliances?.[0]?.id || '');
  const aparelho = cliente?.appliances?.find((a) => a.id === applianceId) || cliente?.appliances?.[0];

  const [tipo, setTipo] = useState(catalogo[0]?.nome || 'Limpeza de Ar');
  const [data, setData] = useState(format(subMonths(new Date(), profile.defaultReturnMonths || 6), 'yyyy-MM-dd'));
  const [valor, setValor] = useState<number>(catalogo[0]?.preco || profile.defaultPrice || 250);
  const [observacoes, setObservacoes] = useState('');
  const [atualizarRetorno, setAtualizarRetorno] = useState(true);
  const [salvando, setSalvando] = useState(false);

  const trocarCliente = (id: string) => {
    setClientId(id);
    const c = clients.find((x) => x.id === id);
    setApplianceId(c?.appliances?.[0]?.id || '');
  };

  const trocarTipo = (novoTipo: string) => {
    setTipo(novoTipo);
    const def = catalogo.find((t) => t.nome === novoTipo);
    if (def && def.preco > 0) setValor(def.preco);
  };

  const salvar = (e: React.FormEvent) => {
    e.preventDefault();
    if (!cliente || !aparelho || !data) return;
    setSalvando(true);
    onSalvar(cliente, aparelho, { data, tipo, valor: Number(valor) || 0, observacoes: observacoes.trim() || undefined, atualizarRetorno });
  };

  const inputCls = 'w-full p-2.5 bg-slate-800/90 border border-slate-700 rounded-xl text-xs text-white placeholder-slate-500 focus:outline-none focus:border-sky-500';
  const hoje = format(new Date(), 'yyyy-MM-dd');

  return (
    <div className="fixed inset-0 z-[65] flex items-center justify-center p-3 sm:p-4 bg-black/85 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="bg-slate-900 border border-slate-700 rounded-2xl w-full max-w-md shadow-2xl text-slate-100 overflow-hidden flex flex-col max-h-[92vh]">
        {/* Header */}
        <div className="p-4 bg-gradient-to-r from-inovar-navy via-slate-900 to-slate-950 border-b border-slate-800 flex items-center justify-between shrink-0">
          <div className="flex items-center gap-2.5">
            <div className="p-2 bg-violet-500/20 text-violet-300 rounded-xl border border-violet-500/30">
              <History className="w-4 h-4" />
            </div>
            <div>
              <span className="text-[10px] font-bold text-inovar-yellow uppercase tracking-wider block">
                CONTROLE ANTES DO APP
              </span>
              <h3 className="text-sm font-bold text-white">Registrar Histórico Anterior</h3>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 text-slate-400 hover:text-white rounded-lg hover:bg-slate-800 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <form onSubmit={salvar} className="p-4 space-y-3 overflow-y-auto">
          <p className="text-[11px] text-slate-400 leading-relaxed">
            Cadastre serviços que o cliente <b className="text-slate-200">já fez antes</b> (com você ou com outro técnico).
            O ciclo de retorno passa a contar a partir da data informada e o aparelho entra na fila com status real.
          </p>

          {/* Cliente / Aparelho — só aparecem quando não fixados */}
          {!target && (
            <div className="space-y-2">
              <div>
                <label className="block text-[11px] font-bold text-slate-300 uppercase tracking-wide mb-1">Cliente *</label>
                <select value={clientId} onChange={(e) => trocarCliente(e.target.value)} className={inputCls}>
                  {clients.map((c) => (
                    <option key={c.id} value={c.id}>{c.name}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-[11px] font-bold text-slate-300 uppercase tracking-wide mb-1">Aparelho *</label>
                {cliente?.appliances && cliente.appliances.length > 0 ? (
                  <select value={applianceId} onChange={(e) => setApplianceId(e.target.value)} className={inputCls}>
                    {cliente.appliances.map((a) => (
                      <option key={a.id} value={a.id}>{a.brand} {a.capacityBtu} BTUs — {a.room}</option>
                    ))}
                  </select>
                ) : (
                  <p className="text-[11px] text-amber-400 bg-amber-500/10 border border-amber-500/30 rounded-lg p-2">
                    Este cliente não tem aparelhos cadastrados. Cadastre o aparelho primeiro (aba Clientes).
                  </p>
                )}
              </div>
            </div>
          )}

          {/* Alvo fixado */}
          {target && (
            <div className="p-3 bg-slate-800/70 rounded-xl border border-slate-700">
              <p className="text-xs font-bold text-white">{target.client.name}</p>
              <p className="text-[11px] text-slate-400">
                {target.appliance.brand} {target.appliance.capacityBtu} BTUs • {target.appliance.room}
              </p>
            </div>
          )}

          <div className="grid grid-cols-2 gap-2">
            <div>
              <label className="block text-[11px] font-bold text-slate-300 uppercase tracking-wide mb-1">Tipo de Serviço</label>
              <select value={tipo} onChange={(e) => trocarTipo(e.target.value)} className={inputCls}>
                {(catalogo.length > 0 ? catalogo : [{ nome: tipo, preco: 0 }]).map((t) => (
                  <option key={t.nome} value={t.nome}>{t.nome}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-[11px] font-bold text-slate-300 uppercase tracking-wide mb-1">Valor (R$)</label>
              <input
                type="number" min="0" step="10"
                value={valor}
                onChange={(e) => setValor(Number(e.target.value))}
                className={inputCls}
              />
            </div>
          </div>

          <div>
            <label className="block text-[11px] font-bold text-slate-300 uppercase tracking-wide mb-1">
              Data em que o serviço foi feito *
            </label>
            <input
              type="date"
              required
              max={hoje}
              value={data}
              onChange={(e) => setData(e.target.value)}
              className={inputCls}
            />
            <p className="text-[10px] text-slate-500 mt-1">
              Datas passadas entram como OS concluída no histórico (não dispara WhatsApp ao cliente).
            </p>
          </div>

          <div>
            <label className="block text-[11px] font-bold text-slate-300 uppercase tracking-wide mb-1">Observações (opcional)</label>
            <textarea
              rows={2}
              value={observacoes}
              onChange={(e) => setObservacoes(e.target.value)}
              placeholder="Ex.: feito pelo técnico anterior, sem garantia registrada..."
              className={inputCls + ' resize-y'}
            />
          </div>

          <label className="flex items-start gap-2 p-3 bg-slate-800/60 border border-slate-700 rounded-xl cursor-pointer select-none">
            <input
              type="checkbox"
              checked={atualizarRetorno}
              onChange={(e) => setAtualizarRetorno(e.target.checked)}
              className="mt-0.5 accent-sky-500"
            />
            <span className="text-[11px] text-slate-300 leading-snug">
              Iniciar o ciclo de retorno a partir desta data
              <span className="block text-[10px] text-slate-500">
                Próxima manutenção cai em ~{profile.defaultReturnMonths || 6} meses após {data.split('-').reverse().join('/')}.
                Desmarque se o serviço anterior não vale como ciclo (ex.: só uma visita orçada).
              </span>
            </span>
          </label>

          <div className="flex gap-2 pt-1">
            <button
              type="button"
              onClick={onClose}
              className="flex-1 py-2.5 bg-slate-800 hover:bg-slate-750 text-slate-200 border border-slate-700 rounded-xl text-xs font-bold transition-colors"
            >
              Cancelar
            </button>
            <button
              type="submit"
              disabled={!cliente || !aparelho || salvando}
              className="flex-1 py-2.5 bg-violet-600 hover:bg-violet-500 disabled:opacity-40 disabled:cursor-not-allowed text-white rounded-xl text-xs font-extrabold flex items-center justify-center gap-1.5 transition-all active:scale-95"
            >
              <Save className="w-4 h-4" />
              <span>Registrar Histórico</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
