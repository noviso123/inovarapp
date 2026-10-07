import React, { useState, useEffect, useRef } from 'react';
import {
  SupabaseCustomer,
  SupabaseAirConditioner,
  SupabaseService,
  SupabaseProfile,
  SupabaseService as SupabaseApi,
  supabase
} from '../services/supabase';
import {
  Wind,
  Plus,
  Calendar,
  Clock,
  ShieldCheck,
  CheckCircle2,
  AlertCircle,
  Phone,
  MapPin,
  FileText,
  Sparkles,
  ChevronRight,
  Loader2,
  Wrench,
  HelpCircle,
  Calculator,
  PenTool,
  KeyRound,
  UserCog,
  Bell,
  BadgeCheck,
  X
} from 'lucide-react';
import { CentralNotificacoes, NotificacoesStore } from './CentralNotificacoes';
import { format, parseISO, differenceInCalendarDays, addMonths, parseISO as pISO } from 'date-fns';
import { comprimirFotoPerfil, subirFotoPerfil, urlFotoPerfil, removerFotoPerfil as removerFotoPerfilSvc } from '../services/perfilFoto';
import { buscarCep, mascaraCep } from '../services/cep';

interface ClientePortalProps {
  profile: SupabaseProfile;
  onOpenAgendamento?: () => void;
  onRefresh?: () => void;
}

// Assinatura digital em canvas (dedo/mouse)
const SignaturePad: React.FC<{ onChange: (dataUrl: string | null) => void }> = ({ onChange }) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const drawing = useRef(false);
  const dirty = useRef(false);

  const pos = (e: any) => {
    const canvas = canvasRef.current!;
    const rect = canvas.getBoundingClientRect();
    const t = e.touches ? e.touches[0] : e;
    return { x: ((t.clientX - rect.left) / rect.width) * canvas.width, y: ((t.clientY - rect.top) / rect.height) * canvas.height };
  };

  const start = (e: any) => {
    e.preventDefault();
    drawing.current = true;
    const ctx = canvasRef.current!.getContext('2d')!;
    const p = pos(e);
    ctx.beginPath();
    ctx.moveTo(p.x, p.y);
  };
  const move = (e: any) => {
    if (!drawing.current) return;
    e.preventDefault();
    const ctx = canvasRef.current!.getContext('2d')!;
    const p = pos(e);
    ctx.lineWidth = 2.5;
    ctx.lineCap = 'round';
    ctx.strokeStyle = '#0B2D4E';
    ctx.lineTo(p.x, p.y);
    ctx.stroke();
    dirty.current = true;
  };
  const end = () => {
    drawing.current = false;
    if (dirty.current && canvasRef.current) onChange(canvasRef.current.toDataURL('image/png'));
  };
  const clear = () => {
    const canvas = canvasRef.current!;
    canvas.getContext('2d')!.clearRect(0, 0, canvas.width, canvas.height);
    dirty.current = false;
    onChange(null);
  };

  return (
    <div>
      <canvas
        ref={canvasRef}
        width={520}
        height={200}
        className="w-full touch-none bg-white border-2 border-dashed border-slate-300 rounded-xl cursor-crosshair"
        onMouseDown={start}
        onMouseMove={move}
        onMouseUp={end}
        onMouseLeave={end}
        onTouchStart={start}
        onTouchMove={move}
        onTouchEnd={end}
      />
      <div className="flex items-center justify-between mt-1.5">
        <span className="text-[10px] text-slate-400">Assine com o dedo ou o mouse</span>
        <button type="button" onClick={clear} className="text-[11px] font-bold text-red-400 hover:text-red-300">Limpar</button>
      </div>
    </div>
  );
};

export const ClientePortal: React.FC<ClientePortalProps> = ({ profile }) => {
  const [customer, setCustomer] = useState<SupabaseCustomer | null>(null);
  const [appliances, setAppliances] = useState<SupabaseAirConditioner[]>([]);
  const [services, setServices] = useState<SupabaseService[]>([]);
  const [budgets, setBudgets] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  // Foto de perfil (storage privado, URL assinada)
  const [fotoPerfil, setFotoPerfil] = useState<string | null>(null);
  const fotoInputRef = useRef<HTMLInputElement>(null);
  const [enviandoFoto, setEnviandoFoto] = useState(false);

  useEffect(() => {
    urlFotoPerfil().then(setFotoPerfil);
  }, []);

  const trocarFoto = async (file?: File | null) => {
    if (!file) return;
    setEnviandoFoto(true);
    try {
      const base64 = await comprimirFotoPerfil(file);
      const anterior = fotoPerfil;
      // Prévia imediata: a foto aparece na hora, mesmo antes da resposta da rede.
      setFotoPerfil(base64);
      const r = await subirFotoPerfil(base64);
      if (r.ok) {
        setFotoPerfil(await urlFotoPerfil());
        setFeedbackMsg({ type: 'success', text: 'Foto de perfil atualizada!' });
      } else {
        setFotoPerfil(anterior);
        setFeedbackMsg({ type: 'error', text: r.erro || 'Não foi possível salvar a foto.' });
      }
    } catch {
      setFeedbackMsg({ type: 'error', text: 'Não foi possível preparar esta imagem. Tente uma foto JPG, PNG ou WebP.' });
    } finally {
      setEnviandoFoto(false);
      if (fotoInputRef.current) fotoInputRef.current.value = '';
    }
  };

  const removerFoto = async () => {
    if (!confirm('Remover sua foto de perfil?')) return;
    await removerFotoPerfilSvc();
    setFotoPerfil(null);
  };

  // Modal states
  const [showNovoAparelho, setShowNovoAparelho] = useState(false);
  const [showSolicitarServico, setShowSolicitarServico] = useState(false);
  const [showTrocarSenha, setShowTrocarSenha] = useState(false);
  const [showCompletarCadastro, setShowCompletarCadastro] = useState(false);
  const [feedbackMsg, setFeedbackMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  // Orçamento com assinatura
  const [budgetToSign, setBudgetToSign] = useState<any | null>(null);
  const [assinatura, setAssinatura] = useState<string | null>(null);
  const [enviandoResposta, setEnviandoResposta] = useState(false);

  // Notificações
  const prevStatuses = useRef<Map<string, string>>(new Map());

  // WhatsApp/telefone da empresa (Configurações do técnico) — sem número fixo no código
  const [empresaWhats, setEmpresaWhats] = useState('');
  useEffect(() => {
    supabase.auth.getSession().then(({ data }) => {
      if (!data.session) return;
      fetch('/api/configuracoes', { headers: { Authorization: `Bearer ${data.session.access_token}` } })
        .then((r) => (r.ok ? r.json() : null))
        .then((j) => {
          const tel = String(j?.config?.phone || '').replace(/\D/g, '');
          if (tel.length >= 10) setEmpresaWhats(tel.startsWith('55') ? tel : '55' + tel);
        })
        .catch(() => {});
    });
  }, []);
  const notify = (title: string, body: string) => {
    NotificacoesStore.registrar(title, body);
    setFeedbackMsg({ type: 'success', text: `${title} — ${body}` });
    try {
      if (typeof Notification !== 'undefined' && Notification.permission === 'granted') {
        new Notification(title, { body, icon: undefined });
      }
    } catch { /* ignore */ }
  };

  // Novo Aparelho form
  const [appMarca, setAppMarca] = useState('LG');
  const [appModelo, setAppModelo] = useState('');
  const [appBtus, setAppBtus] = useState<number>(12000);
  const [appTipo, setAppTipo] = useState('Split Hi-Wall');
  const [appAmbiente, setAppAmbiente] = useState('Sala');

  // Solicitar Serviço form
  const [selectedApplianceId, setSelectedApplianceId] = useState<string>('');
  // espelho do estado para uso dentro de callbacks de polling (evita stale closure)
  const selAparelhoRef = useRef<string>('');
  useEffect(() => { selAparelhoRef.current = selectedApplianceId; }, [selectedApplianceId]);
  const [servTipo, setServTipo] = useState<SupabaseService['tipo']>('LIMPEZA');
  const [servProblema, setServProblema] = useState('');
  const [servData, setServData] = useState('');
  const [servObs, setServObs] = useState('');
  const [submittingServ, setSubmittingServ] = useState(false);

  const loadCustomerData = async (silencioso = false) => {
    if (!silencioso) setLoading(true);
    try {
      const cust = await SupabaseApi.fetchCustomerByProfileId(profile.id);
      if (cust) {
        setCustomer(cust);
        const [apps, servs, budgs] = await Promise.all([
          SupabaseApi.fetchCustomerAppliances(cust.id),
          SupabaseApi.fetchCustomerServices(cust.id),
          SupabaseApi.fetchBudgets()
        ]);
        setAppliances(apps);
        const meusOrcamentos = ((budgs as any)?.data || []).filter((b: any) => b.cliente_id === cust.id);
        setBudgets(meusOrcamentos);
        setServices(servs);
        // ref evita stale closure: sem ele, o polling de 45s captura o valor antigo
        // de selectedApplianceId ('') e desfaz a escolha do usuário
        if (apps.length > 0 && !selAparelhoRef.current) {
          setSelectedApplianceId(apps[0].id);
        }

        // Detecta mudanças de status desde a última leitura → notifica
        const atual = new Map<string, string>();
        servs.forEach((s) => atual.set('svc:' + s.id, s.status));
        meusOrcamentos.forEach((b: any) => atual.set('bud:' + b.id, b.status));
        if (prevStatuses.current.size > 0) {
          for (const [key, st] of atual) {
            const before = prevStatuses.current.get(key);
            if (before && before !== st) {
              if (key.startsWith('svc:')) {
                const svc = servs.find((s) => 'svc:' + s.id === key);
                const labels: Record<string, string> = {
                  AGENDADO: '✅ Serviço AGENDADO pela Inovar',
                  EM_ANDAMENTO: '🔧 Serviço EM EXECUÇÃO hoje',
                  CONCLUIDO: '🏁 Serviço CONCLUÍDO — garantia ativa!',
                  CANCELADO: '⚠️ Serviço cancelado'
                };
                notify(labels[st] || 'Atualização de serviço', (svc?.tipo || 'Serviço').replace('_', ' '));
              } else if (key.startsWith('bud:')) {
                notify(
                  st === 'APROVADO' ? '✅ Orçamento aprovado' : st === 'RECUSADO' ? 'Orçamento recusado' : 'Orçamento atualizado',
                  'Acompanhe os detalhes no seu portal.'
                );
              }
            }
          }
        }
        prevStatuses.current = atual;
      }
    } catch (err) {
      console.error('Erro ao carregar dados do cliente:', err);
    } finally {
      if (!silencioso) setLoading(false);
    }
  };

  useEffect(() => {
    loadCustomerData();
    // Polling: mantém o cliente atualizado com o andamento em tempo real
    const intervalo = setInterval(() => loadCustomerData(true), 45000);
    return () => clearInterval(intervalo);
  }, [profile.id]);

  // Resposta do cliente ao orçamento (aprovar com assinatura / recusar)
  const responderOrcamento = async (orcamentoId: string, acao: 'APROVAR' | 'RECUSAR', dataUrl: string | null) => {
    setEnviandoResposta(true);
    try {
      const { data: sessionData } = await supabase.auth.getSession();
      const r = await fetch('/api/orcamento-resposta', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${sessionData?.session?.access_token || ''}`
        },
        body: JSON.stringify({ orcamento_id: orcamentoId, acao, assinatura: dataUrl })
      });
      const resp = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(resp.error || 'Falha ao enviar resposta');

      setFeedbackMsg({ type: 'success', text: resp.mensagem || 'Resposta enviada!' });
      setBudgetToSign(null);
      setAssinatura(null);
      loadCustomerData(true);
    } catch (err: any) {
      setFeedbackMsg({ type: 'error', text: err.message || 'Erro ao responder orçamento.' });
    } finally {
      setEnviandoResposta(false);
    }
  };

  const trocarSenha = async (novaSenha: string, setErr: (s: string) => void) => {
    try {
      const { error } = await supabase.auth.updateUser({ password: novaSenha });
      if (error) throw error;
      setShowTrocarSenha(false);
      setFeedbackMsg({ type: 'success', text: 'Senha alterada com sucesso! Use-a no próximo login.' });
    } catch (err: any) {
      setErr(err.message || 'Não foi possível alterar a senha.');
    }
  };

  const handleCriarAparelho = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!customer) return;

    try {
      const { data, error } = await SupabaseApi.createAirConditioner({
        cliente_id: customer.id,
        marca: appMarca,
        modelo: appModelo || undefined,
        btus: Number(appBtus),
        tipo: appTipo,
        ambiente: appAmbiente,
        ultima_manutencao: new Date().toISOString().split('T')[0]
      });

      if (error) throw error;

      setFeedbackMsg({ type: 'success', text: 'Aparelho cadastrado com sucesso!' });
      setShowNovoAparelho(false);
      setAppModelo('');
      loadCustomerData();
    } catch (err: any) {
      setFeedbackMsg({ type: 'error', text: err.message || 'Erro ao cadastrar aparelho.' });
    }
  };

  const handleSolicitarServico = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!customer) return;
    setSubmittingServ(true);

    try {
      const { data, error } = await SupabaseApi.createService({
        cliente_id: customer.id,
        aparelho_id: selectedApplianceId || undefined,
        tipo: servTipo,
        problema: servProblema || undefined,
        data_agendamento: servData || undefined,
        status: 'PENDENTE',
        observacoes: servObs || undefined
      });

      if (error) throw error;

      setFeedbackMsg({
        type: 'success',
        text: 'Solicitação de serviço enviada com sucesso! Nossa equipe entrará em contato para confirmar.'
      });
      setShowSolicitarServico(false);
      setServProblema('');
      setServObs('');
      loadCustomerData();
    } catch (err: any) {
      setFeedbackMsg({ type: 'error', text: err.message || 'Erro ao enviar solicitação.' });
    } finally {
      setSubmittingServ(false);
    }
  };

  // Alerta automático: aparelhos com retorno vencido/vencendo (ciclo de 6 meses)
  const aparelhosNoPrazo = appliances
    .map((app) => {
      const base = app.ultima_manutencao;
      if (!base) return null;
      const retorno = addMonths(new Date(base + 'T00:00:00'), 6);
      const dias = differenceInCalendarDays(retorno, new Date());
      return dias <= 7 ? { app, retorno, dias } : null;
    })
    .filter(Boolean) as { app: SupabaseAirConditioner; retorno: Date; dias: number }[];

  if (loading) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-slate-400 gap-3">
        <Loader2 className="w-8 h-8 text-inovar-yellow animate-spin" />
        <span className="text-sm font-medium">Carregando seu painel do cliente...</span>
      </div>
    );
  }

  return (
    <div className="operational-screen space-y-5 animate-in fade-in duration-300 text-slate-800">
      {/* Toast Feedback */}
      {feedbackMsg && (
        <div
          className={`p-3.5 rounded-xl text-xs flex items-center justify-between shadow-lg ${
            feedbackMsg.type === 'success'
              ? 'bg-emerald-950/80 border border-emerald-700 text-emerald-200'
              : 'bg-red-950/80 border border-red-700 text-red-200'
          }`}
        >
          <div className="flex items-center gap-2">
            {feedbackMsg.type === 'success' ? (
              <CheckCircle2 className="w-4 h-4 text-emerald-400 shrink-0" />
            ) : (
              <AlertCircle className="w-4 h-4 text-red-400 shrink-0" />
            )}
            <span>{feedbackMsg.text}</span>
          </div>
          <button onClick={() => setFeedbackMsg(null)} className="text-slate-400 hover:text-white font-bold ml-2">
            ✕
          </button>
        </div>
      )}

      {/* ALERTA AUTOMÁTICO de ciclo de manutenção */}
      {aparelhosNoPrazo.length > 0 && (
        <div className="p-4 rounded-2xl bg-amber-950/50 border border-amber-700/70 shadow-lg">
          <div className="flex items-start gap-3">
            <div className="p-2 bg-amber-500/20 text-amber-300 rounded-xl shrink-0">
              <Clock className="w-5 h-5" />
            </div>
            <div className="flex-1 min-w-0">
              <h4 className="text-sm font-extrabold text-amber-200">
                Hora da manutenção preventiva! ❄️
              </h4>
              <p className="text-xs text-amber-100/80 mt-1 leading-relaxed">
                {aparelhosNoPrazo.length === 1
                  ? `O ciclo de limpeza de ar do seu ${aparelhosNoPrazo[0].app.marca} (${aparelhosNoPrazo[0].app.ambiente || 'aparelho'}) `
                  : `Os ciclos de limpeza de ar de ${aparelhosNoPrazo.length} aparelhos `}
                {aparelhosNoPrazo.length === 1 && (
                  <>
                    {aparelhosNoPrazo[0].dias < 0
                      ? `venceu há ${Math.abs(aparelhosNoPrazo[0].dias)} dia(s) (retornava em ${format(aparelhosNoPrazo[0].retorno, 'dd/MM')})`
                      : `vence em ${aparelhosNoPrazo[0].dias} dia(s) (${format(aparelhosNoPrazo[0].retorno, 'dd/MM')})`}
                    . Manter o ciclo evita fungos, mau cheiro e maior consumo de energia.
                  </>
                )}
                {aparelhosNoPrazo.length > 1 && 'estão no prazo de limpeza de ar (ciclo de 6 meses).'}
              </p>
            </div>
            <button
              onClick={() => setShowSolicitarServico(true)}
              className="shrink-0 px-3 py-2 bg-inovar-yellow text-inovar-navy font-bold rounded-xl text-xs hover:brightness-105 active:scale-95 transition-all"
            >
              Agendar Agora
            </button>
          </div>
        </div>
      )}

      {/* Customer Greeting Banner */}
      <div className="relative overflow-hidden rounded-2xl bg-gradient-to-br from-inovar-navy via-slate-900 to-slate-950 border border-slate-800 p-5 shadow-xl">
        <div className="absolute top-0 right-0 w-64 h-64 bg-inovar-yellow/10 rounded-full blur-3xl pointer-events-none -mr-20 -mt-20"></div>
        <div className="relative z-10 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div className="flex items-center gap-3.5">
            {/* Foto de perfil — cliente troca/remover a qualquer momento */}
            <div className="relative shrink-0 group">
              <button
                onClick={() => fotoInputRef.current?.click()}
                title="Trocar foto de perfil"
                className="w-16 h-16 rounded-2xl overflow-hidden border-2 border-inovar-yellow/50 bg-slate-800 flex items-center justify-center hover:border-inovar-yellow transition-colors"
              >
                {enviandoFoto ? (
                  <Loader2 className="w-6 h-6 text-inovar-yellow animate-spin" />
                ) : fotoPerfil ? (
                  <img key={fotoPerfil} src={fotoPerfil} alt="Foto de perfil" className="w-full h-full object-cover" />
                ) : (
                  <span className="text-xl font-black text-inovar-yellow">
                    {profile.nome?.split(' ')[0]?.[0]?.toUpperCase() || '?'}
                  </span>
                )}
              </button>
              {fotoPerfil && (
                <button
                  onClick={removerFoto}
                  title="Remover foto"
                  className="absolute -top-1.5 -right-1.5 p-1 bg-slate-900 border border-red-500/50 text-red-400 rounded-full hover:bg-red-950 transition-colors"
                >
                  <X className="w-3 h-3" />
                </button>
              )}
              <input
                ref={fotoInputRef}
                type="file"
                accept="image/*"
                className="hidden"
                onChange={(e) => trocarFoto(e.target.files?.[0])}
              />
            </div>
            <div>
              <div className="flex items-center gap-2 mb-1">
                <span className="px-2 py-0.5 rounded-full bg-inovar-yellow/20 text-inovar-yellow border border-inovar-yellow/30 text-[10px] font-extrabold uppercase tracking-wide">
                  Portal do Cliente
                </span>
                <span className="text-xs text-slate-400">Inovar Refrigeração</span>
              </div>
              <h2 className="text-xl sm:text-2xl font-black text-white tracking-tight">
                Olá, {profile.nome.split(' ')[0]}!
              </h2>
              <p className="text-xs text-slate-300 mt-1 max-w-lg leading-relaxed">
                Acompanhe a saúde dos seus aparelhos de ar-condicionado, solicite limpeza de ar e consulte suas garantias.
              </p>
            </div>
          </div>

          <div className="flex flex-wrap gap-2 items-center">
            <button
              onClick={() => setShowSolicitarServico(true)}
              className="px-4 py-2.5 bg-inovar-yellow hover:brightness-105 active:scale-95 text-inovar-navy font-bold rounded-xl text-xs flex items-center gap-2 shadow-lg transition-all"
            >
              <Calendar className="w-4 h-4" />
              <span>Solicitar Atendimento</span>
            </button>
            <a
              href={`https://wa.me/${empresaWhats || '5527999999999'}?text=Ol%C3%A1%20sou%20cliente%20da%20Inovar%20e%20gostaria%20de%20tirar%20uma%20d%C3%BAvida`}
              target="_blank"
              rel="noopener noreferrer"
              className="px-3 py-2.5 bg-emerald-600 hover:bg-emerald-500 text-white font-semibold rounded-xl text-xs flex items-center gap-1.5 shadow transition-all"
            >
              <Phone className="w-3.5 h-3.5" />
              <span>WhatsApp Inovar</span>
            </a>
            <button
              onClick={() => setShowCompletarCadastro(true)}
              className="px-3 py-2.5 bg-slate-800/90 hover:bg-slate-700 text-slate-200 border border-slate-700 rounded-xl text-xs font-semibold flex items-center gap-1.5 transition-all"
            >
              <UserCog className="w-3.5 h-3.5" />
              <span>Meus Dados</span>
            </button>
            <button
              onClick={() => setShowTrocarSenha(true)}
              className="px-3 py-2.5 bg-slate-800/90 hover:bg-slate-700 text-slate-200 border border-slate-700 rounded-xl text-xs font-semibold flex items-center gap-1.5 transition-all"
            >
              <KeyRound className="w-3.5 h-3.5" />
              <span>Alterar Senha</span>
            </button>
            <CentralNotificacoes />
          </div>
        </div>
      </div>

      {/* Meus Aparelhos Section */}
      <div className="space-y-3">
        <div className="flex items-center justify-between px-1">
          <div className="flex items-center gap-2">
            <div className="p-1.5 bg-sky-500/20 text-sky-400 rounded-lg">
              <Wind className="w-4 h-4" />
            </div>
            <div>
              <h3 className="text-sm font-extrabold text-white">Meus Aparelhos</h3>
              <p className="text-[11px] text-slate-400">Seus equipamentos monitorados pela Inovar</p>
            </div>
          </div>

          <button
            onClick={() => setShowNovoAparelho(true)}
            className="flex items-center gap-1.5 px-3 py-1.5 bg-slate-800 hover:bg-slate-700 text-inovar-yellow border border-slate-700 rounded-lg text-xs font-semibold transition-colors"
          >
            <Plus className="w-3.5 h-3.5" />
            <span>Adicionar Aparelho</span>
          </button>
        </div>

        {appliances.length === 0 ? (
          <div className="p-8 text-center bg-slate-900/60 border border-slate-800/80 rounded-2xl">
            <Wind className="w-10 h-10 text-slate-600 mx-auto mb-2" />
            <h4 className="text-sm font-bold text-slate-300">Nenhum aparelho cadastrado</h4>
            <p className="text-xs text-slate-400 max-w-sm mx-auto mt-1 mb-4">
              Cadastre seu ar-condicionado para ter histórico de manutenções, alerta de limpeza de ar e controle de garantia.
            </p>
            <button
              onClick={() => setShowNovoAparelho(true)}
              className="px-4 py-2 bg-inovar-yellow text-inovar-navy font-bold rounded-xl text-xs"
            >
              Cadastrar Meu Primeiro Ar-Condicionado
            </button>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {appliances.map((app) => (
              <div
                key={app.id}
                className="bg-slate-900/90 border border-slate-800 hover:border-slate-700 rounded-2xl p-4 transition-all shadow-md flex flex-col justify-between"
              >
                <div>
                  <div className="flex items-start justify-between">
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-extrabold text-slate-900">
                          {app.marca} {app.modelo || ''}
                        </span>
                        <span className="px-2 py-0.5 rounded-full bg-slate-800 border border-slate-700 text-[10px] font-semibold text-slate-300">
                          {app.btus ? `${app.btus.toLocaleString('pt-BR')} BTUs` : 'Split'}
                        </span>
                      </div>
                      <p className="text-xs text-slate-400 mt-0.5">
                        Ambiente: <span className="text-slate-200 font-medium">{app.ambiente || 'Não informado'}</span>
                        {app.tipo && ` • ${app.tipo}`}
                      </p>
                    </div>
                    <div className="p-2 bg-slate-800 rounded-xl text-sky-400">
                      <Wind className="w-4 h-4" />
                    </div>
                  </div>

                  <div className="mt-3 pt-3 border-t border-slate-800/80 flex items-center justify-between text-xs">
                    <div className="text-slate-400">
                      <span>Última Manutenção: </span>
                      <span className="font-semibold text-slate-800">
                        {app.ultima_manutencao
                          ? format(parseISO(app.ultima_manutencao), 'dd/MM/yyyy')
                          : 'Pendente'}
                      </span>
                    </div>

                    <span className="px-2 py-0.5 bg-emerald-950/60 border border-emerald-800/80 text-emerald-400 rounded-full text-[10px] font-semibold">
                      Monitorado
                    </span>
                  </div>
                </div>

                <div className="mt-3 pt-2 flex items-center justify-end gap-2">
                  <button
                    onClick={() => {
                      setSelectedApplianceId(app.id);
                      setShowSolicitarServico(true);
                    }}
                    className="px-3 py-1.5 bg-slate-800 hover:bg-slate-700 text-sky-300 hover:text-white rounded-lg text-xs font-semibold transition-colors flex items-center gap-1"
                  >
                    <Wrench className="w-3 h-3" />
                    <span>Pedir Limpeza de Ar</span>
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Solicitações & Histórico de Ordens de Serviço */}
      <div className="space-y-3 pt-2">
        <div className="flex items-center gap-2 px-1">
          <div className="p-1.5 bg-amber-500/20 text-inovar-yellow rounded-lg">
            <FileText className="w-4 h-4" />
          </div>
          <div>
            <h3 className="text-sm font-extrabold text-white">Minhas Ordens de Serviço & Agendamentos</h3>
            <p className="text-[11px] text-slate-400">Status em tempo real das manutenções realizadas</p>
          </div>
        </div>

        {services.length === 0 ? (
          <div className="p-6 text-center bg-slate-900/40 border border-slate-800/60 rounded-2xl text-slate-400 text-xs">
            Você ainda não possui atendimentos registrados. Quando uma limpeza de ar ou conserto for agendado ou concluído, ele aparecerá aqui com comprovante e termo de garantia.
          </div>
        ) : (
          <div className="space-y-2">
            {services.map((serv) => {
              const statusColors: Record<string, { bg: string; text: string; label: string }> = {
                PENDENTE: { bg: 'bg-amber-100 border-amber-200', text: 'text-amber-800', label: 'Aguardando confirmação' },
                AGENDADO: { bg: 'bg-blue-100 border-blue-200', text: 'text-blue-800', label: 'Agendado' },
                EM_ANDAMENTO: { bg: 'bg-purple-100 border-purple-200', text: 'text-purple-800', label: 'Em execução' },
                CONCLUIDO: { bg: 'bg-emerald-100 border-emerald-200', text: 'text-emerald-800', label: 'Concluído • Garantia ativa' },
                CANCELADO: { bg: 'bg-red-100 border-red-200', text: 'text-red-800', label: 'Cancelado' }
              };

              const conf = statusColors[serv.status] || { bg: 'bg-slate-100', text: 'text-slate-700', label: serv.status };

              return (
                <div
                  key={serv.id}
                  className="bg-slate-900/90 border border-slate-800 rounded-xl p-3.5 flex flex-col sm:flex-row sm:items-center justify-between gap-3 shadow-sm"
                >
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      <span className="text-xs font-bold text-slate-900">
                        {serv.tipo.replace('_', ' ')}
                      </span>
                      <span className={`px-2 py-0.5 rounded-full text-[10px] font-bold border ${conf.bg} ${conf.text}`}>
                        {conf.label}
                      </span>
                    </div>

                    <p className="text-xs text-slate-400">
                      {serv.data_agendamento
                        ? `Data marcada: ${format(parseISO(serv.data_agendamento), 'dd/MM/yyyy')}`
                        : `Solicitado em: ${serv.data_solicitacao ? format(parseISO(serv.data_solicitacao), 'dd/MM/yyyy') : 'Hoje'}`}
                      {serv.problema && ` • "${serv.problema}"`}
                    </p>
                  </div>

                  <div className="flex items-center gap-2 justify-end">
                    {serv.valor && (
                      <span className="text-xs font-bold text-emerald-400 px-2 py-1 bg-emerald-950/40 rounded-lg border border-emerald-800/40">
                        R$ {Number(serv.valor).toFixed(2)}
                      </span>
                    )}

                    {serv.status === 'CONCLUIDO' && (
                      <div className="flex items-center gap-1 text-[11px] text-emerald-400 font-semibold bg-emerald-950/60 px-2.5 py-1 rounded-lg border border-emerald-800/80">
                        <ShieldCheck className="w-3.5 h-3.5" />
                        <span>Garantia 90 Dias</span>
                      </div>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Meus Orçamentos (propostas da Inovar) */}
      <div className="space-y-3 pt-2">
        <div className="flex items-center gap-2 px-1">
          <div className="p-1.5 bg-inovar-yellow/20 text-inovar-yellow rounded-lg">
            <Calculator className="w-4 h-4" />
          </div>
          <div>
            <h3 className="text-sm font-extrabold text-white">Meus Orçamentos</h3>
            <p className="text-[11px] text-slate-400">Propostas enviadas pela Inovar — aprove com sua assinatura</p>
          </div>
        </div>

        {budgets.length === 0 ? (
          <div className="p-5 text-center bg-slate-900/40 border border-slate-800/60 rounded-2xl text-slate-400 text-xs">
            Nenhum orçamento recebido ainda. Quando o técnico enviar uma proposta, ela aparece aqui para você aprovar.
          </div>
        ) : (
          <div className="space-y-2">
            {budgets.map((b) => {
              const st = b.status;
              let meta: any = {};
              try { meta = JSON.parse(b.descricao || '{}'); } catch { meta = {}; }
              // "Assinado" somente quando a assinatura tem data registrada (assinatura_em não-nulo)
              const badge =
                st === 'APROVADO'
                  ? { cls: 'bg-emerald-100 border-emerald-200 text-emerald-800', label: 'Aprovado' + (meta.assinatura_em ? ' • Assinado' : '') }
                  : st === 'RECUSADO'
                    ? { cls: 'bg-red-100 border-red-200 text-red-800', label: 'Recusado' }
                    : st === 'EXPIRADO'
                      ? { cls: 'bg-slate-100 border-slate-200 text-slate-700', label: 'Expirado' }
                      : { cls: 'bg-amber-100 border-amber-200 text-amber-800', label: 'Aguardando sua resposta' };
              const qtdItens = Array.isArray(meta.items) ? meta.items.length : 0;

              return (
                <div key={b.id} className="bg-slate-900/90 border border-slate-800 rounded-xl p-3.5 shadow-sm">
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2 flex-wrap">
                        <span className="text-xs font-bold text-slate-900">Proposta #{b.numero}</span>
                        <span className={`px-2 py-0.5 rounded-full text-[10px] font-bold border ${badge.cls}`}>{badge.label}</span>
                      </div>
                      <p className="text-[11px] text-slate-400 mt-1">
                        Emitida em {b.data ? format(parseISO(String(b.data).slice(0, 10) + 'T00:00:00'), 'dd/MM/yyyy') : '—'}
                        {b.validade ? ` • válida até ${format(parseISO(String(b.validade).slice(0, 10) + 'T00:00:00'), 'dd/MM/yyyy')}` : ''}
                        {qtdItens ? ` • ${qtdItens} item(ns)` : ''}
                      </p>
                      {b.tipo_servico && <p className="text-[11px] text-slate-300 mt-0.5 truncate">{b.tipo_servico}</p>}
                      {meta.assinatura_em && (
                        <p className="text-[10px] text-emerald-400 mt-1 flex items-center gap-1">
                          <BadgeCheck className="w-3 h-3" /> Assinado digitalmente em {format(parseISO(meta.assinatura_em), 'dd/MM/yyyy HH:mm')}
                        </p>
                      )}
                    </div>
                    <div className="text-right shrink-0">
                      <span className="text-sm font-extrabold text-inovar-yellow block">R$ {Number(b.valor_total).toFixed(2)}</span>
                    </div>
                  </div>

                  {(st === 'ENVIADO' || st === 'RASCUNHO') && (
                    <div className="flex gap-2 mt-3 pt-3 border-t border-slate-800/80">
                      <button
                        onClick={() => { setBudgetToSign(b); setAssinatura(null); }}
                        className="flex-1 py-2 bg-emerald-600 hover:bg-emerald-500 active:scale-[0.99] text-white font-bold rounded-lg text-xs flex items-center justify-center gap-1.5 transition-all"
                      >
                        <PenTool className="w-3.5 h-3.5" />
                        <span>Aprovar e Assinar</span>
                      </button>
                      <button
                        onClick={() => {
                          if (window.confirm('Recusar a proposta #' + b.numero + '? A Inovar será notificada.')) {
                            responderOrcamento(b.id, 'RECUSAR', null);
                          }
                        }}
                        className="px-3 py-2 bg-slate-800 hover:bg-red-950/60 border border-slate-700 text-slate-300 hover:text-red-300 rounded-lg text-xs font-semibold transition-colors"
                      >
                        Recusar
                      </button>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Modal: Adicionar Aparelho */}
      {showNovoAparelho && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm animate-in fade-in duration-200">
          <div className="bg-slate-900 border border-slate-700 rounded-2xl w-full max-w-md overflow-hidden shadow-2xl text-slate-100 p-5">
            <div className="flex items-center justify-between mb-4 border-b border-slate-800 pb-3">
              <h3 className="text-base font-extrabold text-white flex items-center gap-2">
                <Wind className="w-5 h-5 text-inovar-yellow" />
                <span>Cadastrar Ar-Condicionado</span>
              </h3>
              <button onClick={() => setShowNovoAparelho(false)} className="text-slate-400 hover:text-white">✕</button>
            </div>

            <form onSubmit={handleCriarAparelho} className="space-y-3">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Marca</label>
                <select
                  value={appMarca}
                  onChange={(e) => setAppMarca(e.target.value)}
                  className="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-xl text-sm text-white"
                >
                  {['LG', 'Gree', 'Midea', 'Samsung', 'Daikin', 'Carrier', 'Elgin', 'Fujitsu', 'Consul', 'Electrolux', 'Philco', 'Outro'].map((m) => (
                    <option key={m} value={m}>{m}</option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Modelo (Opcional)</label>
                <input
                  type="text"
                  value={appModelo}
                  onChange={(e) => setAppModelo(e.target.value)}
                  placeholder="Ex: Dual Inverter Voice"
                  className="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-xl text-sm text-white placeholder-slate-500"
                />
              </div>

              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">Capacidade (BTUs)</label>
                  <select
                    value={appBtus}
                    onChange={(e) => setAppBtus(Number(e.target.value))}
                    className="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-xl text-sm text-white"
                  >
                    {[9000, 12000, 18000, 24000, 30000, 36000, 48000, 60000].map((b) => (
                      <option key={b} value={b}>{b.toLocaleString('pt-BR')} BTUs</option>
                    ))}
                  </select>
                </div>

                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">Tipo</label>
                  <select
                    value={appTipo}
                    onChange={(e) => setAppTipo(e.target.value)}
                    className="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-xl text-sm text-white"
                  >
                    {['Split Hi-Wall', 'Inverter', 'Cassete', 'Piso Teto', 'Janela', 'Multi Split', 'Portátil'].map((t) => (
                      <option key={t} value={t}>{t}</option>
                    ))}
                  </select>
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Ambiente Instalado</label>
                <input
                  type="text"
                  value={appAmbiente}
                  onChange={(e) => setAppAmbiente(e.target.value)}
                  placeholder="Ex: Quarto Casal, Sala de Estar, Escritório"
                  className="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-xl text-sm text-white placeholder-slate-500"
                />
              </div>

              <button
                type="submit"
                className="w-full mt-3 py-2.5 bg-inovar-yellow hover:brightness-105 text-inovar-navy font-bold rounded-xl text-sm shadow"
              >
                Salvar Aparelho
              </button>
            </form>
          </div>
        </div>
      )}

      {/* Modal: Aprovar Orçamento com Assinatura */}
      {budgetToSign && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-3 bg-black/80 backdrop-blur-sm animate-in fade-in duration-200 overflow-y-auto">
          <div className="bg-slate-900 border border-slate-700 rounded-2xl w-full max-w-md shadow-2xl text-slate-100 p-5 my-4">
            <div className="flex items-center justify-between mb-3 border-b border-slate-800 pb-3">
              <h3 className="text-base font-extrabold text-white flex items-center gap-2">
                <PenTool className="w-5 h-5 text-emerald-400" />
                <span>Aprovar Proposta #{budgetToSign.numero}</span>
              </h3>
              <button onClick={() => setBudgetToSign(null)} className="text-slate-400 hover:text-white">✕</button>
            </div>

            <div className="space-y-3 text-xs">
              <div className="p-3 bg-slate-800/60 rounded-xl space-y-1">
                <div className="flex justify-between"><span className="text-slate-400">Valor total:</span><b className="text-inovar-yellow">R$ {Number(budgetToSign.valor_total).toFixed(2)}</b></div>
                {budgetToSign.condicoes && <div className="flex justify-between gap-2"><span className="text-slate-400">Pagamento:</span><span className="text-right text-slate-200">{budgetToSign.condicoes}</span></div>}
              </div>

              <div>
                <label className="block text-[11px] font-bold text-slate-300 mb-1.5 uppercase tracking-wide">Assinatura Digital do Cliente</label>
                <SignaturePad onChange={setAssinatura} />
              </div>

              <div className="p-2.5 bg-slate-800/40 rounded-xl text-[10px] text-slate-400 flex items-start gap-2">
                <ShieldCheck className="w-4 h-4 text-inovar-yellow shrink-0" />
                <span>Ao assinar, você aprova a proposta e a Inovar recebe a notificação imediatamente para agendar o serviço. A assinatura fica registrada no orçamento.</span>
              </div>

              <div className="flex gap-2 pt-1">
                <button onClick={() => setBudgetToSign(null)} className="flex-1 py-2.5 bg-slate-800 text-slate-300 rounded-xl font-semibold">Voltar</button>
                <button
                  disabled={!assinatura || enviandoResposta}
                  onClick={() => responderOrcamento(budgetToSign.id, 'APROVAR', assinatura)}
                  className="flex-1 py-2.5 bg-emerald-600 hover:bg-emerald-500 text-white font-bold rounded-xl flex items-center justify-center gap-1.5 disabled:opacity-40 disabled:cursor-not-allowed"
                >
                  {enviandoResposta ? <Loader2 className="w-4 h-4 animate-spin" /> : <CheckCircle2 className="w-4 h-4" />}
                  <span>{enviandoResposta ? 'Enviando...' : 'Confirmar e Assinar'}</span>
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Modal: Alterar Senha */}
      {showTrocarSenha && (
        <TrocarSenhaModal
          onClose={() => setShowTrocarSenha(false)}
          onConfirm={trocarSenha}
        />
      )}

      {/* Modal: Completar Cadastro */}
      {showCompletarCadastro && customer && (
        <CompletarCadastroModal
          customer={customer}
          onClose={() => setShowCompletarCadastro(false)}
          onSaved={() => {
            setShowCompletarCadastro(false);
            setFeedbackMsg({ type: 'success', text: 'Cadastro atualizado com sucesso!' });
            loadCustomerData(true);
          }}
        />
      )}

      {/* Modal: Solicitar Serviço / Agendamento */}
      {showSolicitarServico && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm animate-in fade-in duration-200">
          <div className="bg-slate-900 border border-slate-700 rounded-2xl w-full max-w-md overflow-hidden shadow-2xl text-slate-100 p-5">
            <div className="flex items-center justify-between mb-4 border-b border-slate-800 pb-3">
              <h3 className="text-base font-extrabold text-white flex items-center gap-2">
                <Calendar className="w-5 h-5 text-inovar-yellow" />
                <span>Solicitar Atendimento da Inovar</span>
              </h3>
              <button onClick={() => setShowSolicitarServico(false)} className="text-slate-400 hover:text-white">✕</button>
            </div>

            <form onSubmit={handleSolicitarServico} className="space-y-3">
              {appliances.length > 0 && (
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">Qual Aparelho?</label>
                  <select
                    value={selectedApplianceId}
                    onChange={(e) => setSelectedApplianceId(e.target.value)}
                    className="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-xl text-sm text-white"
                  >
                    {appliances.map((a) => (
                      <option key={a.id} value={a.id}>
                        {a.marca} {a.modelo || ''} ({a.ambiente})
                      </option>
                    ))}
                  </select>
                </div>
              )}

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Tipo de Serviço</label>
                <select
                  value={servTipo}
                  onChange={(e) => setServTipo(e.target.value as any)}
                  className="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-xl text-sm text-white"
                >
                  <option value="LIMPEZA">Limpeza de Ar</option>
                  <option value="MANUTENCAO_PREVENTIVA">Manutenção Preventiva</option>
                  <option value="MANUTENCAO_CORRETIVA">Conserto / Reparo (Não está gelando / Barulho)</option>
                  <option value="RECARGA_GAS">Carga ou Teste de Gás Refrigerante</option>
                  <option value="INSTALACAO">Instalação / Desinstalação</option>
                  <option value="AVALIACAO">Visita Técnica de Avaliação</option>
                </select>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Data Preferida para Visita</label>
                <input
                  type="date"
                  value={servData}
                  onChange={(e) => setServData(e.target.value)}
                  className="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-xl text-sm text-white"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Observações ou Sintomas</label>
                <textarea
                  rows={2}
                  value={servProblema}
                  onChange={(e) => setServProblema(e.target.value)}
                  placeholder="Ex: Pingando água na sala, cheiro ruim ao ligar, etc."
                  className="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-xl text-sm text-white placeholder-slate-500"
                ></textarea>
              </div>

              <div className="p-3 bg-slate-800/60 rounded-xl text-[11px] text-slate-300 flex items-center gap-2">
                <ShieldCheck className="w-4 h-4 text-inovar-yellow shrink-0" />
                <span>Garantia de 90 dias com certificado e produtos antibacterianos homologados.</span>
              </div>

              <button
                type="submit"
                disabled={submittingServ}
                className="w-full mt-2 py-3 bg-inovar-yellow hover:brightness-105 text-inovar-navy font-bold rounded-xl text-sm shadow flex items-center justify-center gap-2"
              >
                {submittingServ ? <Loader2 className="w-4 h-4 animate-spin" /> : 'Confirmar Solicitação'}
              </button>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};

// ------- Modal: trocar senha -------
const TrocarSenhaModal: React.FC<{ onClose: () => void; onConfirm: (senha: string, setErr: (s: string) => void) => Promise<void> }> = ({ onClose, onConfirm }) => {
  const [s1, setS1] = useState('');
  const [s2, setS2] = useState('');
  const [err, setErr] = useState('');
  const [saving, setSaving] = useState(false);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="bg-slate-900 border border-slate-700 rounded-2xl w-full max-w-sm shadow-2xl text-slate-100 p-5">
        <div className="flex items-center justify-between mb-4 border-b border-slate-800 pb-3">
          <h3 className="text-base font-extrabold text-white flex items-center gap-2">
            <KeyRound className="w-5 h-5 text-inovar-yellow" />
            <span>Alterar Minha Senha</span>
          </h3>
          <button onClick={onClose} className="text-slate-400 hover:text-white">✕</button>
        </div>
        <div className="space-y-3 text-xs">
          {err && <div className="p-2.5 bg-red-950/60 border border-red-800 rounded-xl text-red-200">{err}</div>}
          <div>
            <label className="block text-[11px] font-bold text-slate-300 mb-1">Nova senha (mín. 6 caracteres)</label>
            <input type="password" value={s1} onChange={(e) => setS1(e.target.value)} className="w-full p-2 bg-slate-800 border border-slate-700 rounded-lg text-white" placeholder="••••••••" />
          </div>
          <div>
            <label className="block text-[11px] font-bold text-slate-300 mb-1">Confirmar nova senha</label>
            <input type="password" value={s2} onChange={(e) => setS2(e.target.value)} className="w-full p-2 bg-slate-800 border border-slate-700 rounded-lg text-white" placeholder="••••••••" />
          </div>
          <div className="flex gap-2 pt-1">
            <button onClick={onClose} className="flex-1 py-2.5 bg-slate-800 text-slate-300 rounded-xl font-semibold">Cancelar</button>
            <button
              disabled={saving || s1.length < 6 || s1 !== s2}
              onClick={async () => { setSaving(true); await onConfirm(s1, setErr); setSaving(false); }}
              className="flex-1 py-2.5 bg-inovar-yellow text-inovar-navy font-bold rounded-xl disabled:opacity-40 disabled:cursor-not-allowed"
            >
              {saving ? <Loader2 className="w-4 h-4 animate-spin mx-auto" /> : 'Salvar Senha'}
            </button>
          </div>
          {s1 && s2 && s1 !== s2 && <p className="text-[10px] text-red-400">As senhas não coincidem.</p>}
        </div>
      </div>
    </div>
  );
};

// ------- Modal: completar/editar cadastro -------
const CompletarCadastroModal: React.FC<{ customer: SupabaseCustomer; onClose: () => void; onSaved: () => void }> = ({ customer, onClose, onSaved }) => {
  const [whatsapp, setWhatsapp] = useState(customer.whatsapp || '');
  const [endereco, setEndereco] = useState(customer.endereco || '');
  const [numero, setNumero] = useState(customer.numero || '');
  const [bairro, setBairro] = useState(customer.bairro || '');
  const [cidade, setCidade] = useState(customer.cidade || '');
  const [cep, setCep] = useState('');
  const [buscandoCep, setBuscandoCep] = useState(false);
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState('');

  // CEP preenche rua/bairro/cidade automaticamente — campos seguem editáveis
  const aplicarCep = async (v: string) => {
    const m = mascaraCep(v);
    setCep(m);
    if (m.replace(/\D/g, '').length !== 8) return;
    setBuscandoCep(true);
    const end = await buscarCep(m);
    setBuscandoCep(false);
    if (end) {
      if (end.rua) setEndereco(end.rua);
      if (end.bairro) setBairro(end.bairro);
      if (end.cidade) setCidade(end.cidade);
    }
  };

  const salvar = async () => {
    setSaving(true);
    setErr('');
    try {
      const { error } = await SupabaseApi.updateCustomer(customer.id, {
        whatsapp,
        endereco,
        numero,
        bairro,
        cidade
      });
      if (error) throw error;
      onSaved();
    } catch (e: any) {
      setErr(e.message || 'Erro ao salvar.');
    } finally {
      setSaving(false);
    }
  };

  const inputCls = 'w-full p-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white placeholder-slate-500';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-in fade-in duration-200 overflow-y-auto">
      <div className="bg-slate-900 border border-slate-700 rounded-2xl w-full max-w-md shadow-2xl text-slate-100 p-5 my-4">
        <div className="flex items-center justify-between mb-4 border-b border-slate-800 pb-3">
          <h3 className="text-base font-extrabold text-white flex items-center gap-2">
            <UserCog className="w-5 h-5 text-sky-400" />
            <span>Meus Dados de Cadastro</span>
          </h3>
          <button onClick={onClose} className="text-slate-400 hover:text-white">✕</button>
        </div>
        <div className="space-y-3 text-xs">
          {err && <div className="p-2.5 bg-red-950/60 border border-red-800 rounded-xl text-red-200">{err}</div>}
          <div>
            <label className="block text-[11px] font-bold text-slate-300 mb-1">WhatsApp</label>
            <input className={inputCls} value={whatsapp} onChange={(e) => setWhatsapp(e.target.value)} placeholder="(27) 99999-9999" />
          </div>
          <div>
            <label className="block text-[11px] font-bold text-slate-300 mb-1">CEP (preenche o endereço automático)</label>
            <div className="relative">
              <input
                className={inputCls + ' pr-9'}
                value={cep}
                onChange={(e) => aplicarCep(e.target.value)}
                placeholder="Ex: 29060-270"
                inputMode="numeric"
              />
              {buscandoCep && <Loader2 className="w-4 h-4 text-inovar-yellow animate-spin absolute right-3 top-2.5" />}
            </div>
          </div>
          <div className="grid grid-cols-3 gap-2">
            <div className="col-span-2">
              <label className="block text-[11px] font-bold text-slate-300 mb-1">Endereço</label>
              <input className={inputCls} value={endereco} onChange={(e) => setEndereco(e.target.value)} placeholder="Rua / Av." />
            </div>
            <div>
              <label className="block text-[11px] font-bold text-slate-300 mb-1">Número</label>
              <input className={inputCls} value={numero} onChange={(e) => setNumero(e.target.value)} placeholder="123" />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-2">
            <div>
              <label className="block text-[11px] font-bold text-slate-300 mb-1">Bairro</label>
              <input className={inputCls} value={bairro} onChange={(e) => setBairro(e.target.value)} />
            </div>
            <div>
              <label className="block text-[11px] font-bold text-slate-300 mb-1">Cidade</label>
              <input className={inputCls} value={cidade} onChange={(e) => setCidade(e.target.value)} />
            </div>
          </div>
          <div className="flex gap-2 pt-1">
            <button onClick={onClose} className="flex-1 py-2.5 bg-slate-800 text-slate-300 rounded-xl font-semibold">Cancelar</button>
            <button
              disabled={saving}
              onClick={salvar}
              className="flex-1 py-2.5 bg-inovar-yellow text-inovar-navy font-bold rounded-xl flex items-center justify-center gap-1.5 disabled:opacity-40"
            >
              {saving ? <Loader2 className="w-4 h-4 animate-spin" /> : <CheckCircle2 className="w-4 h-4" />}
              <span>Salvar Dados</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};
