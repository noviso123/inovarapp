import React from 'react';
import { BudgetEstimate, TechnicianProfile, Client } from '../types';
import { BudgetService } from '../services/budgetService';
import { BudgetPdfService } from '../services/budgetPdfGenerator';
import { DocLinkService } from '../services/docLink';
import { supabase } from '../services/supabase';
import { WhatsAppService } from '../services/whatsapp';
import { emitirNotificacao } from '../services/appNotifications';
import {
  X,
  Share2,
  Download,
  CheckCircle,
  Clock,
  Trash2,
  FileText,
  DollarSign,
  ShieldCheck,
  Calendar,
  AlertCircle,
  ArrowRight,
  Edit3
} from 'lucide-react';
import { format, parseISO, isPast } from 'date-fns';

interface VisualizarOrcamentoModalProps {
  budget: BudgetEstimate;
  profile: TechnicianProfile;
  clients: Client[];
  onClose: () => void;
  onUpdateStatus: (budgetId: string, status: 'pendente' | 'aprovado' | 'recusado') => void;
  onDeleteBudget: (budgetId: string) => void;
  onIniciarServicoAprovado: (budget: BudgetEstimate) => void;
  onAgendarOrcamento?: (budget: BudgetEstimate) => void;
  onMarcarRecebido?: (budget: BudgetEstimate) => void;
  onEditar?: (budget: BudgetEstimate) => void;
}

export const VisualizarOrcamentoModal: React.FC<VisualizarOrcamentoModalProps> = ({
  budget,
  profile,
  clients,
  onClose,
  onUpdateStatus,
  onDeleteBudget,
  onIniciarServicoAprovado,
  onAgendarOrcamento,
  onMarcarRecebido,
  onEditar
}) => {
  const clienteAtual = clients.find((c) => c.id === budget.clientId);
  const aparelhoAtual = clienteAtual?.appliances?.find((a) => a.id === budget.applianceId);
  const budgetForDocument: BudgetEstimate = {
    ...budget,
    clientName: clienteAtual?.name || budget.clientName,
    clientPhone: clienteAtual?.phone || budget.clientPhone,
    clientDocument: clienteAtual?.document || budget.clientDocument,
    clientAddress: clienteAtual ? [clienteAtual.address, clienteAtual.neighborhood, clienteAtual.city].filter(Boolean).join(', ') : budget.clientAddress,
    equipmentName: budget.equipmentName || (aparelhoAtual ? `${aparelhoAtual.brand} ${aparelhoAtual.model || ''}`.trim() : undefined),
    applianceDesc: budget.applianceDesc || (aparelhoAtual ? `${aparelhoAtual.brand} ${aparelhoAtual.model || ''} ${aparelhoAtual.capacityBtu} BTUs (${aparelhoAtual.room})`.trim() : budget.applianceDesc)
  };
  const isExpired = isPast(new Date(budget.validUntil));

  const handleSendWhatsApp = async () => {
    // Envia o PDF como anexo pelo sistema próprio WhatsApp Go.
    let manualUrl: string | null = null;
    try {
      const doc = await BudgetPdfService.buildBudgetPdf(budgetForDocument, profile);
      const base64 = doc.output('datauristring');
      const { data: sessionData } = await supabase.auth.getSession();
      const up = await fetch('/api/documentos', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${sessionData?.session?.access_token || ''}` },
        body: JSON.stringify({ acao: 'upload', nome: 'ORCAMENTO_' + budget.clientName.replace(/\s+/g, '_') + '.pdf', base64 })
      });
      if (!up.ok) throw new Error('upload falhou');
      const { path } = await up.json();
      const lk = await fetch('/api/documentos', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${sessionData?.session?.access_token || ''}` },
        body: JSON.stringify({ acao: 'link', path, dias: 7 })
      });
      if (!lk.ok) throw new Error('link falhou');
      const { url, shortUrl } = await lk.json();
      if (!url) throw new Error('link vazio');
      manualUrl = shortUrl || url;
      const texto = BudgetService.generateWhatsAppBudget(budget, profile);
      const r = await fetch('/api/whatsapp', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${sessionData?.session?.access_token || ''}` },
        body: JSON.stringify({ acao: 'enviar', telefone: budget.clientPhone, texto, documento_url: url, documento_nome: 'PROPOSTA_' + budget.clientName.replace(/\s+/g, '_') + '.pdf' })
      });
      const envio = await r.json().catch(() => ({}));
      if (!r.ok || !envio.ok) throw new Error('envio automático indisponível');
      emitirNotificacao('Proposta na fila do WhatsApp', 'O documento comercial foi registrado para envio e pode ser acompanhado na central da equipe.');
      return;
    } catch (errEnvio) {
      // fallback: wa.me com texto + link do PDF hospedado
      try {
        if (!manualUrl) {
          const doc = await BudgetPdfService.buildBudgetPdf(budgetForDocument, profile);
          manualUrl = await DocLinkService.pdfParaLink(doc, 'ORCAMENTO.pdf');
        }
        const text = BudgetService.generateWhatsAppBudget(budget, profile) + (manualUrl ? '\n\n📄 *Proposta em PDF:* ' + manualUrl : '');
        WhatsAppService.openWhatsApp(budget.clientPhone, text);
      } catch { /* nada mais a fazer */ }
    }
  };

  const handleDownloadPdf = async () => {
    await BudgetPdfService.generateBudgetPdf(budgetForDocument, profile);
    emitirNotificacao('Documento gerado', 'A proposta foi baixada em PDF.');
  };

  const handleApprove = () => {
    onUpdateStatus(budget.id, 'aprovado');
    onIniciarServicoAprovado(budget);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-4 bg-black/85 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="bg-slate-900 border border-slate-700/80 rounded-2xl w-full max-w-xl max-h-[92vh] flex flex-col shadow-2xl text-slate-100 overflow-hidden">
        {/* Header */}
        <div className="p-4 bg-gradient-to-r from-inovar-navy via-slate-900 to-slate-950 border-b border-slate-800 flex items-center justify-between shrink-0">
          <div className="flex items-center gap-2.5">
            <div className="p-2 bg-inovar-yellow/20 text-inovar-yellow rounded-xl border border-inovar-yellow/30">
              <FileText className="w-5 h-5" />
            </div>
            <div>
              <span className="text-[10px] font-bold text-inovar-yellow uppercase tracking-wider block">
                INOVAR REFRIGERAÇÃO • DETALHES DO ORÇAMENTO
              </span>
              <h3 className="text-base font-bold text-white flex items-center gap-2">
                <span>Proposta #{budget.id.toUpperCase().slice(0, 8)}</span>
                <span
                  className={`text-[10px] px-2 py-0.5 rounded-full font-bold uppercase ${
                    budget.status === 'aprovado'
                      ? 'bg-emerald-500/20 text-emerald-300 border border-emerald-500/30'
                      : budget.status === 'recusado'
                      ? 'bg-red-500/20 text-red-300 border border-red-500/30'
                      : 'bg-amber-500/20 text-amber-300 border border-amber-500/30'
                  }`}
                >
                  {budget.status}
                </span>
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

        {/* Body */}
        <div className="p-4 sm:p-5 overflow-y-auto space-y-4 text-xs">
          {/* Status banner */}
          <div
            className={`p-3 rounded-xl border flex items-center justify-between ${
              budget.status === 'aprovado'
                ? 'bg-emerald-950/40 border-emerald-800/80 text-emerald-200'
                : isExpired
                ? 'bg-red-950/40 border-red-800/80 text-red-200'
                : 'bg-amber-950/40 border-amber-800/80 text-amber-200'
            }`}
          >
            <div className="flex items-center gap-2.5">
              {budget.status === 'aprovado' ? (
                <CheckCircle className="w-5 h-5 text-emerald-400" />
              ) : isExpired ? (
                <AlertCircle className="w-5 h-5 text-red-400" />
              ) : (
                <Clock className="w-5 h-5 text-amber-400" />
              )}
              <div>
                <p className="font-bold uppercase tracking-wider text-[11px]">
                  {budget.status === 'aprovado'
                    ? 'Orçamento Aprovado pelo Cliente'
                    : isExpired
                    ? 'Proposta Vencida'
                    : 'Aguardando Aprovação do Cliente'}
                </p>
                <p className="text-[10px] text-slate-300">
                  Emitido em: {format(new Date(budget.date), 'dd/MM/yyyy')} • Validade até:{' '}
                  {format(new Date(budget.validUntil), 'dd/MM/yyyy')}
                </p>
              </div>
            </div>

            {budget.status === 'pendente' && (
              <button
                onClick={handleApprove}
                className="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 text-white rounded-lg font-bold text-xs flex items-center gap-1 shadow-md transition-all active:scale-95"
              >
                <span>Aprovar Orçamento</span>
                <ArrowRight className="w-3.5 h-3.5" />
              </button>
            )}
            {budget.status === 'aprovado' && (
              <div className="flex flex-col sm:flex-row gap-1.5">
                <button
                  onClick={() => onAgendarOrcamento?.(budget)}
                  className="px-3 py-1.5 bg-blue-600 hover:bg-blue-500 text-white rounded-lg font-bold text-xs flex items-center gap-1 shadow-md transition-all active:scale-95"
                >
                  <Calendar className="w-3.5 h-3.5" />
                  <span>Agendar Atendimento</span>
                </button>
                {!budget.pago ? (
                  <button
                    onClick={() => onMarcarRecebido?.(budget)}
                    className="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 text-white rounded-lg font-bold text-xs flex items-center gap-1 shadow-md transition-all active:scale-95"
                  >
                    <DollarSign className="w-3.5 h-3.5" />
                    <span>Marcar como Recebido</span>
                  </button>
                ) : (
                  <span className="px-3 py-1.5 bg-emerald-500/20 text-emerald-300 border border-emerald-500/30 rounded-lg font-bold text-xs flex items-center gap-1">
                    <CheckCircle className="w-3.5 h-3.5" />
                    <span>RECEBIDO</span>
                  </span>
                )}
              </div>
            )}
          </div>

          {/* Dados do Cliente */}
          <div className="bg-slate-800/70 p-3 rounded-xl border border-slate-700/80 space-y-1">
            <span className="text-[10px] text-slate-400 uppercase font-bold block">Cliente & Aparelho</span>
            <p className="font-bold text-sm text-white">{budget.clientName}</p>
            {budget.equipmentName && <p className="text-[11px] text-sky-300">Equipamento: {budget.equipmentName}</p>}
            <p className="text-[11px] text-slate-300">
              WhatsApp: {budget.clientPhone} {budget.applianceDesc ? `• ${budget.applianceDesc}` : ''}
            </p>
          </div>

          {/* Tabela de Itens */}
          <div className="bg-slate-800/70 rounded-xl border border-slate-700/80 overflow-hidden">
            <div className="bg-slate-850 p-2.5 border-b border-slate-700/80 flex justify-between font-bold text-slate-300 text-[11px]">
              <span>Descrição dos Serviços e Materiais</span>
              <span>Total</span>
            </div>
            <div className="divide-y divide-slate-800/80 p-1">
              {budget.items.map((it) => (
                <div key={it.id} className="p-2 flex justify-between items-center text-xs">
                  <div>
                    <span className="text-white font-medium block">{it.description}</span>
                    <span className="text-[10px] text-slate-400">
                      {it.quantity}x • R$ {it.unitPrice.toFixed(2)} cada
                    </span>
                  </div>
                  <span className="font-bold text-slate-200">R$ {it.totalPrice.toFixed(2)}</span>
                </div>
              ))}
            </div>

            {/* Totais */}
            <div className="bg-slate-850/80 p-3 border-t border-slate-700/80 space-y-1 text-right">
              <div className="text-[11px] text-slate-400">
                Subtotal: <span className="font-semibold text-slate-200">R$ {budget.totalValue.toFixed(2)}</span>
              </div>
              {budget.discount > 0 && (
                <div className="text-[11px] text-red-400">
                  Desconto: - R$ {budget.discount.toFixed(2)}
                </div>
              )}
              <div className="text-base font-extrabold text-emerald-400 pt-1 border-t border-slate-700/50">
                Valor Total: R$ {budget.finalValue.toFixed(2)}
              </div>
            </div>
          </div>

          {/* Condições comerciais */}
          <div className="bg-slate-800/70 p-3.5 rounded-xl border border-slate-700/80 space-y-2 text-[11px] text-slate-300">
            <div>
              <span className="font-semibold text-slate-400 block">Condições de Pagamento:</span>
              <span>{budget.paymentConditions}</span>
            </div>
            <div className="grid grid-cols-2 gap-2">
              <div>
                <span className="font-semibold text-slate-400 block">Tempo de Execução:</span>
                <span>{budget.executionTime}</span>
              </div>
              <div>
                <span className="font-semibold text-slate-400 block">Garantia:</span>
                <span className="text-emerald-400 font-semibold">{budget.warrantyTerms}</span>
              </div>
            </div>
            {budget.notes && (
              <div>
                <span className="font-semibold text-slate-400 block">Observações:</span>
                <span className="italic">{budget.notes}</span>
              </div>
            )}
          </div>

          {/* Assinatura digital do cliente */}
          {budget.assinatura && (
            <div className="bg-white rounded-xl border-2 border-emerald-700/60 p-3 text-slate-900">
              <div className="flex items-center justify-between mb-2">
                <span className="text-[10px] font-extrabold uppercase tracking-wider text-emerald-700 flex items-center gap-1">
                  <ShieldCheck className="w-3.5 h-3.5" />
                  Assinatura digital do cliente
                </span>
                {budget.assinaturaEm && (
                  <span className="text-[10px] text-slate-500 font-semibold">
                    {format(new Date(budget.assinaturaEm), 'dd/MM/yyyy')} às {format(new Date(budget.assinaturaEm), 'HH:mm')}
                  </span>
                )}
              </div>
              <img src={budget.assinatura} alt="Assinatura do cliente" className="max-h-28 object-contain mx-auto" />
              <p className="text-center text-[10px] text-slate-600 border-t border-slate-200 mt-2 pt-1.5 font-semibold">
                {budget.clientName} — concordante com os termos desta proposta
              </p>
            </div>
          )}

          {/* Ações */}
          <div className="space-y-2 pt-1">
            <button
              onClick={handleSendWhatsApp}
              className="w-full py-2.5 px-4 bg-emerald-600 hover:bg-emerald-500 text-white rounded-xl font-bold text-xs shadow-md shadow-emerald-900/20 flex items-center justify-center gap-2 transition-all active:scale-95"
            >
              <Share2 className="w-4 h-4" />
              <span>Enviar Proposta Comercial no WhatsApp</span>
            </button>

            <button
              onClick={handleDownloadPdf}
              className="w-full py-2.5 px-4 bg-slate-800 hover:bg-slate-700 text-white rounded-xl font-semibold text-xs border border-slate-700 flex items-center justify-center gap-2 transition-all active:scale-95"
            >
              <Download className="w-4 h-4 text-sky-400" />
              <span>Baixar Proposta Oficial em PDF</span>
            </button>

            <div className="flex gap-2 pt-1">
              {budget.status !== 'aprovado' && onEditar && (
                <button
                  onClick={() => onEditar(budget)}
                  className="flex-1 py-1.5 bg-sky-800 hover:bg-sky-700 text-sky-200 rounded-lg text-xs font-bold flex items-center justify-center gap-1.5"
                  title="Editar este orçamento em vez de gerar um novo"
                >
                  <Edit3 className="w-3.5 h-3.5" />
                  <span>Editar Orçamento</span>
                </button>
              )}

              {budget.status !== 'aprovado' && (
                <button
                  onClick={() => onUpdateStatus(budget.id, 'recusado')}
                  className="flex-1 py-1.5 bg-slate-800 hover:bg-slate-750 text-slate-400 hover:text-slate-200 rounded-lg text-xs"
                >
                  Marcar como Recusado
                </button>
              )}

              <button
                onClick={() => {
                  if (confirm('Deseja realmente excluir este orçamento?')) {
                    onDeleteBudget(budget.id);
                    onClose();
                  }
                }}
                className="py-1.5 px-3 text-slate-500 hover:text-red-400 rounded-lg text-xs flex items-center justify-center gap-1 transition-colors"
              >
                <Trash2 className="w-3.5 h-3.5" />
                <span>Excluir</span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
