import React, { useState } from 'react';
import { Client, Appliance, ServiceType } from '../types';
import { montarCatalogo, TipoCatalogo } from '../services/catalogo';
import {
  X,
  Wrench,
  Search,
  ChevronRight
} from 'lucide-react';

interface IniciarServicoModalProps {
  clients: Client[];
  initialServiceType?: ServiceType;
  onClose: () => void;
  onSelect: (client: Client, appliance: Appliance, serviceType: ServiceType) => void | Promise<void>;
  catalogo?: TipoCatalogo[];
}

export const IniciarServicoModal: React.FC<IniciarServicoModalProps> = ({
  clients,
  initialServiceType = 'Limpeza de Ar',
  onClose,
  onSelect,
  catalogo
}) => {
  const [saving, setSaving] = useState(false);
  const choose = async (client: Client, appliance: Appliance, service: ServiceType) => { if (saving) return; setSaving(true); try { await onSelect(client, appliance, service); } finally { setSaving(false); } };
  const [selectedService, setSelectedService] = useState<ServiceType>(initialServiceType);
  const [step, setStep] = useState<'servico' | 'cliente'>('servico');
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedClient, setSelectedClient] = useState<Client | null>(null);

  // Catálogo resolvido: fixos (editáveis/removíveis) + personalizados cadastrados pelo admin
  const SERVICOS_INOVAR = (catalogo && catalogo.length > 0 ? catalogo : montarCatalogo()).map((t) => ({
    type: (t.fixo || t.nome) as ServiceType,
    title: t.nome,
    desc: t.card
      ? t.card.desc
      : 'Serviço personalizado da Inovar' + (t.preco ? ` • R$ ${t.preco.toLocaleString('pt-BR')}` : ''),
    icon: t.card?.icon || Wrench,
    color: t.card?.color || 'sky',
    badge: t.card?.badge || 'Personalizado'
  }));

  const filteredClients = clients.filter(
    (c) =>
      c.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      c.phone.includes(searchTerm) ||
      (c.neighborhood && c.neighborhood.toLowerCase().includes(searchTerm.toLowerCase()))
  );

  const handleChooseClient = (client: Client) => {
    setSelectedClient(client);
    if (client.appliances && client.appliances.length === 1) {
      choose(client, client.appliances[0], selectedService);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-2 sm:p-4 bg-slate-950/60 backdrop-blur-sm animate-in fade-in duration-200">
      <div role="dialog" aria-modal="true" aria-labelledby="iniciar-servico-titulo" className="bg-white border border-slate-200 rounded-2xl w-full max-w-2xl max-h-[calc(100dvh-1rem)] sm:max-h-[90dvh] flex flex-col shadow-2xl text-slate-900 overflow-hidden">
        {/* Top Header */}
        <div className="p-4 bg-slate-50 border-b border-slate-200 flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <div className="p-2 bg-amber-100 text-amber-700 rounded-xl border border-amber-200">
              <Wrench className="w-5 h-5" />
            </div>
            <div>
              <span className="text-[10px] font-bold text-amber-700 uppercase tracking-wider block">
                INOVAR REFRIGERAÇÃO • EXECUÇÃO DE CAMPO
              </span>
              <h3 id="iniciar-servico-titulo" className="text-base font-bold text-slate-900">
                {step === 'servico' ? 'Selecione o Tipo de Serviço' : 'Selecione o Cliente & Aparelho'}
              </h3>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-2 text-slate-500 hover:text-slate-900 rounded-lg hover:bg-slate-200 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Step 1: Escolha do Serviço */}
        {step === 'servico' && (
          <div className="p-4 overflow-y-auto space-y-2.5">
            <p className="text-xs leading-relaxed text-slate-600 mb-2">
              Escolha qual procedimento técnico você vai realizar no cliente para abrir o checklist adequado:
            </p>

            <div className="grid grid-cols-1 gap-2.5">
              {SERVICOS_INOVAR.map((s) => {
                const IconComponent = s.icon;
                const isSelected = selectedService === s.type;

                return (
                  <button
                    key={s.type}
                    onClick={() => {
                      setSelectedService(s.type);
                      setStep('cliente');
                    }}
                    className={`p-3 rounded-xl border text-left flex items-center justify-between transition-all active:scale-[0.99] group ${
                      isSelected
                        ? 'bg-sky-50 border-sky-400 shadow-sm'
                        : 'bg-white hover:bg-slate-50 border-slate-200 hover:border-sky-300'
                    }`}
                  >
                    <div className="flex items-center gap-3 min-w-0">
                      <div
                        className={`w-10 h-10 rounded-xl flex items-center justify-center shrink-0 ${
                          s.color === 'emerald'
                            ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/30'
                            : s.color === 'sky'
                            ? 'bg-sky-500/20 text-sky-400 border border-sky-500/30'
                            : s.color === 'amber'
                            ? 'bg-amber-500/20 text-amber-400 border border-amber-500/30'
                            : s.color === 'purple'
                            ? 'bg-purple-500/20 text-purple-400 border border-purple-500/30'
                            : s.color === 'blue'
                            ? 'bg-blue-500/20 text-blue-400 border border-blue-500/30'
                            : 'bg-slate-100 text-slate-600 border border-slate-200'
                        }`}
                      >
                        <IconComponent className="w-5 h-5" />
                      </div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="font-bold text-sm text-slate-900 group-hover:text-sky-700 transition-colors">
                            {s.title}
                          </span>
                          <span className="text-[10px] font-semibold px-1.5 py-0.5 rounded bg-slate-100 text-slate-600 border border-slate-200">
                            {s.badge}
                          </span>
                        </div>
                        <p className="text-[11px] text-slate-600 truncate mt-0.5">{s.desc}</p>
                      </div>
                    </div>

                    <ChevronRight className="w-4 h-4 text-slate-400 group-hover:text-sky-700 shrink-0 ml-2" />
                  </button>
                );
              })}
            </div>
          </div>
        )}

        {/* Step 2: Escolha do Cliente */}
        {step === 'cliente' && !selectedClient && (
          <div className="p-4 flex flex-col flex-1 overflow-hidden">
            <div className="flex items-center justify-between mb-3">
              <button
                onClick={() => setStep('servico')}
                className="text-xs text-sky-700 hover:text-sky-900 flex items-center gap-1 font-semibold"
              >
                ← Voltar para tipos de serviço
              </button>
              <span className="text-[11px] font-bold text-slate-600 px-2 py-1 bg-slate-100 rounded-md">
                Serviço: <span className="text-sky-700">{selectedService}</span>
              </span>
            </div>

            <div className="relative mb-3">
              <Search className="w-4 h-4 text-slate-500 absolute left-3 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                placeholder="Buscar cliente por nome, telefone ou bairro..."
                className="w-full pl-9 pr-3 py-2 bg-white border border-slate-300 rounded-xl text-xs text-slate-900 placeholder-slate-400 focus:outline-none focus:border-sky-500"
                autoFocus
              />
            </div>

            <div className="overflow-y-auto space-y-2 flex-1 max-h-[50vh]">
              {filteredClients.length === 0 ? (
                <div className="text-center py-8 text-slate-500 text-xs">
                  Nenhum cliente encontrado com "{searchTerm}".
                </div>
              ) : (
                filteredClients.map((client) => (
                  <div
                    key={client.id}
                    onClick={() => handleChooseClient(client)}
                    className="p-3 bg-white hover:bg-sky-50 border border-slate-200 hover:border-sky-300 rounded-xl cursor-pointer transition-all flex items-center justify-between group"
                  >
                    <div>
                      <h4 className="font-bold text-sm text-slate-900 group-hover:text-sky-700 transition-colors">
                        {client.name}
                      </h4>
                      <p className="text-[11px] text-slate-600">
                        {client.phone} • {client.neighborhood || client.city || 'Serra - ES'}
                      </p>
                      <div className="mt-1 flex items-center gap-1 text-[10px] text-slate-500">
                        <span className="font-semibold text-slate-700">
                          {client.appliances?.length || 0} aparelho(s):
                        </span>
                        <span>
                          {client.appliances?.map((a) => `${a.brand} (${a.room})`).join(', ') || 'Nenhum'}
                        </span>
                      </div>
                    </div>

                    <ChevronRight className="w-4 h-4 text-slate-400 group-hover:text-sky-700 shrink-0 ml-2" />
                  </div>
                ))
              )}
            </div>
          </div>
        )}

        {/* Step 2.5: Aparelho selecionado */}
        {step === 'cliente' && selectedClient && (
          <div className="p-4 overflow-y-auto space-y-3">
            <div className="flex items-center justify-between mb-2">
              <button
                onClick={() => setSelectedClient(null)}
                className="text-xs text-sky-700 hover:text-sky-900 flex items-center gap-1 font-semibold"
              >
                ← Escolher outro cliente
              </button>
              <span className="text-[11px] font-bold text-slate-600 px-2 py-1 bg-slate-100 rounded-md">
                {selectedService}
              </span>
            </div>

            <div className="p-3 bg-slate-50 rounded-xl border border-slate-200">
              <p className="text-xs font-bold text-slate-900">{selectedClient.name}</p>
              <p className="text-[11px] text-slate-600">{selectedClient.phone} • {selectedClient.address}</p>
            </div>

            <p className="text-xs text-slate-700 font-semibold pt-1">
              Qual aparelho de {selectedClient.name.split(' ')[0]} receberá o serviço?
            </p>

            <div className="space-y-2">
              {selectedClient.appliances && selectedClient.appliances.length > 0 ? (
                selectedClient.appliances.map((app) => (
                  <button
                    key={app.id}
                    disabled={saving} onClick={() => choose(selectedClient, app, selectedService)}
                    className="w-full p-3 bg-white hover:bg-sky-50 border border-slate-200 hover:border-sky-300 rounded-xl text-left flex items-center justify-between transition-all group disabled:opacity-60"
                  >
                    <div>
                      <h5 className="font-bold text-sm text-slate-900 group-hover:text-sky-700">
                        {app.brand} {app.capacityBtu ? `${app.capacityBtu} BTUs` : ''}
                      </h5>
                      <p className="text-[11px] text-slate-600">
                        Ambiente: <span className="text-slate-800 font-medium">{app.room}</span> • Tipo: {app.type}
                      </p>
                    </div>

                    <div className="px-3 py-1 bg-sky-100 text-sky-700 border border-sky-200 rounded-lg text-xs font-bold group-hover:bg-sky-600 group-hover:text-white transition-colors">
                      Ordem de Serviço
                    </div>
                  </button>
                ))
              ) : (
                <div className="p-4 text-center text-slate-500 text-xs">
                  Este cliente não possui aparelhos cadastrados.
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
