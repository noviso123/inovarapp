import React, { useState } from 'react';
import { Client, Appliance, MaintenanceRecord, BudgetEstimate } from '../types';
import { WhatsAppService } from '../services/whatsapp';
import { Search, Plus, Phone, MessageSquare, Wrench, ChevronDown, ChevronUp, MapPin, Wind, Trash2, UserCog, KeyRound, Copy, Check } from 'lucide-react';
import { supabase } from '../services/supabase';

interface ClientesTabProps {
  clients: Client[];
  onOpenNovoCliente: () => void;
  onOpenFicha: (client: Client, appliance: Appliance) => void;
  onOpenChecklist: (client: Client, appliance: Appliance) => void;
  onAddApplianceToClient: (client: Client) => void;
  onDeleteClient: (clientId: string) => void;
  onEditClient: (client: Client) => void;
  budgets: BudgetEstimate[];
  onDeleteAppliance: (clientId: string, applianceId: string) => void;
}

export const ClientesTab: React.FC<ClientesTabProps> = ({
  clients,
  onOpenNovoCliente,
  onOpenFicha,
  onOpenChecklist,
  onAddApplianceToClient,
  onDeleteClient,
  budgets,
  onEditClient,
  onDeleteAppliance
}) => {
  const [search, setSearch] = useState('');
  const [expandedClientId, setExpandedClientId] = useState<string | null>(null);
  const [resettingClientId, setResettingClientId] = useState<string | null>(null);
  const [temporaryAccess, setTemporaryAccess] = useState<{ email: string; senha: string } | null>(null);
  const [accessError, setAccessError] = useState('');
  const [copied, setCopied] = useState(false);

  const resetClientPassword = async (client: Client) => {
    if (!confirm(`Gerar uma nova senha temporária para ${client.name}? A senha atual deixará de funcionar.`)) return;
    setResettingClientId(client.id);
    setAccessError('');
    try {
      const { data } = await supabase.auth.getSession();
      const response = await fetch('/api/contas', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${data.session?.access_token || ''}` },
        body: JSON.stringify({ acao: 'redefinir_cliente', customer_id: client.id })
      });
      const result = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(result.error || 'Não foi possível redefinir a senha.');
      setTemporaryAccess({ email: result.email, senha: result.senha_temporaria });
    } catch (error: any) {
      setAccessError(error.message || 'Não foi possível redefinir a senha.');
    } finally {
      setResettingClientId(null);
    }
  };

  const filteredClients = clients.filter(c => {
    if (!search.trim()) return true;
    const q = search.toLowerCase();
    const matchName = c.name.toLowerCase().includes(q);
    const matchPhone = c.phone.includes(q);
    const matchNeighborhood = c.neighborhood?.toLowerCase().includes(q);
    const matchAppliance = c.appliances.some(a =>
      a.brand.toLowerCase().includes(q) ||
      a.capacityBtu.includes(q) ||
      a.room.toLowerCase().includes(q)
    );
    return matchName || matchPhone || matchNeighborhood || matchAppliance;
  });

  return (
    <div className="space-y-4">
      {/* Top Search & Add button */}
      <div className="bg-white p-4 rounded-2xl border border-slate-200 shadow-sm flex flex-col sm:flex-row items-center justify-between gap-3">
        <div className="relative w-full sm:w-80">
          <Search className="w-4 h-4 text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            placeholder="Buscar por cliente, telefone, aparelho..."
            value={search}
            onChange={e => setSearch(e.target.value)}
            className="w-full pl-9 pr-3 py-2 bg-slate-50 border border-slate-300 rounded-xl text-xs text-slate-800 focus:outline-none focus:ring-2 focus:ring-sky-500"
          />
        </div>

        <button
          onClick={onOpenNovoCliente}
          className="w-full sm:w-auto px-4 py-2 bg-sky-600 hover:bg-sky-500 text-white rounded-xl text-xs font-bold flex items-center justify-center gap-1.5 shadow-sm transition-all"
        >
          <Plus className="w-4 h-4" />
          <span>Cadastrar Novo Cliente</span>
        </button>
      </div>

      {/* Clients Cards List */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-3 items-start">
        {filteredClients.length === 0 ? (
          <div className="bg-white p-8 rounded-2xl border border-slate-200 text-center text-slate-400 text-xs">
            Nenhum cliente encontrado para "{search}".
          </div>
        ) : (
          filteredClients.map(client => {
            const isExpanded = expandedClientId === client.id;

            return (
              <div
                key={client.id}
                className="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden transition-all hover:border-slate-300"
              >
                {/* Header of client */}
                <div
                  onClick={() => setExpandedClientId(isExpanded ? null : client.id)}
                  className="p-4 flex items-center justify-between cursor-pointer select-none hover:bg-slate-50/70"
                >
                  <div className="flex items-center gap-3">
                    <div className="w-10 h-10 rounded-xl bg-blue-100 text-blue-700 flex items-center justify-center font-bold text-sm shrink-0">
                      {client.name.substring(0, 2).toUpperCase()}
                    </div>
                    <div>
                      <h3 className="font-bold text-slate-900 text-sm">{client.name}</h3>
                      <div className="flex items-center gap-2 text-xs text-slate-500 mt-0.5">
                        <span className="font-medium text-slate-600">{client.phone}</span>
                        {client.neighborhood && (
                          <>
                            <span>•</span>
                            <span className="flex items-center gap-1">
                              <MapPin className="w-3 h-3 text-slate-400" />
                              {client.neighborhood}
                            </span>
                          </>
                        )}
                      </div>
                    </div>
                  </div>

                  <div className="flex items-center gap-3">
                    <span className="text-xs font-semibold px-2.5 py-1 bg-sky-50 text-sky-800 border border-sky-200 rounded-lg">
                      {client.appliances.length} {client.appliances.length === 1 ? 'aparelho' : 'aparelhos'}
                    </span>
                    {isExpanded ? (
                      <ChevronUp className="w-4 h-4 text-slate-400" />
                    ) : (
                      <ChevronDown className="w-4 h-4 text-slate-400" />
                    )}
                  </div>
                </div>

                {/* Expanded Details */}
                {isExpanded && (
                  <div className="p-4 bg-slate-50/70 border-t border-slate-100 space-y-3 text-xs">
                    {/* Address & Notes */}
                    {client.address && (
                      <p className="text-slate-600 flex items-center gap-1.5">
                        <MapPin className="w-3.5 h-3.5 text-slate-400 shrink-0" />
                        <span>{client.address} {client.city ? `- ${client.city}` : ''}</span>
                      </p>
                    )}

                    {client.notes && (
                      <p className="text-slate-500 italic bg-white p-2.5 rounded-lg border border-slate-200">
                        {client.notes}
                      </p>
                    )}

                    {/* Appliances of this client */}
                    <div>
                      <div className="flex items-center justify-between mb-2">
                        <h4 className="font-bold text-slate-700 uppercase tracking-wider text-[11px] flex items-center gap-1">
                          <Wrench className="w-3.5 h-3.5 text-sky-600" />
                          Aparelhos Cadastrados
                        </h4>
                        <button
                          onClick={() => onAddApplianceToClient(client)}
                          className="text-[11px] font-bold text-sky-700 hover:text-sky-800 flex items-center gap-1"
                        >
                          <Plus className="w-3 h-3" />
                          <span>Adicionar Aparelho</span>
                        </button>
                      </div>

                      <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
                        {client.appliances.map(app => (
                          <div
                            key={app.id}
                            className="bg-white p-3 rounded-xl border border-slate-200 flex flex-col justify-between gap-2 shadow-xs"
                          >
                            <div>
                              <div className="flex items-center justify-between">
                                <span className="font-bold text-slate-800">
                                  {app.brand} {app.capacityBtu} BTUs
                                </span>
                                <span className="text-[10px] font-semibold px-2 py-0.5 bg-slate-100 text-slate-700 rounded">
                                  {app.room}
                                </span>
                              </div>
                              <p className="text-[11px] text-slate-500 mt-1">
                                {app.type} • {app.gasType || 'R-410A'} • {app.voltage || '220V'}
                              </p>
                            </div>

                            <div className="flex gap-2 pt-1 border-t border-slate-100">
                              <button
                                onClick={() => onOpenFicha(client, app)}
                                className="flex-1 py-1.5 px-2 bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-lg text-[11px] font-medium flex items-center justify-center gap-1"
                              >
                                <Wrench className="w-3 h-3 text-sky-600" />
                                <span>Ver Ficha</span>
                              </button>

                              <button
                                onClick={() => onOpenChecklist(client, app)}
                                className="flex-1 py-1.5 px-2 bg-sky-600 hover:bg-sky-500 text-white rounded-lg text-[11px] font-bold flex items-center justify-center gap-1 shadow-xs"
                              >
                                <Wind className="w-3 h-3" />
                                <span>Ordem de Serviço</span>
                              </button>

                              <button
                                onClick={(e) => {
                                  e.stopPropagation();
                                  if (confirm(`Excluir aparelho ${app.brand} ${app.capacityBtu} BTUs (${app.room})?`)) {
                                    onDeleteAppliance(client.id, app.id);
                                  }
                                }}
                                title="Excluir aparelho"
                                className="p-1.5 text-slate-400 hover:text-red-600 rounded-lg hover:bg-red-50 transition-colors"
                              >
                                <Trash2 className="w-3.5 h-3.5" />
                              </button>
                            </div>
                          </div>
                        ))}
                      </div>
                    </div>

                    {/* Quick WhatsApp & Call Bar */}
                    <div className="flex flex-wrap gap-2 pt-2 items-center">
                      <button
                        onClick={() => {
                          const url = `https://api.whatsapp.com/send?phone=${WhatsAppService.cleanPhone(client.phone)}`;
                          window.open(url, '_blank');
                        }}
                        className="flex-auto min-w-[8rem] py-2 px-3 bg-emerald-600 hover:bg-emerald-500 text-white rounded-xl font-bold flex items-center justify-center gap-1.5 shadow-sm"
                      >
                        <MessageSquare className="w-4 h-4" />
                        <span>WhatsApp</span>
                      </button>

                      <button
                        onClick={(e) => { e.stopPropagation(); onEditClient(client); }}
                        title="Editar cliente"
                        className="flex-auto min-w-[6.5rem] py-2 px-4 bg-sky-50 hover:bg-sky-100 text-sky-700 rounded-xl font-semibold flex items-center justify-center gap-1.5 border border-sky-100 transition-colors"
                      >
                        <UserCog className="w-4 h-4" />
                        <span>Editar</span>
                      </button>

                      <button
                        onClick={(e) => { e.stopPropagation(); resetClientPassword(client); }}
                        disabled={resettingClientId === client.id}
                        title="Criar senha temporária e exigir alteração no próximo acesso"
                        className="flex-auto min-w-[9.5rem] py-2 px-4 bg-violet-50 hover:bg-violet-100 text-violet-700 rounded-xl font-semibold flex items-center justify-center gap-1.5 border border-violet-200 transition-colors disabled:opacity-50"
                      >
                        <KeyRound className="w-4 h-4" />
                        <span>{resettingClientId === client.id ? 'Gerando...' : 'Redefinir senha'}</span>
                      </button>

                      <button
                        onClick={() => window.location.href = `tel:${client.phone}`}
                        className="flex-auto min-w-[6rem] py-2 px-4 bg-slate-800 hover:bg-slate-700 text-white rounded-xl font-semibold flex items-center justify-center gap-1.5"
                      >
                        <Phone className="w-4 h-4" />
                        <span>Ligar</span>
                      </button>

                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          if (confirm(`Deseja realmente excluir o cliente "${client.name}" e todos os seus aparelhos?`)) {
                            onDeleteClient(client.id);
                          }
                        }}
                        title="Excluir cliente"
                        className="p-2 text-slate-400 hover:text-red-600 rounded-xl hover:bg-red-50 transition-colors"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </div>

                    {/* Orçamentos e pagamentos do cliente — abaixo da barra de ações */}
                    {(() => {
                      const meus = budgets.filter((b) => b.clientId === client.id);
                      const recebido = meus.filter((b) => b.pago).reduce((s, b) => s + (b.finalValue || 0), 0);
                      const aberto = meus.filter((b) => !b.pago && b.status !== 'recusado').reduce((s, b) => s + (b.finalValue || 0), 0);
                      return (
                        <div className="mt-3 pt-3 border-t border-slate-200">
                          <span className="text-[10px] font-extrabold text-slate-500 uppercase tracking-wide">Orçamentos &amp; Pagamentos</span>
                          {meus.length === 0 ? (
                            <p className="text-[11px] text-slate-400 mt-1">Nenhum orçamento para este cliente ainda.</p>
                          ) : (
                            <div className="mt-1.5 space-y-1.5">
                              {meus.map((b) => (
                                <div key={b.id} className="flex items-center justify-between text-[11px] bg-slate-50 rounded-lg px-2.5 py-1.5">
                                  <span className={`font-semibold ${b.pago ? 'text-emerald-700' : 'text-slate-600'}`}>
                                    {b.pago ? '✅' : '⏳'} {b.applianceDesc ? b.applianceDesc.slice(0, 34) : 'Serviço'}
                                  </span>
                                  <span className={`font-bold ${b.pago ? 'text-emerald-700' : 'text-amber-600'}`}>
                                    R$ {b.finalValue.toFixed(2)}
                                  </span>
                                </div>
                              ))}
                              <div className="flex justify-between text-[11px] font-bold pt-1">
                                <span className="text-emerald-700">Recebido: R$ {recebido.toFixed(2)}</span>
                                <span className="text-amber-600">A receber: R$ {aberto.toFixed(2)}</span>
                              </div>
                            </div>
                          )}
                        </div>
                      );
                    })()}
                  </div>
                )}
              </div>
            );
          })
        )}
      </div>
      {accessError && <div role="alert" className="fixed left-1/2 bottom-24 z-[80] -translate-x-1/2 max-w-[92vw] rounded-xl bg-red-700 px-4 py-3 text-sm font-semibold text-white shadow-xl">{accessError}</div>}
      {temporaryAccess && (
        <div className="fixed inset-0 z-[90] grid place-items-center bg-slate-950/70 p-4" role="dialog" aria-modal="true" aria-labelledby="temporary-password-title">
          <div className="w-full max-w-md rounded-2xl bg-white p-5 text-slate-900 shadow-2xl">
            <h2 id="temporary-password-title" className="text-lg font-bold">Senha temporária criada</h2>
            <p className="mt-1 text-sm text-slate-600">O cliente deverá criar uma nova senha pessoal no próximo acesso.</p>
            <div className="mt-4 rounded-xl border border-slate-200 bg-slate-50 p-3">
              <p className="text-xs text-slate-500">Login</p><p className="break-all font-semibold">{temporaryAccess.email}</p>
              <p className="mt-3 text-xs text-slate-500">Senha temporária</p><p className="font-mono text-lg font-bold tracking-wide">{temporaryAccess.senha}</p>
            </div>
            <div className="mt-4 grid grid-cols-2 gap-2">
              <button onClick={async () => { await navigator.clipboard.writeText(`Login: ${temporaryAccess.email}\nSenha temporária: ${temporaryAccess.senha}`); setCopied(true); }} className="min-h-11 rounded-xl border border-slate-300 font-bold flex items-center justify-center gap-2">{copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}{copied ? 'Copiado' : 'Copiar acesso'}</button>
              <button onClick={() => { setTemporaryAccess(null); setCopied(false); }} className="min-h-11 rounded-xl bg-sky-600 font-bold text-white">Concluir</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
