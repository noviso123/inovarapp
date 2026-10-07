import React, { useState } from 'react';
import { Client } from '../types';
import { X, Save, UserCog, Loader2 } from 'lucide-react';
import { PuxarContato } from './PuxarContato';

interface EditarClienteModalProps {
  client: Client;
  onClose: () => void;
  onSave: (client: Client) => void;
}

// Edição completa do cadastro do cliente — salva no banco e sincroniza
export const EditarClienteModal: React.FC<EditarClienteModalProps> = ({ client, onClose, onSave }) => {
  const [nome, setNome] = useState(client.name);
  const [telefone, setTelefone] = useState(client.phone);
  const [endereco, setEndereco] = useState(client.address || '');
  const [bairro, setBairro] = useState(client.neighborhood || '');
  const [cidade, setCidade] = useState(client.city || '');
  const [observacoes, setObservacoes] = useState(client.notes || '');
  const [salvando, setSalvando] = useState(false);

  const inputCls = 'w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs';

  const salvar = (e: React.FormEvent) => {
    e.preventDefault();
    setSalvando(true);
    onSave({
      ...client,
      name: nome,
      phone: telefone,
      address: endereco,
      neighborhood: bairro,
      city: cidade,
      notes: observacoes
    });
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/80 backdrop-blur-xs overflow-y-auto">
      <div className="bg-white text-slate-900 rounded-2xl w-full max-w-md shadow-2xl border border-slate-200 overflow-hidden my-6">
        <div className="bg-slate-900 p-4 text-white flex items-center justify-between">
          <div className="flex items-center gap-2">
            <div className="p-2 bg-sky-500/20 text-sky-400 rounded-xl">
              <UserCog className="w-5 h-5" />
            </div>
            <div>
              <h3 className="font-bold text-sm">Editar Cliente</h3>
              <p className="text-[11px] text-slate-400">Salva no banco — visível em todas as plataformas</p>
            </div>
          </div>
          <button onClick={onClose} className="p-1.5 text-slate-400 hover:text-white rounded-lg">
            <X className="w-5 h-5" />
          </button>
        </div>

        <form onSubmit={salvar} className="p-5 max-h-[70vh] overflow-y-auto space-y-3 text-xs">
          {/* AUTO-PREENCHIMENTO: agenda nativa do celular + contatos do Google */}
          <PuxarContato
            onSelecionar={(c) => {
              if (c.nome) setNome(c.nome);
              if (c.telefone) setTelefone(c.telefone);
            }}
          />

          <div>
            <label className="block font-semibold text-slate-700 mb-1">Nome Completo *</label>
            <input type="text" required value={nome} onChange={e => setNome(e.target.value)} className={inputCls} />
          </div>
          <div className="grid grid-cols-2 gap-2">
            <div>
              <label className="block font-semibold text-slate-700 mb-1">WhatsApp / Telefone *</label>
              <input type="tel" required value={telefone} onChange={e => setTelefone(e.target.value)} className={inputCls} />
            </div>
            <div>
              <label className="block font-semibold text-slate-700 mb-1">Cidade</label>
              <input type="text" value={cidade} onChange={e => setCidade(e.target.value)} className={inputCls} />
            </div>
          </div>
          <div>
            <label className="block font-semibold text-slate-700 mb-1">Endereço (Rua, Número, Apto)</label>
            <input type="text" value={endereco} onChange={e => setEndereco(e.target.value)} className={inputCls} />
          </div>
          <div>
            <label className="block font-semibold text-slate-700 mb-1">Bairro</label>
            <input type="text" value={bairro} onChange={e => setBairro(e.target.value)} className={inputCls} />
          </div>
          <div>
            <label className="block font-semibold text-slate-700 mb-1">Observações</label>
            <textarea rows={2} value={observacoes} onChange={e => setObservacoes(e.target.value)} className={inputCls} />
          </div>

          <div className="pt-2 flex gap-2">
            <button type="button" onClick={onClose} className="flex-1 py-2 px-4 bg-slate-100 text-slate-700 rounded-xl font-semibold">
              Cancelar
            </button>
            <button
              type="submit"
              disabled={salvando}
              className="flex-1 py-2 px-4 bg-sky-600 hover:bg-sky-500 text-white rounded-xl font-bold flex items-center justify-center gap-1.5 disabled:opacity-50"
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
