import React, { useState, useEffect, useRef } from 'react';
import { Client, Appliance, BudgetItem, BudgetEstimate, TechnicianProfile } from '../types';
import {
  X,
  Plus,
  Trash2,
  DollarSign,
  FileText,
  Calendar,
  Clock,
  ShieldCheck,
  Zap,
  Sparkles,
  Flame,
  Wrench,
  CheckCircle2,
  ChevronDown
} from 'lucide-react';
import { addDays, differenceInCalendarDays, format, parseISO } from 'date-fns';
import { montarCatalogo, TipoCatalogo } from '../services/catalogo';

interface NovoOrcamentoModalProps {
  clients: Client[];
  profile: TechnicianProfile;
  catalogo?: TipoCatalogo[];
  initialClient?: Client;
  editing?: BudgetEstimate;
  onClose: () => void;
  onSave: (budget: BudgetEstimate) => void;
}

// Itens rápidos granulares (peças/materiais). Os TIPOS DE SERVIÇO já vêm do
// catálogo editável com os preços do admin — não duplicar aqui.
const ITENS_RAPIDOS: { desc: string; price: number; category: 'servico' | 'peca' | 'material' }[] = [
  { desc: 'Substituição de Capacitor de Partida + Teste Operacional', price: 260, category: 'peca' },
  { desc: 'Kit Tubulação de Cobre com Isolamento Térmico (por metro)', price: 85, category: 'material' },
  { desc: 'Suporte de Condensadora Externa Reforçado com Coxins', price: 120, category: 'peca' },
  { desc: 'Desobstrução de Dreno e Limpeza de Bandeja', price: 170, category: 'servico' }
];

export const NovoOrcamentoModal: React.FC<NovoOrcamentoModalProps> = ({
  clients,
  profile,
  catalogo,
  initialClient,
  editing,
  onClose,
  onSave
}) => {
  const [selectedClientId, setSelectedClientId] = useState<string>(initialClient?.id || (clients[0]?.id || ''));
  const [customClientName, setCustomClientName] = useState(initialClient?.name || '');
  const [customClientPhone, setCustomClientPhone] = useState(initialClient?.phone || '');
  const [applianceDesc, setApplianceDesc] = useState('');
  const [equipmentName, setEquipmentName] = useState('');
  const [selectedApplianceId, setSelectedApplianceId] = useState('');

  const [validityDays, setValidityDays] = useState(7);
  const [executionTime, setExecutionTime] = useState('2 a 3 horas');
  const [paymentConditions, setPaymentConditions] = useState('À vista no PIX ou cartão a consultar o valor à parte com taxa');
  const [warrantyTerms, setWarrantyTerms] = useState('90 dias para mão de obra / 1 ano para instalação');
  const [notes, setNotes] = useState('');
  const [discount, setDiscount] = useState<number>(0);

  // Items list
  const [items, setItems] = useState<BudgetItem[]>([
    {
      id: '1',
      description: 'Limpeza de Ar c/ Bactericida Hospitalar',
      quantity: 1,
      unitPrice: 250,
      totalPrice: 250,
      category: 'servico'
    }
  ]);

  const selectedClient = clients.find((c) => c.id === selectedClientId);

  // Pre-seleciona o primeiro aparelho do cliente ao abrir (evita bloqueio indevido).
  // Guard por cliente: o array `clients` é recriado a cada sync (60s) — sem o ref,
  // este efeito reverteria o que o técnico digitou com o modal aberto.
  const prevClientIdRef = useRef<string | null>(null);
  useEffect(() => {
    if (prevClientIdRef.current === selectedClientId) return;
    prevClientIdRef.current = selectedClientId;
    const cl = clients.find((x) => x.id === selectedClientId);
    if (cl && cl.appliances && cl.appliances.length > 0) {
      setSelectedApplianceId(cl.appliances[0].id);
      const app = cl.appliances[0];
      setEquipmentName(`${app.brand} ${app.model || ''}`.trim());
      setApplianceDesc(`${app.brand} ${app.model || ''} ${app.capacityBtu} BTUs (${app.room})`.trim());
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedClientId, clients]);

  // MODO EDIÇÃO: pré-carrega o orçamento existente — edita o atual em vez de gerar outro
  useEffect(() => {
    if (!editing) return;
    if (editing.clientId && clients.some((c) => c.id === editing.clientId)) setSelectedClientId(editing.clientId);
    else setSelectedClientId(''); // avulso/cliente excluído: NUNCA reatribuir a clients[0]
    prevClientIdRef.current = editing.clientId && clients.some((c) => c.id === editing.clientId) ? editing.clientId : '';
    setCustomClientName(editing.clientName || '');
    setCustomClientPhone(editing.clientPhone || '');
    setApplianceDesc(editing.applianceDesc || '');
    setEquipmentName(editing.equipmentName || '');
    setSelectedApplianceId(editing.applianceId || '');
    setExecutionTime(editing.executionTime || '2 a 3 horas');
    setPaymentConditions(editing.paymentConditions || '');
    setWarrantyTerms(editing.warrantyTerms || '');
    setNotes(editing.notes || '');
    setDiscount(Number(editing.discount) || 0);
    if (editing.validUntil) {
      try {
        const dias = differenceInCalendarDays(parseISO(editing.validUntil), new Date());
        setValidityDays(dias > 0 ? Math.min(dias, 7) : 1);
      } catch { /* mantém padrão */ }
    }
    if (editing.items?.length) setItems(editing.items.map((it) => ({ ...it })));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleClientChange = (clientId: string) => {
    setSelectedClientId(clientId);
    const c = clients.find((cl) => cl.id === clientId);
    if (c) {
      setCustomClientName(c.name);
      setCustomClientPhone(c.phone);
      if (c.appliances && c.appliances.length > 0) {
        const app = c.appliances[0];
        setEquipmentName(`${app.brand}${app.model ? ` ${app.model}` : ''}`.trim());
        setApplianceDesc(`${app.brand} ${app.capacityBtu} BTUs (${app.room})`);
      }
    }
  };

  const handleAddItem = (desc: string, price: number, cat: 'servico' | 'peca' | 'material') => {
    const newItem: BudgetItem = {
      id: Math.random().toString(36).substring(2, 9),
      description: desc,
      quantity: 1,
      unitPrice: price,
      totalPrice: price,
      category: cat
    };
    setItems([...items, newItem]);
  };

  const handleUpdateItem = (id: string, field: keyof BudgetItem, val: any) => {
    setItems(
      items.map((it) => {
        if (it.id === id) {
          const updated = { ...it, [field]: val };
          if (field === 'quantity' || field === 'unitPrice') {
            updated.totalPrice = Number(updated.quantity || 0) * Number(updated.unitPrice || 0);
          }
          return updated;
        }
        return it;
      })
    );
  };

  const handleRemoveItem = (id: string) => {
    setItems(items.filter((it) => it.id !== id));
  };

  const subtotal = items.reduce((acc, it) => acc + (it.totalPrice || 0), 0);
  const finalValue = Math.max(0, subtotal - (Number(discount) || 0));

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (items.length === 0) {
      alert('Adicione pelo menos um item ao orçamento.');
      return;
    }

    const today = new Date();
    const validUntilDate = addDays(today, validityDays);

    if (selectedClient && selectedClient.appliances && selectedClient.appliances.length > 0 && !selectedApplianceId) {
      alert('Selecione o aparelho do cliente para vincular ao orçamento.');
      return;
    }
    if (selectedClient && (!selectedClient.appliances || selectedClient.appliances.length === 0) && !applianceDesc.trim()) {
      alert('Cadastre um aparelho para este cliente antes de gerar o orçamento — o equipamento é obrigatório no fluxo.');
      return;
    }

    const newBudget: BudgetEstimate = {
      id: editing ? editing.id : 'orc_' + Math.random().toString(36).substring(2, 8),
      numero: editing?.numero,
      clientId: selectedClientId || 'avulso',
      clientName: customClientName || selectedClient?.name || 'Cliente Inovar',
      clientPhone: customClientPhone || selectedClient?.phone || '',
      clientAddress: selectedClient ? [selectedClient.address, selectedClient.neighborhood, selectedClient.city].filter(Boolean).join(', ') : '',
      clientDocument: selectedClient?.document || editing?.clientDocument || '',
      equipmentName: equipmentName.trim() || applianceDesc.split(' BTUs')[0].trim(),
      applianceDesc: applianceDesc,
      date: editing?.date || format(today, 'yyyy-MM-dd'),
      validUntil: format(validUntilDate, 'yyyy-MM-dd'),
      items: items,
      totalValue: subtotal,
      discount: Number(discount) || 0,
      finalValue: finalValue,
      paymentConditions: paymentConditions,
      executionTime: executionTime,
      warrantyTerms: warrantyTerms,
      status: editing ? editing.status : 'pendente',
      notes: notes,
      assinatura: editing?.assinatura ?? null,
      assinaturaEm: editing?.assinaturaEm ?? null,
      pago: editing?.pago || false,
      pagoEm: editing?.pagoEm ?? null,
      valorRecebido: editing?.valorRecebido ?? null,
      applianceId: selectedApplianceId || null
    };

    onSave(newBudget);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-4 bg-black/85 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="bg-slate-900 border border-slate-700/80 rounded-2xl w-full max-w-2xl max-h-[92vh] flex flex-col shadow-2xl text-slate-100 overflow-hidden">
        {/* Header */}
        <div className="p-4 bg-gradient-to-r from-inovar-navy via-slate-900 to-slate-950 border-b border-slate-800 flex items-center justify-between shrink-0">
          <div className="flex items-center gap-2.5">
            <div className="p-2 bg-inovar-yellow/20 text-inovar-yellow rounded-xl border border-inovar-yellow/30">
              <FileText className="w-5 h-5" />
            </div>
            <div>
              <span className="text-[10px] font-bold text-inovar-yellow uppercase tracking-wider block">
                INOVAR REFRIGERAÇÃO • PROPOSTA COMERCIAL
              </span>
              <h3 className="text-base font-bold text-white">
                {editing ? `Editar Orçamento ${editing.numero || 'existente'}` : 'Criar Novo Orçamento Técnico'}
              </h3>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 text-slate-400 hover:text-white rounded-lg hover:bg-slate-800 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Content Form */}
        <form onSubmit={handleSubmit} className="p-4 sm:p-5 overflow-y-auto space-y-4 flex-1 text-xs">
          {/* Cliente e Aparelho */}
          <div className="bg-slate-800/80 p-3.5 rounded-xl border border-slate-700/80 space-y-3">
            <span className="font-bold text-slate-200 block text-[11px] uppercase tracking-wider">
              Dados do Cliente
            </span>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
              <div>
                <label className="block text-slate-400 mb-1">Selecionar Cliente Cadastrado:</label>
                <select
                  value={selectedClientId}
                  onChange={(e) => handleClientChange(e.target.value)}
                  className="w-full p-2 bg-slate-900 border border-slate-700 rounded-lg text-white font-medium"
                >
                  <option value="">Cliente Avulso / Novo</option>
                  {clients.map((c) => (
                    <option key={c.id} value={c.id}>
                      {c.name} ({c.neighborhood || c.city || 'ES'})
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-slate-400 mb-1">Nome Completo do Cliente:</label>
                <input
                  type="text"
                  required
                  value={customClientName}
                  onChange={(e) => setCustomClientName(e.target.value)}
                  placeholder="Nome do cliente"
                  className="w-full p-2 bg-slate-900 border border-slate-700 rounded-lg text-white"
                />
              </div>

              <div>
                <label className="block text-slate-400 mb-1">WhatsApp / Telefone:</label>
                <input
                  type="text"
                  required
                  value={customClientPhone}
                  onChange={(e) => setCustomClientPhone(e.target.value)}
                  placeholder="(27) 99999-9999"
                  className="w-full p-2 bg-slate-900 border border-slate-700 rounded-lg text-white"
                />
              </div>

              <div>
                <label className="block text-slate-400 mb-1">Nome do equipamento:</label>
                <input type="text" value={equipmentName} onChange={(e) => setEquipmentName(e.target.value)} placeholder="Ex: LG Dual Inverter" className="w-full p-2 bg-slate-900 border border-slate-700 rounded-lg text-white" />
              </div>

              <div>
                <label className="block text-slate-400 mb-1">Equipamento (obrigatório — aparelho do cliente):</label>
                {selectedClient && selectedClient.appliances && selectedClient.appliances.length > 0 ? (
                  <select
                    value={selectedApplianceId}
                    onChange={(e) => {
                      setSelectedApplianceId(e.target.value);
                      const ap = selectedClient.appliances.find((a) => a.id === e.target.value);
                      if (ap) { setEquipmentName(`${ap.brand} ${ap.model || ''}`.trim()); setApplianceDesc(`${ap.brand} ${ap.model || ''} ${ap.capacityBtu} BTUs (${ap.room})`.trim()); }
                    }}
                    className="w-full p-2 bg-slate-900 border border-slate-700 rounded-lg text-white font-medium"
                  >
                    {selectedClient.appliances.map((a) => (
                      <option key={a.id} value={a.id}>{a.brand} {a.model || ''} {a.capacityBtu} BTUs ({a.room})</option>
                    ))}
                  </select>
                ) : (
                  <input
                    type="text"
                    value={applianceDesc}
                    onChange={(e) => setApplianceDesc(e.target.value)}
                    placeholder="Ex: LG Dual Inverter 12.000 BTUs (Sala)"
                    className="w-full p-2 bg-slate-900 border border-slate-700 rounded-lg text-white"
                  />
                )}
                {(!selectedClient?.appliances || selectedClient.appliances.length === 0) && (
                  <p className="text-[10px] text-amber-400 mt-1">⚠️ Este cliente não tem aparelhos cadastrados. Cadastre o aparelho dele para vincular ao serviço.</p>
                )}
              </div>
            </div>
          </div>

          {/* Atalhos de Itens Rápidos */}
          <div>
            <div className="flex items-center justify-between mb-1.5">
              <span className="font-bold text-slate-300 text-[11px] uppercase tracking-wider">
                Inserir Itens Rápidos com 1 Clique:
              </span>
              <span className="text-[10px] text-sky-400 font-semibold">Tabela Padrão Inovar</span>
            </div>

            <div className="flex flex-wrap gap-1.5">
              {(catalogo && catalogo.length > 0 ? catalogo : montarCatalogo()).filter((t) => t.preco > 0).map((t, idx) => (
                <button
                  type="button"
                  key={'custom_' + idx}
                  onClick={() => { handleAddItem(t.nome, t.preco, 'servico'); if (t.tempoMedio) setExecutionTime(t.tempoMedio); if (t.garantiaPadrao) setWarrantyTerms(t.garantiaPadrao); }}
                  className="px-2.5 py-1 bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 hover:border-emerald-500 rounded-lg text-[11px] flex items-center gap-1 transition-all active:scale-95"
                >
                  <Plus className="w-3 h-3 text-emerald-400" />
                  <span>{t.nome}</span>
                  <span className="font-bold text-emerald-400 ml-1">R$ {t.preco}</span>
                </button>
              ))}
              {ITENS_RAPIDOS.map((it, idx) => (
                <button
                  type="button"
                  key={idx}
                  onClick={() => handleAddItem(it.desc, it.price, it.category)}
                  className="px-2.5 py-1 bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 hover:border-sky-500 rounded-lg text-[11px] flex items-center gap-1 transition-all active:scale-95"
                >
                  <Plus className="w-3 h-3 text-sky-400" />
                  <span>{it.desc.split(' ')[0]} {it.desc.split(' ')[1]}</span>
                  <span className="font-bold text-emerald-400 ml-1">R$ {it.price}</span>
                </button>
              ))}
            </div>
          </div>

          {/* Lista de Itens do Orçamento */}
          <div className="bg-slate-800/80 p-3.5 rounded-xl border border-slate-700/80 space-y-2.5">
            <div className="flex items-center justify-between">
              <span className="font-bold text-slate-200 text-[11px] uppercase tracking-wider">
                Itens da Proposta ({items.length})
              </span>
              <button
                type="button"
                onClick={() => handleAddItem('Novo Serviço / Material', 100, 'servico')}
                className="text-xs text-sky-400 hover:text-sky-300 font-semibold flex items-center gap-1"
              >
                <Plus className="w-3.5 h-3.5" />
                <span>Adicionar Linha</span>
              </button>
            </div>

            <div className="space-y-2">
              {items.map((it, index) => (
                <div
                  key={it.id}
                  className="p-2.5 bg-slate-900/90 rounded-lg border border-slate-700/80 flex flex-col sm:flex-row items-start sm:items-center gap-2"
                >
                  <div className="flex-1 w-full">
                    <input
                      type="text"
                      required
                      value={it.description}
                      onChange={(e) => handleUpdateItem(it.id, 'description', e.target.value)}
                      placeholder="Descrição do serviço ou material"
                      className="w-full p-1.5 bg-slate-800 border border-slate-700 rounded text-xs text-white"
                    />
                  </div>

                  <div className="flex items-center gap-2 w-full sm:w-auto justify-between sm:justify-end">
                    <div className="flex items-center gap-1">
                      <span className="text-[10px] text-slate-400">Qtd:</span>
                      <input
                        type="number"
                        min="1"
                        value={it.quantity}
                        onChange={(e) => handleUpdateItem(it.id, 'quantity', parseInt(e.target.value) || 1)}
                        className="w-12 p-1.5 bg-slate-800 border border-slate-700 rounded text-xs text-center text-white"
                      />
                    </div>

                    <div className="flex items-center gap-1">
                      <span className="text-[10px] text-slate-400">R$:</span>
                      <input
                        type="number"
                        min="0"
                        step="5"
                        value={it.unitPrice}
                        onChange={(e) => handleUpdateItem(it.id, 'unitPrice', parseFloat(e.target.value) || 0)}
                        className="w-20 p-1.5 bg-slate-800 border border-slate-700 rounded text-xs text-right text-white font-semibold"
                      />
                    </div>

                    <span className="font-bold text-emerald-400 text-xs min-w-[70px] text-right">
                      R$ {it.totalPrice.toFixed(2)}
                    </span>

                    <button
                      type="button"
                      onClick={() => handleRemoveItem(it.id)}
                      className="p-1 text-slate-400 hover:text-red-400 rounded transition-colors"
                      title="Remover item"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
              ))}
            </div>

            {/* Totais & Desconto */}
            <div className="pt-2 border-t border-slate-700/80 flex flex-col sm:flex-row items-end justify-between gap-2">
              <div className="flex items-center gap-2 w-full sm:w-auto">
                <span className="text-slate-400 text-[11px]">Desconto Especial (R$):</span>
                <input
                  type="number"
                  min="0"
                  value={discount}
                  onChange={(e) => setDiscount(parseFloat(e.target.value) || 0)}
                  placeholder="0.00"
                  className="w-24 p-1.5 bg-slate-900 border border-slate-700 rounded text-xs text-right text-red-400 font-bold"
                />
              </div>

              <div className="text-right">
                <div className="text-[11px] text-slate-400">
                  Subtotal: <span className="text-slate-200 font-semibold">R$ {subtotal.toFixed(2)}</span>
                </div>
                <div className="text-base font-extrabold text-emerald-400">
                  Total da Proposta: R$ {finalValue.toFixed(2)}
                </div>
              </div>
            </div>
          </div>

          {/* Condições & Prazos */}
          <div className="bg-slate-800/80 p-3.5 rounded-xl border border-slate-700/80 grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="block text-slate-400 mb-1">Forma / Condições de Pagamento:</label>
              <input
                type="text"
                value={paymentConditions}
                onChange={(e) => setPaymentConditions(e.target.value)}
                className="w-full p-2 bg-slate-900 border border-slate-700 rounded-lg text-white"
              />
            </div>

            <div>
              <label className="block text-slate-400 mb-1">Prazo de Execução:</label>
              <input
                type="text"
                value={executionTime}
                onChange={(e) => setExecutionTime(e.target.value)}
                className="w-full p-2 bg-slate-900 border border-slate-700 rounded-lg text-white"
              />
            </div>

            <div>
              <label className="block text-slate-400 mb-1">Termo de Garantia Oferecido:</label>
              <input
                type="text"
                value={warrantyTerms}
                onChange={(e) => setWarrantyTerms(e.target.value)}
                className="w-full p-2 bg-slate-900 border border-slate-700 rounded-lg text-white"
              />
            </div>

            <div>
              <label className="block text-slate-400 mb-1">Validade da Proposta:</label>
              <select
                value={validityDays}
                onChange={(e) => setValidityDays(parseInt(e.target.value))}
                className="w-full p-2 bg-slate-900 border border-slate-700 rounded-lg text-white"
              >
                <option value={7}>7 dias corridos (Padrão)</option>
              </select>
            </div>

            <div className="sm:col-span-2">
              <label className="block text-slate-400 mb-1">Observações Técnicas / Detalhes Adicionais:</label>
              <textarea
                rows={2}
                value={notes}
                onChange={(e) => setNotes(e.target.value)}
                placeholder="Ex: Ponto elétrico de 220V já existente. Material de isolamento e cabos inclusos."
                className="w-full p-2 bg-slate-900 border border-slate-700 rounded-lg text-white"
              />
            </div>
          </div>

          {/* Submit */}
          <div className="flex gap-2 pt-1 shrink-0">
            <button
              type="button"
              onClick={onClose}
              className="py-3 px-4 bg-slate-800 hover:bg-slate-700 text-slate-300 font-semibold rounded-xl text-xs"
            >
              Cancelar
            </button>
            <button
              type="submit"
              className="flex-1 py-3 px-4 bg-inovar-yellow hover:brightness-105 active:scale-[0.99] text-inovar-navy font-bold rounded-xl text-sm transition-all shadow-lg flex items-center justify-center gap-2"
            >
              <CheckCircle2 className="w-4 h-4" />
              <span>Gerar e Salvar Orçamento</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
