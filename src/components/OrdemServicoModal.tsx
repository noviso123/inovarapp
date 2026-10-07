import React from 'react';
import { useState, useEffect, useRef } from 'react';
import { Client, Appliance, MaintenanceRecord, TechnicianProfile } from '../types';
import { WhatsAppService } from '../services/whatsapp';
import { PdfService } from '../services/pdfGenerator';
import { emitirNotificacao } from '../services/appNotifications';
import { DocLinkService } from '../services/docLink';
import { supabase } from '../services/supabase';
import {
  X,
  FileText,
  Download,
  Share2,
  Camera,
  Loader2,
  ShieldCheck,
  CheckCircle,
  Calendar,
  DollarSign,
  Trash2,
  Sparkles,
  Zap,
  Wrench,
  Flame,
  Activity
} from 'lucide-react';
import { format, parseISO, addDays, differenceInDays } from 'date-fns';
import { AttachmentStore, fileToDataUrl } from '../services/attachments';

interface OrdemServicoModalProps {
  client: Client;
  appliance: Appliance;
  maintenance: MaintenanceRecord;
  profile: TechnicianProfile;
  onClose: () => void;
  onDeleteMaintenance?: (recordId: string) => void;
  onEditOS?: (serviceId: string, campos: { status?: string; data?: string; valor?: number; garantiaDias?: number; observacoes?: string }) => void;
}

export const OrdemServicoModal: React.FC<OrdemServicoModalProps> = ({
  client,
  appliance,
  maintenance,
  profile,
  onClose,
  onDeleteMaintenance,
  onEditOS
}) => {
  // Datas blindadas: OS de serviço agendado/cancelado não tem retorno nem conclusão —
  // sem isso o modal quebrava a tela inteira ao abrir.
  const dataBaseStr = maintenance.date || maintenance.scheduledDate || maintenance.completedAt || new Date().toISOString().slice(0, 10);
  const serviceDate = parseISO(String(dataBaseStr).slice(0, 10) + 'T00:00:00');
  const warrantyDays = maintenance.warrantyDays || 90;
  const warrantyExpiryDate = addDays(serviceDate, warrantyDays);
  const daysRemaining = differenceInDays(warrantyExpiryDate, new Date());
  const isWarrantyValid = daysRemaining >= 0;
  const retornoISO = maintenance.returnDate && !isNaN(parseISO(maintenance.returnDate).getTime())
    ? maintenance.returnDate
    : addDays(serviceDate, warrantyDays).toISOString().slice(0, 10);
  const retornoDate = parseISO(retornoISO);

  const handleDownloadPdf = async () => {
    await PdfService.generateServiceOrderPdf(client, appliance, maintenance, profile);
    emitirNotificacao('Documento gerado', 'A Ordem de Serviço foi baixada em PDF.');
  };

  const handleSendWhatsAppReceipt = async () => {
    const msg0 = WhatsAppService.generateReceiptMessage(client, appliance, maintenance, profile);
    let manualUrl: string | null = null;
    // Envia a OS como anexo PDF pelo sistema próprio WhatsApp Go.
    try {
      const doc = await PdfService.buildServiceOrderPdf(client, appliance, maintenance, profile);
      const base64 = doc.output('datauristring');
      const { data: sessionData } = await supabase.auth.getSession();
      const up = await fetch('/api/documentos', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${sessionData?.session?.access_token || ''}` },
        body: JSON.stringify({ acao: 'upload', nome: 'OS_' + client.name.replace(/\s+/g, '_') + '.pdf', base64 })
      });
      if (!up.ok) throw new Error('upload falhou');
      const { path } = await up.json();
      const lk = await fetch('/api/documentos', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${sessionData?.session?.access_token || ''}` },
        body: JSON.stringify({ acao: 'link', path, dias: 30 })
      });
      if (!lk.ok) throw new Error('link falhou');
      const { url, shortUrl } = await lk.json();
      if (!url) throw new Error('link vazio');
      manualUrl = shortUrl || url;
      const r = await fetch('/api/whatsapp', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${sessionData?.session?.access_token || ''}` },
        body: JSON.stringify({ acao: 'enviar', telefone: client.phone, texto: msg0, documento_url: url, documento_nome: 'OS_' + client.name.replace(/\s+/g, '_') + '.pdf' })
      });
      const envio = await r.json().catch(() => ({}));
      if (!r.ok || !envio.ok) throw new Error('envio automático indisponível');
      emitirNotificacao('Documento na fila do WhatsApp', 'A Ordem de Serviço foi registrada para envio e pode ser acompanhada na central da equipe.');
    } catch (errAnexo) {
      // fallback: wa.me com texto + link do PDF hospedado
      try {
        if (!manualUrl) {
          const doc = await PdfService.buildServiceOrderPdf(client, appliance, maintenance, profile);
          manualUrl = await DocLinkService.pdfParaLink(doc, 'OS.pdf');
        }
        let msg = WhatsAppService.generateReceiptMessage(client, appliance, maintenance, profile);
        if (manualUrl) msg += '\n\n📄 *OS em PDF:* ' + manualUrl;
        WhatsAppService.openWhatsApp(client.phone, msg);
      } catch { /* nada mais a fazer */ }
    }
  };

  // ---- Fotos do serviço (registro fotográfico) ----
  const fileRef = useRef<HTMLInputElement>(null);
  const [fotos, setFotos] = useState<{ nome: string; caminho: string; url: string }[]>([]);
  const [carregandoFotos, setCarregandoFotos] = useState(false);
  const [enviandoFoto, setEnviandoFoto] = useState(false);
  const [erroFoto, setErroFoto] = useState('');
  const [zoomFoto, setZoomFoto] = useState<{ nome: string; url: string } | null>(null);

  // Comprime a imagem no dispositivo (câmera do celular gera 3-8MB, que estoura
  // o limite do gateway em base64) — máx. 1600px, JPEG ~200KB
  const comprimirImagem = (file: File): Promise<string> =>
    new Promise((resolve, reject) => {
      const fr = new FileReader();
      fr.onerror = () => reject(new Error('Falha ao ler o arquivo'));
      fr.onload = () => {
        const bruto = fr.result as string;
        const img = new Image();
        img.onerror = () => resolve(bruto); // decodificação falhou: segue com o original
        img.onload = () => {
          try {
            const MAX = 1600;
            const escala = Math.min(1, MAX / Math.max(img.width, img.height));
            const canvas = document.createElement('canvas');
            canvas.width = Math.round(img.width * escala);
            canvas.height = Math.round(img.height * escala);
            const ctx = canvas.getContext('2d');
            if (!ctx) return resolve(bruto);
            ctx.drawImage(img, 0, 0, canvas.width, canvas.height);
            resolve(canvas.toDataURL('image/jpeg', 0.82));
          } catch {
            resolve(bruto);
          }
        };
        img.src = bruto;
      };
      fr.readAsDataURL(file);
    });

  const carregarFotos = async () => {
    let remotas: { nome: string; caminho: string; url: string }[] = [];
    try {
      const { data: sessionData } = await supabase.auth.getSession();
      const r = await fetch('/api/documentos', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${sessionData?.session?.access_token || ''}` },
        body: JSON.stringify({ acao: 'fotos', serviceId: maintenance.id })
      });
      const j = await r.json();
      remotas = j.fotos || [];
    } catch { /* silencioso */ }
    try {
      const locais = await AttachmentStore.list(maintenance.id);
      setFotos([...remotas, ...locais.map((x) => ({ nome: x.name, caminho: `local:${x.id}`, url: x.dataUrl }))]);
    } catch { setFotos(remotas); }
  };

  useEffect(() => { carregarFotos(); }, [maintenance.id]);

  const anexarFotos = async (files: FileList | null) => {
    if (!files || !files.length) return;
    setEnviandoFoto(true);
    setErroFoto('');
    try {
      const { data: sessionData } = await supabase.auth.getSession();
      const auth = { 'Content-Type': 'application/json', Authorization: `Bearer ${sessionData?.session?.access_token || ''}` };
      let ok = 0;
      let falhas = 0;
      for (const file of Array.from(files)) {
        try {
          const base64 = await comprimirImagem(file);
          const r = await fetch('/api/documentos', {
            method: 'POST',
            headers: auth,
            body: JSON.stringify({ acao: 'upload', nome: file.name, base64, pasta: 'fotos', serviceId: maintenance.id })
          });
          if (r.ok) ok++;
          else {
            await AttachmentStore.save({ serviceId: maintenance.id, name: file.name, type: file.type, size: file.size, dataUrl: await fileToDataUrl(file) });
            ok++;
          }
        } catch {
          try {
            const dataUrl = await fileToDataUrl(file);
            await AttachmentStore.save({ serviceId: maintenance.id, name: file.name, type: file.type, size: file.size, dataUrl });
            ok++;
          } catch { falhas++; }
        }
      }
      if (ok > 0) {
        await carregarFotos();
        setErroFoto(falhas > 0 ? `${ok} foto(s) anexada(s), ${falhas} falharam. Tente novamente para as que faltaram.` : '');
      } else if (falhas > 0) {
        setErroFoto('Não foi possível enviar as fotos (arquivo muito grande ou sem conexão). Tente novamente.');
      }
    } finally {
      setEnviandoFoto(false);
      if (fileRef.current) fileRef.current.value = '';
    }
  };

  const excluirFoto = async (foto: { caminho: string }) => {
    if (!confirm('Excluir esta foto?')) return;
    try {
      if (foto.caminho.startsWith('local:')) {
        await AttachmentStore.remove(foto.caminho.slice(6));
        await carregarFotos();
        return;
      }
      const { data: sessionData } = await supabase.auth.getSession();
      await fetch('/api/documentos', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${sessionData?.session?.access_token || ''}` },
        body: JSON.stringify({ acao: 'excluirfoto', path: foto.caminho })
      });
      await carregarFotos();
    } catch { /* ignore */ }
  };


  const chk = maintenance.checklist;
  const isInstalacao = maintenance.serviceType === 'Instalação';
  const isCorretiva = maintenance.serviceType === 'Manutenção Corretiva';
  const isGas = maintenance.serviceType === 'Recarga de Gás';

  // ---- Modo edicao da OS (valor, data, garantia, status) ----
  const [editandoOS, setEditandoOS] = useState(false);
  const [editData, setEditData] = useState(format(serviceDate, 'yyyy-MM-dd'));
  const [editValor, setEditValor] = useState(String(maintenance.price));
  const [editGarantia, setEditGarantia] = useState(String(maintenance.warrantyDays || 90));
  const [editStatus, setEditStatus] = useState(
    maintenance.status === 'concluido' ? 'CONCLUIDO' : maintenance.status === 'em_andamento' ? 'EM_ANDAMENTO' : maintenance.status === 'agendado' ? 'AGENDADO' : 'CANCELADO'
  );
  const [salvandoOS, setSalvandoOS] = useState(false);

  const salvarEdicaoOS = () => {
    if (!onEditOS) return;
    setSalvandoOS(true);
    onEditOS(maintenance.id, {
      status: editStatus,
      data: editData,
      valor: Number(editValor),
      garantiaDias: Number(editGarantia)
    });
    setSalvandoOS(false);
    setEditandoOS(false);
  };

  const laborPrice = maintenance.laborPrice || (maintenance.partsPrice ? maintenance.price - maintenance.partsPrice : maintenance.price);
  const partsPrice = maintenance.partsPrice || 0;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-4 bg-slate-950/85 backdrop-blur-xs overflow-y-auto">
      <div className="bg-white text-slate-900 rounded-2xl w-full max-w-lg shadow-2xl border border-slate-200 overflow-hidden my-4">
        {/* Top bar */}
        <div className="bg-gradient-to-r from-slate-900 via-blue-950 to-slate-900 p-4 text-white flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <div className="p-2 bg-blue-500/20 border border-blue-400/30 rounded-xl text-sky-400">
              <ShieldCheck className="w-5 h-5" />
            </div>
            <div>
              <span className="text-[10px] font-bold text-sky-400 uppercase tracking-wider block">
                INOVAR REFRIGERAÇÃO • COMPROVANTE & GARANTIA
              </span>
              <h3 className="text-base font-bold text-white flex items-center gap-2">
                <span>Ordem de Serviço</span>
                <span className="text-xs px-2 py-0.5 rounded bg-sky-500/20 text-sky-300 font-mono">
                  #{maintenance.id.toUpperCase().slice(0, 8)}
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

        {/* Content */}
        <div className="p-4 sm:p-5 max-h-[75vh] overflow-y-auto space-y-3.5">
          {/* Guarantee status card */}
          <div className={`p-3.5 rounded-xl border flex items-center justify-between ${
            isWarrantyValid
              ? 'bg-emerald-50 border-emerald-200 text-emerald-900'
              : 'bg-amber-50 border-amber-200 text-amber-900'
          }`}>
            <div className="flex items-center gap-2.5">
              <div className={`p-2 rounded-lg ${isWarrantyValid ? 'bg-emerald-100 text-emerald-700' : 'bg-amber-100 text-amber-700'}`}>
                <ShieldCheck className="w-5 h-5" />
              </div>
              <div>
                <p className="text-xs font-bold uppercase tracking-wider">
                  {isWarrantyValid ? 'Garantia Inovar Ativa' : 'Garantia Expirada'}
                </p>
                <p className="text-[11px] text-slate-600">
                  {isWarrantyValid
                    ? `Válida por mais ${daysRemaining} dias (até ${format(warrantyExpiryDate, 'dd/MM/yyyy')})`
                    : `Expirou em ${format(warrantyExpiryDate, 'dd/MM/yyyy')}`}
                </p>
              </div>
            </div>
            <span className="text-xs font-extrabold px-2.5 py-1 bg-white rounded-md shadow-xs text-emerald-700 border border-emerald-200">
              {warrantyDays} dias
            </span>
          </div>

          {/* OS Summary Box */}
          <div className="bg-slate-50 p-4 rounded-xl border border-slate-200 space-y-2.5 text-xs">
            <div className="flex justify-between border-b border-slate-200 pb-2">
              <span className="text-slate-500 font-medium">Cliente:</span>
              <span className="font-bold text-slate-800">{client.name}</span>
            </div>

            <div className="flex justify-between border-b border-slate-200 pb-2">
              <span className="text-slate-500 font-medium">Equipamento:</span>
              <span className="font-bold text-slate-800">
                {appliance.brand} {appliance.capacityBtu} BTUs ({appliance.room})
              </span>
            </div>

            <div className="flex justify-between border-b border-slate-200 pb-2 items-center">
              <span className="text-slate-500 font-medium">Serviço Realizado:</span>
              <span className="font-bold px-2 py-0.5 bg-blue-100 text-blue-800 rounded text-[11px]">
                {maintenance.serviceType}
              </span>
            </div>

            <div className="flex justify-between border-b border-slate-200 pb-2">
              <span className="text-slate-500 font-medium">
                {maintenance.status === 'concluido' ? 'Data de Conclusão:' : maintenance.status === 'cancelado' ? 'Data de Cancelamento:' : 'Data Agendada:'}
              </span>
              <span className="text-slate-800 font-semibold">{format(serviceDate, 'dd/MM/yyyy')}</span>
            </div>

            <div className="flex justify-between border-b border-slate-200 pb-2">
              <span className="text-slate-500 font-medium">Próximo Retorno Recomendado:</span>
              <span className="font-bold text-sky-700">{format(retornoDate, 'dd/MM/yyyy')}</span>
            </div>

            {/* Discriminação de Valores */}
            <div className="pt-1.5 space-y-1 bg-white p-3 rounded-lg border border-slate-200">
              <div className="flex justify-between text-[11px] text-slate-600">
                <span>Mão de Obra Técnica:</span>
                <span className="font-semibold">R$ {laborPrice.toFixed(2)}</span>
              </div>
              {partsPrice > 0 && (
                <div className="flex justify-between text-[11px] text-slate-600">
                  <span>Peças / Materiais {maintenance.partsUsed ? `(${maintenance.partsUsed})` : ''}:</span>
                  <span className="font-semibold">R$ {partsPrice.toFixed(2)}</span>
                </div>
              )}
              <div className="flex justify-between items-center pt-1 border-t border-slate-100 text-xs">
                <span className="font-bold text-slate-800">VALOR TOTAL:</span>
                <span className="font-black text-emerald-700 text-sm">
                  R$ {maintenance.price.toFixed(2)}
                  <span className="text-[10px] font-normal text-slate-500 block text-right">
                    {maintenance.paymentMethod}
                  </span>
                </span>
              </div>
            </div>
          </div>

          {/* Ciclo de vida do serviço: status + todas as datas, tudo à vista */}
          {(() => {
            const rot: Record<string, { texto: string; cls: string }> = {
              concluido: { texto: 'Concluído', cls: 'bg-emerald-100 text-emerald-800 border-emerald-200' },
              em_andamento: { texto: 'Em Andamento', cls: 'bg-violet-100 text-violet-800 border-violet-200' },
              agendado: { texto: 'Agendado', cls: 'bg-blue-100 text-blue-800 border-blue-200' },
              cancelado: { texto: 'Cancelado', cls: 'bg-red-100 text-red-700 border-red-200' }
            };
            const st = rot[maintenance.status] || rot.agendado;
            const dt = (v?: string | null) => (v && !isNaN(parseISO(String(v)).getTime()) ? format(parseISO(String(v)), 'dd/MM/yyyy') : '—');
            return (
              <div className="bg-slate-50 p-3.5 rounded-xl border border-slate-200">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-[11px] font-bold text-slate-700 uppercase tracking-wider">Ciclo de Vida do Serviço</span>
                  <span className={`px-2.5 py-0.5 rounded-full text-[10px] font-extrabold border ${st.cls}`}>{st.texto}</span>
                </div>
                <div className="grid grid-cols-2 gap-1.5 text-[11px]">
                  <div className="p-1.5 bg-white rounded border border-slate-200">
                    <span className="text-slate-500 block text-[10px]">📅 Agendamento:</span>
                    <span className="font-bold text-slate-800">{dt(maintenance.scheduledDate)} {maintenance.scheduledTime || ''}</span>
                  </div>
                  <div className="p-1.5 bg-white rounded border border-slate-200">
                    <span className="text-slate-500 block text-[10px]">▶️ Início:</span>
                    <span className="font-bold text-violet-700">{dt(maintenance.startedAt)}</span>
                  </div>
                  <div className="p-1.5 bg-white rounded border border-slate-200">
                    <span className="text-slate-500 block text-[10px]">✅ Conclusão:</span>
                    <span className="font-bold text-emerald-700">{dt(maintenance.completedAt || (maintenance.status === 'concluido' ? maintenance.date : null))}</span>
                  </div>
                  <div className="p-1.5 bg-white rounded border border-slate-200">
                    <span className="text-slate-500 block text-[10px]">✖ Cancelamento:</span>
                    <span className="font-bold text-red-600">{dt(maintenance.cancelledAt)}</span>
                  </div>
                </div>
                {maintenance.cancellationReason && (
                  <p className="text-[10px] text-slate-500 mt-1.5">Motivo: {maintenance.cancellationReason}</p>
                )}
              </div>
            );
          })()}

          {/* Dados Técnicos & Checklist Realizado */}
          {chk && (
            <div className="bg-slate-50 p-3.5 rounded-xl border border-slate-200">
              <span className="text-[11px] font-bold text-slate-700 block uppercase mb-2">
                Especificações Técnicas Registradas:
              </span>

              {isInstalacao ? (
                <div className="grid grid-cols-2 gap-1.5 text-[11px]">
                  <div className="p-1.5 bg-white rounded border border-slate-200">
                    <span className="text-slate-500 block text-[10px]">Vácuo Atingido:</span>
                    <span className="font-bold text-sky-700">{chk.vacuoMicrons || 'Não informado'}</span>
                  </div>
                  <div className="p-1.5 bg-white rounded border border-slate-200">
                    <span className="text-slate-500 block text-[10px]">Teste de Estanqueidade:</span>
                    <span className="font-bold text-emerald-700">{chk.testeNitrogenio ? 'Registrado' : 'Não informado'}</span>
                  </div>
                  <div className="p-1.5 bg-white rounded border border-slate-200">
                    <span className="text-slate-500 block text-[10px]">Salto Térmico (ΔT):</span>
                    <span className="font-bold text-slate-800">{chk.saltoTermicoDeltaT || 'Não informado'}</span>
                  </div>
                  <div className="p-1.5 bg-white rounded border border-slate-200">
                    <span className="text-slate-500 block text-[10px]">Corrente de Operação:</span>
                    <span className="font-bold text-slate-800">{chk.correnteAmperes || 'Não informado'}</span>
                  </div>
                </div>
              ) : isCorretiva ? (
                <div className="space-y-1.5 text-[11px]">
                  <div className="p-1.5 bg-white rounded border border-slate-200">
                    <span className="text-slate-500 block text-[10px]">Diagnóstico:</span>
                    <span className="font-semibold text-slate-800">{chk.diagnosticoTecnico || 'Não informado'}</span>
                  </div>
                  <div className="grid grid-cols-2 gap-1.5">
                    <div className="p-1.5 bg-white rounded border border-slate-200">
                      <span className="text-slate-500 block text-[10px]">Peça Substituída:</span>
                      <span className="font-bold text-amber-700">{maintenance.partsUsed || chk.pecasSubstituidas || 'Não informado'}</span>
                    </div>
                    <div className="p-1.5 bg-white rounded border border-slate-200">
                      <span className="text-slate-500 block text-[10px]">Capacitor Testado:</span>
                      <span className="font-bold text-slate-800">{chk.capacitorTestado || 'Não informado'}</span>
                    </div>
                  </div>
                </div>
              ) : isGas ? (
                <div className="grid grid-cols-2 gap-1.5 text-[11px]">
                  <div className="p-1.5 bg-white rounded border border-slate-200">
                    <span className="text-slate-500 block text-[10px]">Fluido Injetado:</span>
                    <span className="font-bold text-purple-700">{chk.gasAdicionadoGramas || 'Não informado'}</span>
                  </div>
                  <div className="p-1.5 bg-white rounded border border-slate-200">
                    <span className="text-slate-500 block text-[10px]">Pressão de Sucção:</span>
                    <span className="font-bold text-slate-800">{chk.pressaoGasPSI || 'Não informado'}</span>
                  </div>
                  <div className="p-1.5 bg-white rounded border border-slate-200">
                    <span className="text-slate-500 block text-[10px]">Vazamento Sanado:</span>
                    <span className="font-bold text-emerald-700">{chk.testeNitrogenio ? 'Registrado' : 'Não informado'}</span>
                  </div>
                  <div className="p-1.5 bg-white rounded border border-slate-200">
                    <span className="text-slate-500 block text-[10px]">Salto Térmico:</span>
                    <span className="font-bold text-slate-800">{chk.saltoTermicoDeltaT || 'Não informado'}</span>
                  </div>
                </div>
              ) : (
                <div className="flex flex-wrap gap-1.5">
                  {chk.filtrosLavados && (
                    <span className="px-2 py-0.5 bg-emerald-100 text-emerald-800 rounded text-[11px] font-medium">
                      ✓ Filtros Sanitizados
                    </span>
                  )}
                  {chk.serpentinaHigienizada && (
                    <span className="px-2 py-0.5 bg-emerald-100 text-emerald-800 rounded text-[11px] font-medium">
                      ✓ Serpentina Desinfectada
                    </span>
                  )}
                  {chk.turbinaLimpa && (
                    <span className="px-2 py-0.5 bg-emerald-100 text-emerald-800 rounded text-[11px] font-medium">
                      ✓ Turbina limpa
                    </span>
                  )}
                  {chk.drenoDesobstruido && (
                    <span className="px-2 py-0.5 bg-emerald-100 text-emerald-800 rounded text-[11px] font-medium">
                      ✓ Dreno Desobstruído
                    </span>
                  )}
                  {chk.aplicacaoBactericida && (
                    <span className="px-2 py-0.5 bg-emerald-100 text-emerald-800 rounded text-[11px] font-medium">
                      ✓ Bactericida Hospitalar
                    </span>
                  )}
                  {chk.saltoTermicoDeltaT && (
                    <span className="px-2 py-0.5 bg-blue-100 text-blue-800 rounded text-[11px] font-medium">
                      ΔT: {chk.saltoTermicoDeltaT}
                    </span>
                  )}
                </div>
              )}
            </div>
          )}

          {/* Registro fotográfico do serviço */}
          <div className="bg-slate-800/70 rounded-xl border border-slate-700/80 p-3">
            <div className="flex items-center justify-between mb-2">
              <span className="text-[10px] font-extrabold text-slate-300 uppercase tracking-wider flex items-center gap-1.5">
                <Camera className="w-3.5 h-3.5 text-inovar-yellow" /> Registro fotográfico do serviço
              </span>
              <button
                onClick={() => fileRef.current?.click()}
                className="px-2.5 py-1 bg-inovar-yellow text-inovar-navy rounded-lg text-[10px] font-bold hover:brightness-105 transition-all"
              >
                + Anexar fotos
              </button>
              <input ref={fileRef} type="file" accept="image/*" multiple className="hidden" onChange={(e) => anexarFotos(e.target.files)} />
            </div>
            {enviandoFoto && (
              <div className="flex items-center gap-2 text-[11px] text-slate-400 py-2">
                <Loader2 className="w-3.5 h-3.5 animate-spin" /> Enviando fotos...
              </div>
            )}
            {erroFoto && (
              <p className="text-[11px] text-amber-400 bg-amber-500/10 border border-amber-500/30 rounded-lg px-2 py-1.5 mt-1 mb-2">{erroFoto}</p>
            )}
            {fotos.length === 0 && !enviandoFoto ? (
              <p className="text-[11px] text-slate-500">Nenhuma foto anexada. Adicione fotos de antes/depois para o registro.</p>
            ) : (
              <div className="grid grid-cols-3 gap-2">
                {fotos.map((f) => (
                  <div key={f.caminho || f.nome} className="relative group">
                    <img
                      src={f.url}
                      alt="Foto do serviço"
                      onClick={() => setZoomFoto({ nome: f.nome, url: f.url })}
                      className="w-full h-24 object-cover rounded-lg border border-slate-700 cursor-zoom-in hover:brightness-110 transition-all"
                    />
                    <button
                      onClick={() => excluirFoto(f)}
                      title="Excluir foto"
                      className="absolute top-1 right-1 p-1 bg-slate-900/80 text-red-400 rounded-md opacity-0 group-hover:opacity-100 transition-opacity"
                    >
                      <X className="w-3 h-3" />
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Edicao completa da OS */}
          <div className="bg-slate-800/70 rounded-xl border border-slate-700/80 p-3">
            <button
              onClick={() => setEditandoOS(!editandoOS)}
              className="w-full flex items-center justify-between text-[11px] font-extrabold text-slate-300 uppercase tracking-wider"
            >
              <span>Editar Ordem de Serviço</span>
              <span className="text-inovar-yellow">{editandoOS ? '▲' : '▼'}</span>
            </button>
            {editandoOS && (
              <div className="mt-2.5 space-y-2.5">
                <div className="grid grid-cols-2 gap-2">
                  <div>
                    <label className="block text-[10px] text-slate-400 font-semibold mb-1">Data do serviço</label>
                    <input type="date" value={editData} onChange={(e) => setEditData(e.target.value)} className="w-full p-1.5 bg-slate-900 border border-slate-700 rounded-lg text-xs text-white" />
                  </div>
                  <div>
                    <label className="block text-[10px] text-slate-400 font-semibold mb-1">Valor (R$)</label>
                    <input type="number" min="0" step="10" value={editValor} onChange={(e) => setEditValor(e.target.value)} className="w-full p-1.5 bg-slate-900 border border-slate-700 rounded-lg text-xs text-white font-bold" />
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-2">
                  <div>
                    <label className="block text-[10px] text-slate-400 font-semibold mb-1">Garantia (dias)</label>
                    <select value={editGarantia} onChange={(e) => setEditGarantia(e.target.value)} className="w-full p-1.5 bg-slate-900 border border-slate-700 rounded-lg text-xs text-white">
                      {['30', '90', '180', '365'].map((d) => <option key={d} value={d}>{d} dias</option>)}
                    </select>
                  </div>
                  <div>
                    <label className="block text-[10px] text-slate-400 font-semibold mb-1">Status</label>
                    <select value={editStatus} onChange={(e) => setEditStatus(e.target.value)} className="w-full p-1.5 bg-slate-900 border border-slate-700 rounded-lg text-xs text-white">
                      <option value="AGENDADO">Agendado</option>
                      <option value="EM_ANDAMENTO">Em andamento</option>
                      <option value="CONCLUIDO">Concluído</option>
                      <option value="CANCELADO">Cancelado</option>
                    </select>
                  </div>
                </div>
                <button
                  onClick={salvarEdicaoOS}
                  disabled={salvandoOS}
                  className="w-full py-2 bg-inovar-yellow text-inovar-navy rounded-lg text-xs font-extrabold flex items-center justify-center gap-1.5 disabled:opacity-50"
                >
                  {salvandoOS ? <Loader2 className="w-4 h-4 animate-spin" /> : <CheckCircle className="w-4 h-4" />}
                  <span>Salvar no Banco</span>
                </button>
              </div>
            )}
          </div>

          {/* Action buttons */}
          <div className="space-y-2 pt-1">
            <button
              onClick={handleSendWhatsAppReceipt}
              className="w-full py-2.5 px-4 bg-emerald-600 hover:bg-emerald-500 text-white rounded-xl font-bold text-xs shadow-md shadow-emerald-900/20 flex items-center justify-center gap-2 transition-all active:scale-95"
            >
              <Share2 className="w-4 h-4" />
              <span>Enviar Comprovante via WhatsApp</span>
            </button>

            <button
              onClick={handleDownloadPdf}
              className="w-full py-2.5 px-4 bg-slate-900 hover:bg-slate-800 text-white rounded-xl font-semibold text-xs border border-slate-700 flex items-center justify-center gap-2 transition-all active:scale-95"
            >
              <Download className="w-4 h-4 text-sky-400" />
              <span>Baixar ou imprimir Ordem de Serviço</span>
            </button>

            {onDeleteMaintenance && (
              <button
                onClick={() => {
                  if (confirm('Deseja realmente excluir esta Ordem de Serviço do histórico?')) {
                    onDeleteMaintenance(maintenance.id);
                    onClose();
                  }
                }}
                className="w-full py-2 text-slate-400 hover:text-red-500 rounded-lg font-medium text-xs flex items-center justify-center gap-1.5 transition-colors"
              >
                <Trash2 className="w-3.5 h-3.5" />
                <span>Excluir Ordem de Serviço</span>
              </button>
            )}
          </div>
        </div>
      </div>

      {/* Visualizador de foto em tela cheia (anexo abre com um toque) */}
      {zoomFoto && (
        <div
          className="fixed inset-0 z-[70] bg-black/92 flex items-center justify-center p-4 cursor-zoom-out"
          onClick={() => setZoomFoto(null)}
        >
          <img src={zoomFoto.url} alt={zoomFoto.nome} className="max-w-full max-h-full rounded-xl shadow-2xl" />
          <button
            onClick={() => setZoomFoto(null)}
            className="absolute top-4 right-4 p-2 bg-slate-900/80 text-white rounded-full hover:bg-slate-700 transition-colors"
            title="Fechar"
          >
            <X className="w-5 h-5" />
          </button>
          <span className="absolute bottom-4 left-1/2 -translate-x-1/2 text-[11px] text-slate-300 bg-slate-900/80 px-3 py-1 rounded-full max-w-[80vw] truncate">
            {zoomFoto.nome} — toque para fechar
          </span>
        </div>
      )}
    </div>
  );
};
