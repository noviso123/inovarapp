import React, { useState } from 'react';
import { Client, Appliance, MaintenanceRecord, ChecklistData, TechnicianProfile, ServiceType } from '../types';
import {
  X,
  CheckSquare,
  Square,
  Sparkles,
  Wrench,
  Layers,
  Flame
} from 'lucide-react';
import { addMonths, format } from 'date-fns';
import confetti from 'canvas-confetti';
import { montarCatalogo, TipoCatalogo } from '../services/catalogo';

interface ChecklistLimpezaModalProps {
  client: Client;
  appliance: Appliance;
  profile: TechnicianProfile;
  initialServiceType?: ServiceType;
  catalogo?: TipoCatalogo[];
  onClose: () => void;
  onSaveMaintenance: (record: MaintenanceRecord) => void | Promise<void>;
}

export const ChecklistLimpezaModal: React.FC<ChecklistLimpezaModalProps> = ({
  client,
  appliance,
  profile,
  initialServiceType = 'Limpeza de Ar',
  catalogo,
  onClose,
  onSaveMaintenance
}) => {
  const [serviceType, setServiceType] = useState<ServiceType>(initialServiceType);

  // Catálogo resolvido (fixos editáveis/removíveis + personalizados)
  const CATOLOGO_LISTA = catalogo && catalogo.length > 0 ? catalogo : montarCatalogo();
  // Tipo base: mantém o checklist especializado mesmo se o tipo fixo foi renomeado
  const tipoBase = CATOLOGO_LISTA.find((c) => c.nome === serviceType || c.fixo === serviceType)?.fixo ?? serviceType;
  
  // Checklist states
  const [checklist, setChecklist] = useState<ChecklistData>({
    // Limpeza de ar
    filtrosLavados: true,
    serpentinaHigienizada: true,
    turbinaLimpa: true,
    drenoDesobstruido: true,
    bandejaSanitizada: true,
    condensadoraLavada: true,
    aplicacaoBactericida: true,
    testeEletricoCorrente: true,
    correnteAmperes: '',
    pressaoGasPSI: '',
    saltoTermicoDeltaT: '',
    temperaturaRetorno: '',
    temperaturaInsuflamento: '',
    
    // Instalação
    suporteNivelado: true,
    vacuoMicrons: '',
    testeNitrogenio: true,
    valvulasLiberadas: true,

    // Corretiva & Gás
    capacitorTestado: '',
    gasAdicionadoGramas: '',
    pecasSubstituidas: '',
    diagnosticoTecnico: ''
  });

  // Financial states
  const [laborPrice, setLaborPrice] = useState<number>(profile.defaultPrice || 250);
  const [partsPrice, setPartsPrice] = useState<number>(0);
  const [partsUsed, setPartsUsed] = useState<string>('');
  const [paymentMethod, setPaymentMethod] = useState<'PIX' | 'Cartão Crédito' | 'Cartão Débito' | 'Dinheiro' | 'A Faturar'>('PIX');
  const [cycleMonths, setCycleMonths] = useState(profile.defaultReturnMonths || 6);
  const garantiaDoTipo = (tipo: ServiceType) => {
    const item = CATOLOGO_LISTA.find((c) => c.nome === tipo || c.fixo === tipo);
    const match = String(item?.garantiaPadrao || '').match(/\d+/);
    return match ? Number(match[0]) : (profile.defaultWarrantyDays || 90);
  };
  const [warrantyDays, setWarrantyDays] = useState(garantiaDoTipo(initialServiceType));
  const [notes, setNotes] = useState('');

  const totalPrice = Number(laborPrice || 0) + Number(partsPrice || 0);

  const toggleCheck = (key: keyof ChecklistData) => {
    setChecklist((prev) => ({
      ...prev,
      [key]: !prev[key]
    }));
  };

  const [saving, setSaving] = useState(false);
  const handleFinish = async (e: React.FormEvent) => {
    e.preventDefault();
    if (saving) return;
    setSaving(true);
    const today = new Date();
    const returnDate = format(addMonths(today, cycleMonths), 'yyyy-MM-dd');

    const newRecord: MaintenanceRecord = {
      id: 'os_' + Math.random().toString(36).substring(2, 9),
      clientId: client.id,
      applianceId: appliance.id,
      date: format(today, 'yyyy-MM-dd'),
      returnDate: returnDate,
      serviceType: serviceType,
      price: totalPrice,
      laborPrice: laborPrice,
      partsPrice: partsPrice,
      partsUsed: partsUsed || undefined,
      paymentMethod: paymentMethod,
      warrantyDays: warrantyDays,
      notes: notes,
      checklist: checklist,
      status: 'concluido'
    };

    try {
      confetti({
        particleCount: 80,
        spread: 70,
        origin: { y: 0.6 }
      });
    } catch (err) {}

    try { await onSaveMaintenance(newRecord); } finally { setSaving(false); }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-2 sm:p-4 bg-black/80 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="bg-slate-900 border border-slate-700/80 rounded-2xl w-full max-w-xl max-h-[92vh] flex flex-col shadow-2xl text-slate-100 overflow-hidden">
        {/* Header */}
        <div className="p-3.5 sm:p-4 bg-gradient-to-r from-inovar-navy via-slate-900 to-slate-950 border-b border-slate-800 flex items-center justify-between shrink-0">
          <div className="flex items-center gap-2">
            <div className="p-2 bg-inovar-yellow/20 text-inovar-yellow rounded-xl border border-inovar-yellow/30">
              <Wrench className="w-5 h-5" />
            </div>
            <div>
              <h2 className="text-sm sm:text-base font-extrabold text-white flex items-center gap-1.5">
                <span>Checklist e Ordem de Serviço</span>
                <span className="px-2 py-0.5 rounded-full bg-inovar-yellow/20 text-inovar-yellow text-[10px] uppercase font-bold">
                  {serviceType}
                </span>
              </h2>
              <p className="text-[11px] text-slate-300">
                {client.name} • {appliance.brand} {appliance.capacityBtu} ({appliance.room})
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 text-slate-400 hover:text-white rounded-lg hover:bg-slate-800 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Scrollable Form Body */}
        <form onSubmit={handleFinish} className="overflow-y-auto p-3.5 sm:p-5 space-y-4 flex-1">
          {/* Service Selector Tabs */}
          <div>
            <label className="block text-xs font-bold text-slate-300 mb-1.5 uppercase tracking-wide">
              Tipo de Atendimento Técnico
            </label>
            <div className="grid grid-cols-2 sm:grid-cols-3 gap-1.5 text-xs">
              {CATOLOGO_LISTA.map((t) => {
                const IconComponent = t.card?.icon || Wrench;
                const selecionado = t.nome === serviceType || t.fixo === serviceType;
                return (
                  <button
                    type="button"
                    key={t.key}
                    onClick={() => {
                      const tipo = (t.fixo || t.nome) as ServiceType;
                      setServiceType(tipo);
                      setWarrantyDays(garantiaDoTipo(tipo));
                    }}
                    className={`p-2 rounded-xl font-bold flex items-center gap-1.5 transition-all text-[11px] ${
                      selecionado
                        ? 'bg-inovar-yellow text-inovar-navy shadow-md'
                        : t.card
                        ? 'bg-slate-800/80 text-slate-300 hover:bg-slate-700/80 border border-slate-700'
                        : 'bg-slate-800/80 text-slate-300 hover:bg-slate-700/80 border border-sky-500/40'
                    }`}
                    title={t.card ? t.nome : 'Tipo de serviço personalizado do catálogo'}
                  >
                    <IconComponent className="w-3.5 h-3.5 shrink-0" />
                    <span className="truncate">{t.nome}</span>
                  </button>
                );
              })}
            </div>
          </div>

          {/* DYNAMIC CHECKLIST PER SERVICE TYPE */}
          {tipoBase === 'Limpeza de Ar' && (
            <div className="bg-slate-950/80 p-3.5 rounded-xl border border-slate-800 space-y-2.5">
              <h3 className="text-xs font-bold text-sky-400 uppercase tracking-wide flex items-center gap-1.5">
                <Sparkles className="w-3.5 h-3.5" />
                <span>Procedimento Técnico de Limpeza de Ar</span>
              </h3>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 text-xs">
                {[
                  { key: 'filtrosLavados', label: '1. Filtros desmontados e lavados' },
                  { key: 'serpentinaHigienizada', label: '2. Serpentina evaporadora escovada e limpa' },
                  { key: 'turbinaLimpa', label: '3. Turbina desobstruída (remoção de limo)' },
                  { key: 'drenoDesobstruido', label: '4. Dreno e mangueiras desobstruídos' },
                  { key: 'bandejaSanitizada', label: '5. Bandeja de condensado sanitizada' },
                  { key: 'condensadoraLavada', label: '6. Condensadora externa lavada' },
                  { key: 'aplicacaoBactericida', label: '7. Aplicação de bactericida homologado' },
                  { key: 'testeEletricoCorrente', label: '8. Teste elétrico e consumo do compressor' }
                ].map((item) => {
                  const checked = Boolean((checklist as any)[item.key]);
                  return (
                    <div
                      key={item.key}
                      onClick={() => toggleCheck(item.key as any)}
                      className="flex items-center gap-2 p-2 rounded-lg bg-slate-900 border border-slate-800/80 hover:border-slate-700 cursor-pointer select-none"
                    >
                      {checked ? (
                        <CheckSquare className="w-4 h-4 text-emerald-400 shrink-0" />
                      ) : (
                        <Square className="w-4 h-4 text-slate-500 shrink-0" />
                      )}
                      <span className={`text-[11px] ${checked ? 'text-white' : 'text-slate-400'}`}>
                        {item.label}
                      </span>
                    </div>
                  );
                })}
              </div>

              {/* Medições Técnicas */}
              <div className="grid grid-cols-3 gap-2 pt-2 border-t border-slate-800/80">
                <div>
                  <label className="text-[10px] text-slate-400 block mb-1">Corrente (A)</label>
                  <input
                    type="text"
                    value={checklist.correnteAmperes || ''}
                    onChange={(e) => setChecklist({ ...checklist, correnteAmperes: e.target.value })}
                    className="w-full px-2 py-1.5 bg-slate-900 border border-slate-700 rounded-lg text-xs text-white"
                  />
                </div>
                <div>
                  <label className="text-[10px] text-slate-400 block mb-1">Pressão (PSI)</label>
                  <input
                    type="text"
                    value={checklist.pressaoGasPSI || ''}
                    onChange={(e) => setChecklist({ ...checklist, pressaoGasPSI: e.target.value })}
                    className="w-full px-2 py-1.5 bg-slate-900 border border-slate-700 rounded-lg text-xs text-white"
                  />
                </div>
                <div>
                  <label className="text-[10px] text-slate-400 block mb-1">Delta T (Salto)</label>
                  <input
                    type="text"
                    value={checklist.saltoTermicoDeltaT || ''}
                    onChange={(e) => setChecklist({ ...checklist, saltoTermicoDeltaT: e.target.value })}
                    className="w-full px-2 py-1.5 bg-slate-900 border border-slate-700 rounded-lg text-xs text-white"
                  />
                </div>
              </div>
            </div>
          )}

          {tipoBase === 'Instalação' && (
            <div className="bg-slate-950/80 p-3.5 rounded-xl border border-slate-800 space-y-2.5">
              <h3 className="text-xs font-bold text-sky-400 uppercase tracking-wide flex items-center gap-1.5">
                <Layers className="w-3.5 h-3.5" />
                <span>Protocolo de Instalação Profissional Inovar</span>
              </h3>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 text-xs">
                {[
                  { key: 'suporteNivelado', label: '1. Suporte evaporadora/condensadora com nível de bolha' },
                  { key: 'testeNitrogenio', label: '2. Teste de estanqueidade das flanges com Nitrogênio' },
                  { key: 'valvulasLiberadas', label: '3. Abertura das válvulas de serviço após estanqueidade' },
                  { key: 'testeEletricoCorrente', label: '4. Fiação PP conforme norma do fabricante e disjuntor' }
                ].map((item) => {
                  const checked = Boolean((checklist as any)[item.key]);
                  return (
                    <div
                      key={item.key}
                      onClick={() => toggleCheck(item.key as any)}
                      className="flex items-center gap-2 p-2 rounded-lg bg-slate-900 border border-slate-800/80 hover:border-slate-700 cursor-pointer select-none"
                    >
                      {checked ? (
                        <CheckSquare className="w-4 h-4 text-emerald-400 shrink-0" />
                      ) : (
                        <Square className="w-4 h-4 text-slate-500 shrink-0" />
                      )}
                      <span className={`text-[11px] ${checked ? 'text-white' : 'text-slate-400'}`}>
                        {item.label}
                      </span>
                    </div>
                  );
                })}
              </div>

              <div className="grid grid-cols-2 gap-2 pt-2 border-t border-slate-800/80">
                <div>
                  <label className="text-[10px] text-slate-400 block mb-1">Vácuo da Linha (Microns)</label>
                  <input
                    type="text"
                    value={checklist.vacuoMicrons || ''}
                    onChange={(e) => setChecklist({ ...checklist, vacuoMicrons: e.target.value })}
                      placeholder="Informe o valor medido"
                    className="w-full px-2.5 py-1.5 bg-slate-900 border border-slate-700 rounded-lg text-xs text-white"
                  />
                </div>
                <div>
                  <label className="text-[10px] text-slate-400 block mb-1">Superaquecimento Útil</label>
                  <input
                    type="text"
                    value={checklist.superaquecimentoUtil || ''}
                    onChange={(e) => setChecklist({ ...checklist, superaquecimentoUtil: e.target.value })}
                    placeholder="Ex: 5°C a 7°C"
                    className="w-full px-2.5 py-1.5 bg-slate-900 border border-slate-700 rounded-lg text-xs text-white"
                  />
                </div>
              </div>
            </div>
          )}

          {tipoBase === 'Manutenção Corretiva' && (
            <div className="bg-slate-950/80 p-3.5 rounded-xl border border-slate-800 space-y-2.5">
              <h3 className="text-xs font-bold text-sky-400 uppercase tracking-wide flex items-center gap-1.5">
                <Wrench className="w-3.5 h-3.5" />
                <span>Diagnóstico Corretivo & Troca de Peças</span>
              </h3>

              <div className="space-y-2">
                <div>
                  <label className="text-[10px] text-slate-400 block mb-1">Diagnóstico / Defeito Identificado</label>
                  <input
                    type="text"
                    value={checklist.diagnosticoTecnico || ''}
                    onChange={(e) => setChecklist({ ...checklist, diagnosticoTecnico: e.target.value })}
                    placeholder="Ex: Capacitor de partida do compressor em curto / Sensor descalibrado"
                    className="w-full px-2.5 py-1.5 bg-slate-900 border border-slate-700 rounded-lg text-xs text-white"
                  />
                </div>

                <div className="grid grid-cols-2 gap-2">
                  <div>
                    <label className="text-[10px] text-slate-400 block mb-1">Teste do Capacitor (µF)</label>
                    <input
                      type="text"
                      value={checklist.capacitorTestado || ''}
                      onChange={(e) => setChecklist({ ...checklist, capacitorTestado: e.target.value })}
                      placeholder="Ex: 35 µF"
                      className="w-full px-2.5 py-1.5 bg-slate-900 border border-slate-700 rounded-lg text-xs text-white"
                    />
                  </div>
                  <div>
                    <label className="text-[10px] text-slate-400 block mb-1">Peça(s) Substituída(s)</label>
                    <input
                      type="text"
                      value={partsUsed}
                      onChange={(e) => {
                        setPartsUsed(e.target.value);
                        setChecklist({ ...checklist, pecasSubstituidas: e.target.value });
                      }}
                      placeholder="Ex: Capacitor 35µF 450V + Válvula"
                      className="w-full px-2.5 py-1.5 bg-slate-900 border border-slate-700 rounded-lg text-xs text-white"
                    />
                  </div>
                </div>
              </div>
            </div>
          )}

          {tipoBase === 'Recarga de Gás' && (
            <div className="bg-slate-950/80 p-3.5 rounded-xl border border-slate-800 space-y-2.5">
              <h3 className="text-xs font-bold text-sky-400 uppercase tracking-wide flex items-center gap-1.5">
                <Flame className="w-3.5 h-3.5" />
                <span>Procedimento de Carga de Fluido Refrigerante</span>
              </h3>

              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className="text-[10px] text-slate-400 block mb-1">Tipo de Gás</label>
                  <select
                    value={appliance.gasType || 'R-410A'}
                    disabled
                    title="Edite o gás na Ficha do Aparelho do cliente"
                    className="w-full px-2.5 py-1.5 bg-slate-900 border border-slate-700 rounded-lg text-xs text-white opacity-80 cursor-not-allowed"
                  >
                    <option value="R-410A">R-410A</option>
                    <option value="R-32">R-32</option>
                    <option value="R-22">R-22</option>
                    <option value="Outro">Outro</option>
                  </select>
                </div>
                <div>
                  <label className="text-[10px] text-slate-400 block mb-1">Carga na Balança (g)</label>
                  <input
                    type="text"
                    value={checklist.gasAdicionadoGramas || ''}
                    onChange={(e) => setChecklist({ ...checklist, gasAdicionadoGramas: e.target.value })}
                    placeholder="Ex: 550 gramas"
                    className="w-full px-2.5 py-1.5 bg-slate-900 border border-slate-700 rounded-lg text-xs text-white"
                  />
                </div>
              </div>
            </div>
          )}

          {/* CHECKLIST GENÉRICO: preventivas, avaliações e tipos personalizados */}
          {tipoBase !== 'Limpeza de Ar' &&
            tipoBase !== 'Instalação' &&
            tipoBase !== 'Manutenção Corretiva' &&
            tipoBase !== 'Recarga de Gás' && (
              <div className="bg-slate-950/80 p-3.5 rounded-xl border border-slate-800 space-y-2.5">
                <h3 className="text-xs font-bold text-sky-400 uppercase tracking-wide flex items-center gap-1.5">
                  <Wrench className="w-3.5 h-3.5" />
                  <span>Procedimento Executado — {serviceType}</span>
                </h3>
                <div>
                  <label className="text-[10px] text-slate-400 block mb-1">Diagnóstico / Descrição do Serviço</label>
                  <textarea
                    value={checklist.diagnosticoTecnico || ''}
                    onChange={(e) => setChecklist({ ...checklist, diagnosticoTecnico: e.target.value })}
                    rows={3}
                    placeholder="Descreva o que foi verificado e executado no equipamento..."
                    className="w-full px-2.5 py-2 bg-slate-900 border border-slate-700 rounded-lg text-xs text-white focus:outline-none focus:border-sky-500"
                  />
                </div>
                <div>
                  <label className="text-[10px] text-slate-400 block mb-1">Peças e Materiais Utilizados</label>
                  <input
                    type="text"
                    value={checklist.pecasSubstituidas || ''}
                    onChange={(e) => setChecklist({ ...checklist, pecasSubstituidas: e.target.value })}
                    placeholder="Ex: Capacitor 35µF, filtro de linha..."
                    className="w-full px-2.5 py-1.5 bg-slate-900 border border-slate-700 rounded-lg text-xs text-white focus:outline-none focus:border-sky-500"
                  />
                </div>
              </div>
            )}

          {/* FINANCIAL SECTION: Mão de obra + Peças = Total */}
          <div className="bg-slate-950/90 p-3.5 rounded-xl border border-slate-800 space-y-3">
            <h3 className="text-xs font-bold text-inovar-yellow uppercase tracking-wide flex items-center justify-between">
              <span>Fechamento da Ordem de Serviço</span>
              <span className="text-sm font-black text-emerald-400">Total: R$ {totalPrice.toFixed(2)}</span>
            </h3>

            <div className="grid grid-cols-1 sm:grid-cols-3 gap-2 text-xs">
              <div>
                <label className="text-[11px] text-slate-400 block mb-1">Mão de Obra (R$)</label>
                <input
                  type="number"
                  min="0"
                  step="10"
                  value={laborPrice}
                  onChange={(e) => setLaborPrice(Number(e.target.value))}
                  className="w-full px-3 py-2 bg-slate-900 border border-slate-700 rounded-xl text-white font-bold"
                />
              </div>

              <div>
                <label className="text-[11px] text-slate-400 block mb-1">Peças / Materiais (R$)</label>
                <input
                  type="number"
                  min="0"
                  step="10"
                  value={partsPrice}
                  onChange={(e) => setPartsPrice(Number(e.target.value))}
                  className="w-full px-3 py-2 bg-slate-900 border border-slate-700 rounded-xl text-white font-bold"
                />
              </div>

              <div>
                <label className="text-[11px] text-slate-400 block mb-1">Forma de Pagamento</label>
                <select
                  value={paymentMethod}
                  onChange={(e) => setPaymentMethod(e.target.value as any)}
                  className="w-full px-3 py-2 bg-slate-900 border border-slate-700 rounded-xl text-white text-xs font-bold"
                >
                  <option value="PIX">PIX</option>
                  <option value="Cartão Crédito">Cartão de Crédito</option>
                  <option value="Cartão Débito">Cartão de Débito</option>
                  <option value="Dinheiro">Dinheiro</option>
                  <option value="A Faturar">A Faturar (Boleto)</option>
                </select>
              </div>
            </div>

            <div className="grid grid-cols-2 gap-2 text-xs pt-1">
              <div>
                <label className="text-[11px] text-slate-400 block mb-1">Termo de Garantia</label>
                <select
                  value={warrantyDays}
                  onChange={(e) => setWarrantyDays(Number(e.target.value))}
                  className="w-full px-2.5 py-1.5 bg-slate-900 border border-slate-700 rounded-lg text-white"
                >
                  <option value={30}>30 Dias</option>
                  <option value={90}>90 Dias (Padrão Inovar)</option>
                  <option value={180}>180 Dias (Semestral)</option>
                  <option value={365}>365 Dias (1 Ano - Instalação)</option>
                </select>
              </div>

              <div>
                <label className="text-[11px] text-slate-400 block mb-1">Próximo Retorno Recomendado</label>
                <select
                  value={cycleMonths}
                  onChange={(e) => setCycleMonths(Number(e.target.value))}
                  className="w-full px-2.5 py-1.5 bg-slate-900 border border-slate-700 rounded-lg text-white"
                >
                  <option value={3}>3 meses (Clínicas e Comércios)</option>
                  <option value={6}>6 meses (Residencial Ideal)</option>
                  <option value={12}>12 meses (Anual)</option>
                </select>
              </div>
            </div>
          </div>

          <div>
            <label className="block text-[11px] text-slate-400 mb-1">Observações no Certificado do Cliente</label>
            <textarea
              rows={2}
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              placeholder="Ex: Aparelho testado e limpo com produto biodegradável. Recomenda-se manter portas fechadas ao ligar."
              className="w-full px-3 py-2 bg-slate-900 border border-slate-700 rounded-xl text-xs text-white placeholder-slate-500"
            ></textarea>
          </div>
        </form>

        {/* Sticky Action Footer */}
        <div className="p-3 sm:p-4 bg-slate-950 border-t border-slate-800 flex items-center justify-between gap-3 shrink-0">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2.5 bg-slate-800 hover:bg-slate-700 text-slate-300 font-semibold rounded-xl text-xs transition-colors"
          >
            Cancelar
          </button>

          <button
            onClick={handleFinish}
            className="flex-1 py-2.5 bg-inovar-yellow hover:brightness-105 active:scale-[0.99] text-inovar-navy font-bold rounded-xl text-xs sm:text-sm transition-all shadow-lg flex items-center justify-center gap-2"
          >
            <CheckSquare className="w-4 h-4" />
            <span>Concluir e gerar Ordem de Serviço (R$ {totalPrice.toFixed(2)})</span>
          </button>
        </div>
      </div>
    </div>
  );
};
