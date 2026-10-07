import React, { useState, useEffect, useRef } from 'react';
import { Client, Appliance, MaintenanceRecord, TechnicianProfile, ReturnStatus, ServiceType, BudgetEstimate, DadosTipoServico, ChecklistData } from './types';
import { StorageService } from './services/storage';
import { BudgetService } from './services/budgetService';
import {
  SupabaseProfile,
  SupabaseService as SupabaseApi,
  supabase,
  mapSupabaseBudgetToLocal,
  mapLocalBudgetToSupabase,
  SupabaseBudget
} from './services/supabase';
import { Header } from './components/Header';
import { QuemPrecisoChamar } from './components/QuemPrecisoChamar';
import { ProximosRetornos } from './components/ProximosRetornos';
import { ProximosAgendamentos } from './components/ProximosAgendamentos';
import { ClientesTab } from './components/ClientesTab';
import { SolicitacoesTab } from './components/SolicitacoesTab';
import { ServicosTab } from './components/ServicosTab';
import { OrcamentosTab } from './components/OrcamentosTab';
import { ClientePortal } from './components/ClientePortal';
import { RecuperarSenhaModal } from './components/RecuperarSenhaModal';
import { ContaModal } from './components/ContaModal';
import { automaticGoogleSync, connectOrSyncGoogleCalendar, googleCalendarRequest } from './services/googleConnection';
import { PwaStatus } from './components/PwaStatus';
import { enviarNotificacaoNosDispositivos } from './services/pushNotifications';
import { AuthModal } from './components/AuthModal';
import { FichaAparelhoModal } from './components/FichaAparelhoModal';
import { ChecklistLimpezaModal } from './components/ChecklistLimpezaModal';
import { OrdemServicoModal } from './components/OrdemServicoModal';
import { NovoClienteModal } from './components/NovoClienteModal';
import { NovoAgendamentoModal } from './components/NovoAgendamentoModal';
import { NovoAparelhoModal } from './components/NovoAparelhoModal';
import { NovoOrcamentoModal } from './components/NovoOrcamentoModal';
import { VisualizarOrcamentoModal } from './components/VisualizarOrcamentoModal';
import { ConfiguracoesModal } from './components/ConfiguracoesModal';
import { IniciarServicoModal } from './components/IniciarServicoModal';
import { MobileActionSheet } from './components/MobileActionSheet';
import { StatsBar } from './components/StatsBar';
import { AgendaTab } from './components/AgendaTab';
import { EditarClienteModal } from './components/EditarClienteModal';
import { EditarServicoModal } from './components/EditarServicoModal';
import { FinanceiroTab } from './components/FinanceiroTab';
import { AgendarChamadoModal } from './components/AgendarChamadoModal';
import { NovoServicoModal } from './components/NovoServicoModal';
import { NovoHistoricoModal } from './components/NovoHistoricoModal';
import {
  Calendar,
  Users,
  FileText,
  Wrench,
  CheckSquare,
  ShieldCheck,
  Sparkles,
  Smartphone,
  CheckCircle2,
  Wind,
  Menu,
  Clock,
  Zap,
  Flame,
  Plus,
  LogIn,
  AlertCircle,
  Calculator,
  Wallet,
  History
} from 'lucide-react';
import { Logo } from './components/Logo';
import { NotificacoesStore } from './components/CentralNotificacoes';
import { AutoWhatsapp } from './services/autoWhatsapp';
import { montarCatalogo, normalizarNomeServico } from './services/catalogo';
import { textoMensagem, aplicarPlaceholders } from './services/mensagensWhats';
import { MensagensWhatsModal } from './components/MensagensWhatsModal';
import { BudgetPdfService } from './services/budgetPdfGenerator';
import { PdfService } from './services/pdfGenerator';
import { format, parseISO, addMonths } from 'date-fns';
import { baixarConviteICS } from './services/calendario';

export function App() {
  const [clients, setClients] = useState<Client[]>(StorageService.getClients());
  const [maintenances, setMaintenances] = useState<MaintenanceRecord[]>(StorageService.getMaintenances());
  const [profile, setProfile] = useState<TechnicianProfile>(StorageService.getProfile());
  const [budgets, setBudgets] = useState<BudgetEstimate[]>(BudgetService.getBudgets());

  // Supabase state
  const [userProfile, setUserProfile] = useState<SupabaseProfile | null>(null);
  const [servicesList, setServicesList] = useState<any[]>([]);
  const [appointments, setAppointments] = useState<any[]>([]);
  const [showAccount, setShowAccount] = useState(false);
  const [googleSyncError, setGoogleSyncError] = useState('');
  const [googleCalendar, setGoogleCalendar] = useState<{ connected: boolean; lastSync?: string | null }>({ connected: false });
  const [googleCalendarBusy, setGoogleCalendarBusy] = useState(false);
  const [passwordRecovery, setPasswordRecovery] = useState(() => new URLSearchParams(window.location.search).get('recovery') === '1' || window.location.hash.includes('type=recovery'));
  const [passwordChangeRequired, setPasswordChangeRequired] = useState(false);
  const [showAuthModal, setShowAuthModal] = useState(false);
  const [showAuthModalSignup, setShowAuthModalSignup] = useState(false);
  const [authChecked, setAuthChecked] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [toast, setToast] = useState<{ type: 'success' | 'error' | 'info'; text: string } | null>(null);

  // Notificações no dispositivo (navegador/celular) + histórico (sino)
  const fireNotify = (title: string, body: string, url?: string) => {
    NotificacoesStore.registrar(title, body, url);
    setToast({ type: 'info', text: `${title} — ${body}` });
    // Entrega também nas outras instalações autorizadas desta mesma conta.
    void enviarNotificacaoNosDispositivos(title, body).catch(() => {});
    try {
      if (typeof Notification !== 'undefined') {
        if (Notification.permission === 'granted') {
          new Notification(title, { body });
        }
      }
    } catch { /* ignore */ }
  };

  useEffect(() => {
    const receber = (event: Event) => {
      const detail = (event as CustomEvent).detail || {};
      fireNotify(String(detail.titulo || 'InovarApp'), String(detail.texto || 'Há uma atualização no aplicativo.'), typeof detail.url === 'string' ? detail.url : undefined);
    };
    window.addEventListener('inovar:notificacao', receber);
    return () => window.removeEventListener('inovar:notificacao', receber);
  }, []);

  // Lembrete local preciso: enquanto o técnico estiver usando o PWA, avisa uma
  // única vez cerca de 1h antes de cada atendimento agendado. O cron diário
  // continua cuidando dos alertas de retorno e véspera quando o app está fechado.
  useEffect(() => {
    if (userProfile?.tipo !== 'ADMIN' && userProfile?.tipo !== 'TECNICO') return;
    const avisar = () => {
      const agora = new Date();
      const hoje = format(agora, 'yyyy-MM-dd');
      for (const service of servicesList) {
        if (service.status !== 'AGENDADO' || String(service.data_agendamento || '').slice(0, 10) !== hoje || !service.hora_agendamento) continue;
        const [h, m] = String(service.hora_agendamento).slice(0, 5).split(':').map(Number);
        const horario = new Date(agora); horario.setHours(h, m, 0, 0);
        const faltam = Math.round((horario.getTime() - agora.getTime()) / 60000);
        const key = `inovar-lembrete-1h-${service.id}-${hoje}`;
        if (faltam >= 55 && faltam <= 65 && !localStorage.getItem(key)) {
          localStorage.setItem(key, new Date().toISOString());
          const cliente = clients.find((c) => c.id === service.cliente_id);
          fireNotify('Atendimento em aproximadamente 1 hora', `${cliente?.name || 'Cliente'} — ${mapTipoSupabaseToLocal(service.tipo)} às ${service.hora_agendamento}.`);
        }
      }
    };
    avisar();
    const interval = window.setInterval(avisar, 60 * 1000);
    return () => window.clearInterval(interval);
  }, [servicesList, clients, userProfile?.id, userProfile?.tipo]);

  // Monitora mudanças: novo chamado de cliente / resposta de orçamento
  const prevNotif = useRef<Map<string, string>>(new Map());
  useEffect(() => {
    if (!userProfile || userProfile.tipo === 'CLIENTE') return;
    const atual = new Map<string, string>();
    servicesList.forEach((s: any) => atual.set('svc:' + s.id, s.status));
    budgets.forEach((b) => atual.set('bud:' + b.id, b.status));
    if (prevNotif.current.size > 0) {
      for (const [key, st] of atual) {
        const before = prevNotif.current.get(key);
        if (before && before !== st) {
          if (key.startsWith('svc:') && st === 'PENDENTE') {
            fireNotify('Novo chamado de cliente', 'Um cliente solicitou atendimento — confira a Central de Atendimento.');
          }
          if (key.startsWith('bud:') && st === 'APROVADO') {
            fireNotify('Orçamento aprovado pelo cliente!', 'Assinatura registrada — agende o serviço.');
          }
          if (key.startsWith('bud:') && st === 'RECUSADO') {
            fireNotify('Orçamento recusado', 'O cliente recusou a proposta.');
          }
        }
      }
    }
    prevNotif.current = atual;
  }, [servicesList, budgets, userProfile]);

  const [activeTab, setActiveTab] = useState<'inicio' | 'agenda' | 'proximos-agendamentos' | 'financeiro' | 'servicos' | 'orcamentos' | 'clientes' | 'proximos-retornos'>('inicio');
  const [filaFilter, setFilaFilter] = useState<'todos' | 'chamados' | 'atrasado' | 'esta_semana' | 'em_breve' | 'sem_historico'>('todos');
  const [filaSection, setFilaSection] = useState<'fila' | 'chamados' | 'historico'>('fila');

  // Active Modals
  const [modalFicha, setModalFicha] = useState<{ client: Client; appliance: Appliance } | null>(null);
  const [modalChecklist, setModalChecklist] = useState<{ client: Client; appliance: Appliance; serviceType?: ServiceType; budgetId?: string; serviceId?: string } | null>(null);
  const [modalOS, setModalOS] = useState<{ client: Client; appliance: Appliance; record: MaintenanceRecord } | null>(null);
  const [showNovoCliente, setShowNovoCliente] = useState(false);
  const [agendamentoTarget, setAgendamentoTarget] = useState<{ client?: Client; appliance?: Appliance } | null>(null);
  const [showNovoAparelho, setShowNovoAparelho] = useState<Client | null>(null);
  const [showConfiguracoes, setShowConfiguracoes] = useState(false);
  const [showIniciarServico, setShowIniciarServico] = useState<{ isOpen: boolean; initialType?: ServiceType }>({ isOpen: false });
  const [showMobileActionSheet, setShowMobileActionSheet] = useState(false);
  const [editCliente, setEditCliente] = useState<Client | null>(null);
  const [editServico, setEditServico] = useState<{ id: string; tipo: string; data: string; valor: number; cliente: string; aparelho: string; observacoes?: string } | null>(null);
  const [editBudget, setEditBudget] = useState<BudgetEstimate | null>(null);
  const [agendarChamado, setAgendarChamado] = useState<{ id: string; cliente: string; servico: string; aparelho: string; telefone?: string } | null>(null);
  const [agendarOrcamento, setAgendarOrcamento] = useState<BudgetEstimate | null>(null);
  const [showNovoServico, setShowNovoServico] = useState(false);
  const [orcamentoDeChamado, setOrcamentoDeChamado] = useState<Client | null>(null);
  const [historicoModal, setHistoricoModal] = useState<{ client?: Client; appliance?: Appliance } | null>(null);
  const [showMensagens, setShowMensagens] = useState(false);

  // helper: texto da mensagem automática (personalizada ou padrão) com placeholders aplicados
  const msg = (chave: string, vars: Record<string, string | number | undefined | null>) =>
    aplicarPlaceholders(textoMensagem(profile.mensagensWhats, chave), { empresa: profile.businessName || 'Inovar Refrigeração', app: 'https://inovarapp.vercel.app', ...vars });

  const handleSalvarMensagens = (mensagens: Record<string, string>) => {
    persistirPerfil({ ...profile, mensagensWhats: mensagens });
    setToast({ type: 'success', text: 'Mensagens do WhatsApp salvas no banco!' });
  };

  // Orçamentos Modals
  const [showNovoOrcamento, setShowNovoOrcamento] = useState(false);
  const [selectedBudget, setSelectedBudget] = useState<BudgetEstimate | null>(null);

  // Reload data from localStorage
  const loadData = () => {
    setClients(StorageService.getClients());
    setMaintenances(StorageService.getMaintenances());
    setProfile(StorageService.getProfile());
    setBudgets(BudgetService.getBudgets());
  };

  const addMonthsISO = (dateISO: string, months: number): string => {
    const d = new Date(dateISO + (dateISO.length === 10 ? 'T00:00:00' : ''));
    if (isNaN(d.getTime())) return format(new Date(), 'yyyy-MM-dd');
    return format(addMonths(d, months), 'yyyy-MM-dd');
  };

  const mapTipoSupabaseToLocal = (tipo: string): ServiceType => {
    switch (tipo) {
      case 'LIMPEZA': return 'Limpeza de Ar';
      case 'INSTALACAO': return 'Instalação';
      case 'MANUTENCAO_CORRETIVA': return 'Manutenção Corretiva';
      case 'MANUTENCAO_PREVENTIVA': return 'Manutenção Preventiva';
      case 'RECARGA_GAS': return 'Recarga de Gás';
      case 'AVALIACAO': return 'Avaliação Técnica';
      case 'OUTRO': return 'Outro';
      default: return normalizarNomeServico(tipo) as ServiceType;
    }
  };

  // Synchronize data with Supabase (fonte de verdade: banco)
  const syncFromSupabase = async (): Promise<{ maintenances?: MaintenanceRecord[]; clients?: Client[] }> => {
    setSyncing(true);
    let mappedRef: MaintenanceRecord[] | undefined;
    let clientsRef: Client[] | undefined;
    try {
      const [supaCustomers, supaServices, supaBudgets, supaAppointments] = await Promise.all([
        SupabaseApi.fetchCustomersWithAppliances(),
        SupabaseApi.fetchServices(),
        SupabaseApi.fetchBudgets(),
        SupabaseApi.fetchAppointments()
      ]);

      if (supaAppointments?.data) {
        setAppointments(supaAppointments.data);
      }

      if (supaServices?.data) {
        setServicesList(supaServices.data);
      }

      if (supaCustomers && supaCustomers.length > 0) {
        const dbClients: Client[] = supaCustomers.map((sc: any) => ({
          id: sc.id,
          name: sc.nome,
          phone: sc.whatsapp || '',
          document: sc.cpf_cnpj || '',
          address: sc.endereco || '',
          neighborhood: sc.bairro || '',
          city: sc.cidade || 'Serra',
          notes: sc.observacoes || '',
          createdAt: sc.created_at || new Date().toISOString(),
          appliances: (sc.appliances || []).map((sa: any) => ({
            id: sa.id,
            clientId: sc.id,
            brand: sa.marca,
            model: sa.modelo || '',
            type: (sa.tipo as any) || 'Split Hi-Wall',
            capacityBtu: sa.btus ? `${sa.btus.toLocaleString('pt-BR')}` : '12.000',
            room: sa.ambiente || 'Sala de Estar',
            gasType: 'R-410A',
            installDate: sa.data_instalacao || undefined
          }))
        }));

        // Clientes criados localmente (offline) que não estão no banco: tenta
        // enviar e mantém os que falharem para não perder cadastro.
        const dbIds = new Set(dbClients.map((c) => c.id));
        const locaisPendentes = StorageService.getClients().filter(
          (c) => !dbIds.has(c.id) && (c.id.startsWith('c_') || c.id.startsWith('local_'))
        );
        const mantidosLocais: Client[] = [];
        for (const pendente of locaisPendentes) {
          try {
            const { data, error } = await SupabaseApi.createCustomer({
              nome: pendente.name,
              whatsapp: pendente.phone,
              endereco: pendente.address,
              bairro: pendente.neighborhood,
              cidade: pendente.city,
              observacoes: pendente.notes,
              ativo: true
            });
            if (error) throw error;
            if (data?.id) {
              dbClients.unshift({ ...pendente, id: data.id });
            }
          } catch (pushErr) {
            console.warn('Cliente local aguardando sincronização:', pushErr);
            mantidosLocais.push(pendente);
          }
        }

        const mergedClients = [...mantidosLocais, ...dbClients];
        setClients(mergedClients);
        StorageService.saveClients(mergedClients);
        clientsRef = mergedClients;
      }

      if (supaServices?.data) {
        // Preserva registros locais ainda não sincronizados (criados offline)
        const localOnly = StorageService.getMaintenances().filter(
          (m) => (m.id.startsWith('m_') || m.id.startsWith('local_')) &&
                 !supaServices.data.some((s: any) => s.id === m.id)
        );

        const mappedMaintenances: MaintenanceRecord[] = supaServices.data.map((s: any) => {
          const scheduledDate = s.data_agendamento || (s.data_solicitacao ? s.data_solicitacao.split('T')[0] : '');
          const completionMarker = String(s.observacoes || '').match(/\[DATA_CONCLUSAO:(\d{4}-\d{2}-\d{2})\]/);
          const completionDate = (s.data_conclusao ? String(s.data_conclusao).slice(0, 10) : '') || completionMarker?.[1] || (s.status === 'CONCLUIDO' ? scheduledDate : '');
          const startMarker = String(s.observacoes || '').match(/\[DATA_INICIO:(\d{4}-\d{2}-\d{2})\]/);
          const cancellationMarker = String(s.observacoes || '').match(/\[DATA_CANCELAMENTO:(\d{4}-\d{2}-\d{2})\]/);
          const cancellationReasonMarker = String(s.observacoes || '').match(/\[MOTIVO_CANCELAMENTO:([^\]]+)\]/);
          const warrantyMarker = String(s.observacoes || '').match(/\[GARANTIA_DIAS:(\d+)\]/);
          const retornoMarker = String(s.observacoes || '').match(/\[PROXIMO_RETORNO:(\d{4}-\d{2}-\d{2})\]/);
          const maoObraMarker = String(s.observacoes || '').match(/\[MAO_OBRA:(\d+(?:\.\d+)?)\]/);
          const pecasMarker = String(s.observacoes || '').match(/\[PECAS:(\d+(?:\.\d+)?)\]/);
          const checklistMarker = String(s.observacoes || '').match(/\[CHECKLIST:(\{[\s\S]*?\})\]/);
          let checklistRestaurado: ChecklistData | undefined;
          if (checklistMarker) {
            try { checklistRestaurado = JSON.parse(checklistMarker[1]); } catch { /* marcador truncado — ignora */ }
          }
          // Texto limpo para exibição: remove os marcadores internos (ciclo, garantia, checklist)
          const obsLimpa = String(s.observacoes || s.problema || s.descricao || '')
            .replace(/\[(?:GARANTIA_DIAS|DATA_CONCLUSAO|DATA_INICIO|DATA_CANCELAMENTO|MOTIVO_CANCELAMENTO|PROXIMO_RETORNO|MAO_OBRA|PECAS):[^\]]+\]/g, '')
            .replace(/\[CHECKLIST:\{[\s\S]*?\}\]/g, '')
            .replace(/\s+/g, ' ')
            .trim();
          const baseDate = completionDate || scheduledDate || format(new Date(), 'yyyy-MM-dd');
          const isDone = s.status === 'CONCLUIDO';
          return {
            id: s.id,
            clientId: s.cliente_id,
            applianceId: s.aparelho_id || '',
            date: baseDate,
            scheduledDate: scheduledDate || undefined,
            scheduledTime: s.hora_agendamento || undefined,
            completionDate: isDone ? completionDate : undefined,
            completedAt: isDone ? (completionDate ? completionDate + 'T12:00:00' : null) : null,
            startedAt: s.data_inicio ? `${String(s.data_inicio).slice(0, 10)}T12:00:00` : (startMarker?.[1] ? `${startMarker[1]}T12:00:00` : null),
            cancelledAt: s.data_cancelamento ? `${String(s.data_cancelamento).slice(0, 10)}T12:00:00` : (cancellationMarker?.[1] ? `${cancellationMarker[1]}T12:00:00` : null),
            cancellationReason: s.motivo_cancelamento || cancellationReasonMarker?.[1],
            returnDate: isDone ? (retornoMarker?.[1] || addMonthsISO(baseDate, profile.defaultReturnMonths || 6)) : '',
            serviceType: normalizarNomeServico((s.tipo === 'OUTRO' && s.descricao) ? s.descricao : mapTipoSupabaseToLocal(s.tipo)) as ServiceType,
            price: Number(s.valor) || 0,
            laborPrice: maoObraMarker ? Number(maoObraMarker[1]) : undefined,
            partsPrice: pecasMarker ? Number(pecasMarker[1]) : undefined,
            checklist: checklistRestaurado,
            paymentMethod: 'PIX',
            warrantyDays: warrantyMarker ? Number(warrantyMarker[1]) : profile.defaultWarrantyDays || 90,
            notes: obsLimpa || undefined,
            status: isDone ? 'concluido' : s.status === 'CANCELADO' ? 'cancelado' : s.status === 'EM_ANDAMENTO' ? 'em_andamento' : 'agendado'
          };
        });

        const merged = [...localOnly, ...mappedMaintenances];
        setMaintenances(merged);
        StorageService.saveMaintenances(merged);
        mappedRef = mappedMaintenances;
      }

      if (supaBudgets?.data) {
        const clientesConhecidos = StorageService.getClients();
        const dbBudgets = (supaBudgets.data as SupabaseBudget[]).map((row) => {
          const budget = mapSupabaseBudgetToLocal(row);
          const cliente = clientesConhecidos.find((c) => c.id === budget.clientId);
          return cliente ? { ...budget, clientName: cliente.name, clientPhone: cliente.phone, clientAddress: [cliente.address, cliente.neighborhood, cliente.city].filter(Boolean).join(', '), clientDocument: cliente.document || budget.clientDocument } : budget;
        });
        const dbIds = new Set(dbBudgets.map((b) => b.id));

        // Orçamentos criados localmente (offline / sync falhou) que ainda não
        // estão no banco: tenta enviar agora (push) e, se não conseguir,
        // mantém na lista para não perder nenhum orçamento.
        const locaisPendentes = BudgetService.getBudgets().filter(
          (b) => !dbIds.has(b.id) && (b.id.startsWith('orc_') || b.id.startsWith('local_'))
        );
        const mantidosLocais: BudgetEstimate[] = [];
        for (const pendente of locaisPendentes) {
          try {
            let clienteId = isUuid(pendente.clientId) && clients.some((c) => c.id === pendente.clientId)
              ? pendente.clientId
              : '';
            if (!clienteId) {
              const byName = clients.find(
                (c) => c.name.trim().toLowerCase() === pendente.clientName.trim().toLowerCase()
              );
              if (byName) clienteId = byName.id;
            }
            if (!clienteId) {
              const { data: novoCust, error: custErr } = await SupabaseApi.createCustomer({
                nome: pendente.clientName,
                whatsapp: pendente.clientPhone,
                ativo: true
              });
              if (custErr) throw custErr;
              clienteId = novoCust!.id;
            }
            const numero = await SupabaseApi.proximoNumeroOrcamento();
            const payload = mapLocalBudgetToSupabase(pendente, numero, clienteId, profile.name || 'Inovar Refrigeração');
            const { data, error } = await SupabaseApi.createBudget(payload);
            if (error) throw error;
            // entra na lista como orçamento do banco (mesma identidade no futuro)
            dbBudgets.unshift(mapSupabaseBudgetToLocal(data as SupabaseBudget));
          } catch (pushErr) {
            console.warn('Orçamento local aguardando sincronização:', pushErr);
            mantidosLocais.push(pendente);
          }
        }

        const mergedBudgets = [...mantidosLocais, ...dbBudgets];
        setBudgets(mergedBudgets);
        BudgetService.saveBudgets(mergedBudgets);
      }
    } catch (err) {
      console.warn('Sync do Supabase offline ou em carregamento:', err);
    } finally {
      setSyncing(false);
    }
    return { maintenances: mappedRef, clients: clientsRef };
  };

  // Carrega o perfil técnico sincronizado (PIX, assinatura, WhatsApp etc.)
  const carregarPerfilRemoto = async () => {
    try {
      const session = await SupabaseApi.getSession();
      if (!session) return;
      const r = await fetch('/api/configuracoes', { headers: { Authorization: `Bearer ${session.access_token}` } });
      const j = await r.json();
      if (j.ok && j.config) {
        const merged = { ...StorageService.getProfile(), ...j.config };
        // Feed de calendário automático: gera o token na primeira carga (equipe),
        // sem precisar ativar nada em Configurações — o botão calendário já nasce funcionando.
        if (!merged.calendario_token) {
          merged.calendario_token = (crypto.randomUUID ? crypto.randomUUID().replace(/-/g, '') : Date.now().toString(36) + Math.random().toString(36).slice(2));
          fetch('/api/configuracoes', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${session.access_token}` },
            body: JSON.stringify(merged)
          }).catch(() => {});
        }
        setProfile(merged);
        StorageService.saveProfile(merged);
      }
    } catch { /* fallback: perfil local */ }
  };

  useEffect(() => {
    loadData();

    // Check existing auth session securely
    const initAuth = async () => {
      const session = await SupabaseApi.getSession();
      if (session?.user) {
        setPasswordChangeRequired(session.user.user_metadata?.must_change_password === true);
        const p = await SupabaseApi.getProfile(session.user.id);
        setUserProfile(p);
        await syncFromSupabase();
        await carregarPerfilRemoto();
      }
      setAuthChecked(true);
    };

    initAuth();

    // Listen to Auth changes
    const { data: authSub } = supabase.auth.onAuthStateChange((event, session) => {
      if (event === 'PASSWORD_RECOVERY') setPasswordRecovery(true);
      // Supabase auth callbacks run under the auth lock: defer other auth calls.
      setTimeout(async () => {
      if (session?.user) {
        setPasswordChangeRequired(session.user.user_metadata?.must_change_password === true);
        const p = await SupabaseApi.getProfile(session.user.id);
        setUserProfile(p);
        await syncFromSupabase();
        await carregarPerfilRemoto();
      } else {
        setUserProfile(null);
        setPasswordChangeRequired(false);
      }
      }, 0);
    });

    // Sincronização periódica dos dados do aplicativo.
    const intervalo = setInterval(async () => {
      const session = await SupabaseApi.getSession();
      if (!session) return;
      syncFromSupabase();
    }, 60000);

    // Sincronização imediata ao voltar para o app (troca de aba/celular) — dados sempre frescos
    const syncAoVoltar = () => {
      if (document.visibilityState === 'visible') {
        SupabaseApi.getSession().then((s) => { if (s) syncFromSupabase(); });
      }
    };
    document.addEventListener('visibilitychange', syncAoVoltar);
    window.addEventListener('focus', syncAoVoltar);

    return () => {
      authSub.subscription.unsubscribe();
      clearInterval(intervalo);
      document.removeEventListener('visibilitychange', syncAoVoltar);
      window.removeEventListener('focus', syncAoVoltar);
    };
  }, []);

  useEffect(() => {
    if (userProfile?.tipo === 'TECNICO' || userProfile?.tipo === 'ADMIN') automaticGoogleSync().catch(() => {});
  }, [servicesList, userProfile?.id]);
  useEffect(() => {
    const equipe = userProfile?.tipo === 'TECNICO' || userProfile?.tipo === 'ADMIN';
    if (!equipe) return;
    // Após a primeira autorização, mantém o calendário atualizado em segundo plano
    // e também ao retornar ao PWA. O botão da Agenda só serve para conectar ou forçar uma atualização.
    const atualizar = () => automaticGoogleSync().catch(() => {});
    const intervalo = window.setInterval(atualizar, 5 * 60 * 1000);
    const aoRetornar = () => { if (document.visibilityState === 'visible') atualizar(); };
    document.addEventListener('visibilitychange', aoRetornar);
    return () => { window.clearInterval(intervalo); document.removeEventListener('visibilitychange', aoRetornar); };
  }, [userProfile?.id, userProfile?.tipo]);
  useEffect(() => {
    const equipe = userProfile?.tipo === 'TECNICO' || userProfile?.tipo === 'ADMIN';
    if (!equipe) { setGoogleCalendar({ connected: false }); return; }
    googleCalendarRequest()
      .then((data) => setGoogleCalendar({ connected: !!data.connected, lastSync: data.lastSync || null }))
      .catch((error) => setGoogleSyncError(error.message || 'Não foi possível consultar o Google Agenda.'));
  }, [userProfile?.id, userProfile?.tipo]);
  useEffect(() => {
    const onStatus = (event: Event) => {
      const detail = (event as CustomEvent).detail || {};
      setGoogleSyncError(detail.error || '');
      if (!detail.error && typeof detail.connected === 'boolean') {
        setGoogleCalendar((current) => ({ connected: detail.connected, lastSync: detail.lastSync || current.lastSync || null }));
      }
    };
    window.addEventListener('inovar-calendar-status', onStatus);
    return () => window.removeEventListener('inovar-calendar-status', onStatus);
  }, []);

  const handleGoogleCalendarSync = async () => {
    if (userProfile?.tipo !== 'TECNICO' && userProfile?.tipo !== 'ADMIN') return;
    setGoogleCalendarBusy(true);
    try {
      const result = await connectOrSyncGoogleCalendar();
      if (result.authorizing) {
        setToast({ type: 'info', text: 'Autorize a Inovar no Google. Depois disso a agenda seguirá sincronizando automaticamente.' });
        return;
      }
      setGoogleCalendar({ connected: true, lastSync: result.lastSync || new Date().toISOString() });
      setGoogleSyncError('');
      setToast({ type: 'success', text: 'Google Agenda atualizado e sincronização automática ativa.' });
    } catch (error: any) {
      setGoogleSyncError(error.message || 'Não foi possível atualizar o Google Agenda.');
      setToast({ type: 'error', text: error.message || 'Não foi possível atualizar o Google Agenda.' });
    } finally {
      setGoogleCalendarBusy(false);
    }
  };

  const handleSignOut = async () => {
    setShowAccount(false); setGoogleSyncError('');
    await SupabaseApi.signOut();
    setUserProfile(null);
    // fecha tudo que ficou aberto da sessão anterior (modais não devem atravessar logout)
    setSelectedBudget(null);
    setModalOS(null);
    setModalFicha(null);
    setModalChecklist(null);
    setEditCliente(null);
    setEditServico(null);
    setEditBudget(null);
    setAgendarChamado(null);
    setAgendarOrcamento(null);
    setHistoricoModal(null);
    setShowNovoCliente(false);
    setShowNovoServico(false);
    setShowNovoOrcamento(false);
    setShowConfiguracoes(false);
    setShowIniciarServico({ isOpen: false });
    setShowMobileActionSheet(false);
    setShowNovoAparelho(null);
    setOrcamentoDeChamado(null);
    setAgendamentoTarget(null);
  };

  // Compute Queue Items (Quem preciso chamar)
  // Orçamentos também alimentam a fila: pendente entra pela validade e aprovado
  // (ainda sem OS) entra como ação imediata — inclusive clientes sem aparelho.
  const hojeISO = format(new Date(), 'yyyy-MM-dd');
  const queueItems = clients.flatMap((client) => {
    const orcamentosCliente = budgets.filter((b) => b.clientId === client.id && (b.status === 'pendente' || b.status === 'aprovado'));

    const porAparelho = (client.appliances || []).map((appliance) => {
      const appMaintenances = maintenances
        .filter((m) => m.applianceId === appliance.id)
        .sort((a, b) => new Date(b.date).getTime() - new Date(a.date).getTime());

      // chamados e execução vêm dos serviços AO VIVO (o mapeamento junta PENDENTE e AGENDADO)
      const servicosAparelho = servicesList.filter((s: any) => s.cliente_id === client.id && (!s.aparelho_id || s.aparelho_id === appliance.id));
      const chamados = servicosAparelho.filter((s: any) => s.status === 'PENDENTE');
      const emExecucao = servicosAparelho.filter((s: any) => s.status === 'EM_ANDAMENTO');

      const lastMaintenance = appMaintenances.find((m) => m.status === 'concluido');
      // só é "agendado" de verdade se o serviço ao vivo está AGENDADO (PENDENTE
      // também mapeia como 'agendado', mas não tem data marcada)
      const agendadosIds = new Set(servicosAparelho.filter((s: any) => s.status === 'AGENDADO').map((s: any) => s.id));
      const scheduledMaintenance = appMaintenances
        .filter((m) => m.status === 'agendado' && agendadosIds.has(m.id) && !!(m.scheduledDate || m.date))
        .sort((a, b) => (a.scheduledDate || a.date).localeCompare(b.scheduledDate || b.date))[0];
      // A fila também mostra o próximo agendamento, inclusive antes do primeiro atendimento.
      let returnDate = scheduledMaintenance?.scheduledDate || scheduledMaintenance?.date || lastMaintenance?.returnDate || '';
      // Sem atendimento nem agendamento, um orçamento pendente/aprovado coloca o cliente na fila
      let orcamentoRef: BudgetEstimate | undefined;
      if (!returnDate) {
        orcamentoRef = orcamentosCliente.find((b) => !b.applianceId || b.applianceId === appliance.id);
        if (orcamentoRef) {
          returnDate = orcamentoRef.status === 'aprovado' ? hojeISO : (orcamentoRef.validUntil || hojeISO);
        }
      }
      const returnStatus: ReturnStatus = returnDate ? StorageService.getReturnStatus(returnDate) : 'sem_historico';

      return {
        client,
        appliance,
        lastMaintenance,
        scheduledMaintenance,
        returnStatus,
        returnDate,
        historico: appMaintenances,
        orcamento: orcamentoRef,
        deOrcamento: !lastMaintenance && !scheduledMaintenance && !!orcamentoRef,
        chamados,
        emExecucao
      };
    });

    // Cliente com orçamento mas SEM aparelho cadastrado: entra na fila com o
    // equipamento descrito no próprio orçamento
    if ((client.appliances || []).length === 0 && orcamentosCliente.length > 0) {
      const destaque = orcamentosCliente.find((b) => b.status === 'aprovado') || orcamentosCliente[0];
      const returnDate = destaque.status === 'aprovado' ? hojeISO : (destaque.validUntil || hojeISO);
      porAparelho.push({
        client,
        appliance: {
          id: 'orc_' + destaque.id,
          clientId: client.id,
          brand: destaque.applianceDesc || destaque.equipmentName || 'Equipamento do orçamento',
          model: '',
          type: 'Split Hi-Wall',
          capacityBtu: '',
          room: ''
        },
        lastMaintenance: undefined,
        scheduledMaintenance: undefined,
        returnStatus: StorageService.getReturnStatus(returnDate),
        returnDate,
        historico: [],
        orcamento: destaque,
        deOrcamento: true,
        chamados: [],
        emExecucao: []
      });
    }

    return porAparelho;
  });

  const priorityOrder: Record<ReturnStatus, number> = {
    atrasado: 0,
    sem_historico: 1,
    esta_semana: 2,
    em_breve: 3,
    em_dia: 4
  };
  // Prioridade + dentro do mesmo nível o mais urgente primeiro (data mais antiga/próxima)
  const sortedQueue = [...queueItems].sort((a, b) => {
    const pa = priorityOrder[a.returnStatus];
    const pb = priorityOrder[b.returnStatus];
    if (pa !== pb) return pa - pb;
    const da = a.returnDate || '9999-12-31';
    const dbb = b.returnDate || '9999-12-31';
    return da.localeCompare(dbb);
  });

  // Um retorno por aparelho: sempre o próximo retorno calculado a partir do último atendimento.
  // Agendamentos futuros permanecem visíveis, sem duplicar o histórico anterior.
  const returnEvents: { client: Client; appliance: Appliance; maintenance: MaintenanceRecord; returnDate: string }[] = [
    ...queueItems
      .filter((q) => !!q.lastMaintenance?.returnDate && !q.scheduledMaintenance && q.emExecucao.length === 0)
      .map((q) => ({
        client: q.client,
        appliance: q.appliance,
        maintenance: q.lastMaintenance as MaintenanceRecord,
        returnDate: q.lastMaintenance!.returnDate
      }))
  ];

  // Chamados PENDENTES não são "próximos agendamentos" — só o que está realmente AGENDADO
  const pendentesIds = new Set(servicesList.filter((s: any) => s.status === 'PENDENTE').map((s: any) => s.id));
  const appointmentEvents = maintenances
    .map((m) => {
      const client = clients.find((c) => c.id === m.clientId);
      const appliance = client?.appliances?.find((a) => a.id === m.applianceId);
      if (!client || !appliance || !['agendado', 'em_andamento'].includes(m.status) || pendentesIds.has(m.id)) return null;
      return { client, appliance, maintenance: m, scheduledDate: m.scheduledDate || m.date };
    })
    .filter(Boolean) as { client: Client; appliance: Appliance; maintenance: MaintenanceRecord; scheduledDate: string }[];

  // Stats
  const stats = {
    totalClientes: clients.length,
    atrasados: queueItems.filter((q) => q.returnStatus === 'atrasado').length,
    estaSemana: queueItems.filter((q) => q.returnStatus === 'esta_semana').length,
    emBreve: queueItems.filter((q) => q.returnStatus === 'em_breve').length,
    semHistorico: queueItems.filter((q) => q.returnStatus === 'sem_historico').length,
    solicitacoesPendentes: servicesList.filter((s) => s.status === 'PENDENTE').length,
    totalOrcamentos: budgets.length
  };

  const isUuid = (v: string) => /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(v || '');

  const mapServiceTypeToSupabase = (t: string): any => {
    t = normalizarNomeServico(t);
    switch (t) {
      case 'Limpeza de Ar': return 'LIMPEZA';
      case 'Instalação': return 'INSTALACAO';
      case 'Manutenção Corretiva': return 'MANUTENCAO_CORRETIVA';
      case 'Conserto / Carga Gás': return 'MANUTENCAO_CORRETIVA';
      case 'Recarga de Gás': return 'RECARGA_GAS';
      case 'Manutenção Preventiva': return 'MANUTENCAO_PREVENTIVA';
      case 'Avaliação Técnica': return 'AVALIACAO';
      default: return 'OUTRO';
    }
  };

  // Handlers
  const handleSaveClient = async (newClient: Client, acesso?: { email: string; senha: string }) => {
    setShowNovoCliente(false);
    let customerId = '';

    try {
      const { data, error } = await SupabaseApi.createCustomer({
        nome: newClient.name,
        whatsapp: newClient.phone,
        endereco: newClient.address,
        bairro: newClient.neighborhood,
        cidade: newClient.city,
        observacoes: newClient.notes,
        ativo: true
      });
      if (error) throw error;
      if (data?.id) { newClient.id = data.id; customerId = data.id; }
      await syncFromSupabase();
    } catch (err) {
      console.warn('Erro ao sincronizar cliente com Supabase (mantido local):', err);
      const updated = [newClient, ...clients];
      setClients(updated);
      StorageService.saveClients(updated);
    }

    // Conta de acesso do cliente (e-mail + senha inicial 123456)
    if (acesso && customerId) {
      try {
        const { data: sessionData } = await supabase.auth.getSession();
        const r = await fetch('/api/contas', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${sessionData?.session?.access_token || ''}`
          },
          body: JSON.stringify({
            acao: 'do_tecnico',
            email: acesso.email,
            senha: acesso.senha,
            nome: newClient.name,
            telefone: newClient.phone,
            customer_id: customerId
          })
        });
        const resp = await r.json().catch(() => ({}));
        if (r.ok) {
          setToast({ type: 'success', text: resp.mensagem || 'Conta do cliente criada com sucesso!' });
        } else {
          setToast({ type: 'error', text: resp.error || 'Cliente salvo, mas a conta não pôde ser criada.' });
        }
      } catch (err: any) {
        setToast({ type: 'error', text: 'Cliente salvo localmente. Conta de acesso não criada (sem conexão).' });
      }
    }
  };

  const handleSaveAppliance = async (updatedAppliance: Appliance) => {
    const updatedClients = clients.map((c) => {
      if (c.id === updatedAppliance.clientId) {
        return {
          ...c,
          appliances: (c.appliances || []).map((a) => (a.id === updatedAppliance.id ? updatedAppliance : a))
        };
      }
      return c;
    });
    setClients(updatedClients);
    StorageService.saveClients(updatedClients);

    // Sincroniza a edição no banco (visível em todas as plataformas)
    if (isUuid(updatedAppliance.id)) {
      try {
        const { error } = await SupabaseApi.updateAirConditioner(updatedAppliance.id, {
          tipo: updatedAppliance.type,
          marca: updatedAppliance.brand,
          modelo: updatedAppliance.model || undefined,
          btus: parseInt(updatedAppliance.capacityBtu.replace(/\D/g, '')) || undefined,
          ambiente: updatedAppliance.room
        });
        if (error) throw error;
        await syncFromSupabase();
      } catch (err) {
        console.warn('Erro ao editar aparelho no Supabase (mantido local):', err);
      }
    }
  };

  const handleAddApplianceToClient = async (appliance: Appliance) => {
    setShowNovoAparelho(null);

    try {
      const { data, error } = await SupabaseApi.createAirConditioner({
        cliente_id: appliance.clientId,
        marca: appliance.brand,
        modelo: appliance.model,
        btus: parseInt(appliance.capacityBtu.replace(/\D/g, '')) || 12000,
        tipo: appliance.type,
        ambiente: appliance.room,
        ultima_manutencao: new Date().toISOString().split('T')[0]
      });
      if (error) throw error;
      if (data?.id) appliance.id = data.id;
      await syncFromSupabase();
    } catch (err) {
      console.warn('Erro ao sincronizar aparelho com Supabase (mantido local):', err);
      const updatedClients = clients.map((c) => {
        if (c.id === appliance.clientId) {
          return { ...c, appliances: [...(c.appliances || []), appliance] };
        }
        return c;
      });
      setClients(updatedClients);
      StorageService.saveClients(updatedClients);
    }
  };

  const handleSaveMaintenance = async (record: MaintenanceRecord) => {
    const client = clients.find((c) => c.id === record.clientId);
    const appliance = client?.appliances?.find((a) => a.id === record.applianceId);
    const garantiaDias = record.warrantyDays ?? profile.defaultWarrantyDays ?? 90;
    // O atendimento deve concluir a OS já aberta (chamado/agendamento em execução),
    // e não criar uma segunda ordem de serviço para o mesmo pedido.
    const abertos = servicesList.filter((s: any) =>
      s.cliente_id === record.clientId &&
      (s.status === 'PENDENTE' || s.status === 'AGENDADO' || s.status === 'EM_ANDAMENTO') &&
      (!record.applianceId || !s.aparelho_id || s.aparelho_id === record.applianceId)
    );
    const alvo = modalChecklist?.serviceId
      ? abertos.find((s: any) => s.id === modalChecklist.serviceId)
      : abertos.length === 1 ? abertos[0] : undefined;
    if (modalChecklist?.serviceId && !alvo) {
      setToast({ type: 'error', text: 'Esta OS já foi alterada. Atualize a lista antes de concluir.' });
      return;
    }
    if (!modalChecklist?.serviceId && abertos.length > 1) {
      setToast({ type: 'error', text: 'Há mais de uma OS aberta para este aparelho. Abra a OS desejada e toque em Finalizar.' });
      return;
    }

    try {
      let svcRow: any = null;
      // Marcadores persistidos nas observações (sobrevivem ao sync e às edições):
      // retorno escolhido no checklist, checklist técnico completo e repartição do valor
      const extras = [
        record.returnDate ? `[PROXIMO_RETORNO:${record.returnDate}]` : '',
        record.checklist && Object.keys(record.checklist).length ? `[CHECKLIST:${JSON.stringify(record.checklist)}]` : '',
        record.laborPrice != null ? `[MAO_OBRA:${record.laborPrice}]` : '',
        record.partsPrice != null ? `[PECAS:${record.partsPrice}]` : ''
      ].filter(Boolean).join(' ');
      const obs = `Serviço de campo concluído (${record.serviceType}). [GARANTIA_DIAS:${garantiaDias}] Garantia de ${garantiaDias} dias. Pagamento: ${record.paymentMethod}.${extras ? ' ' + extras : ''}`;
      if (alvo) {
        let upd = await SupabaseApi.updateService(alvo.id, {
          tipo: mapServiceTypeToSupabase(record.serviceType),
          status: 'CONCLUIDO',
          valor: record.price,
          data_conclusao: record.date,
          descricao: record.serviceType,
          problema: record.notes,
          observacoes: obs
        });
        if (upd.error) {
          // Base sem a migration de datas: grava a conclusão como marcador nas observações.
          upd = await SupabaseApi.updateService(alvo.id, {
            tipo: mapServiceTypeToSupabase(record.serviceType),
            status: 'CONCLUIDO',
            valor: record.price,
            descricao: record.serviceType,
            problema: record.notes,
            observacoes: `${obs} [DATA_CONCLUSAO:${record.date}]`
          });
        }
        if (upd.error) throw upd.error;
        svcRow = upd.data || { id: alvo.id };
      } else {
        const { data: createdRow, error } = await SupabaseApi.createService({
          cliente_id: record.clientId,
          aparelho_id: record.applianceId || undefined,
          tipo: mapServiceTypeToSupabase(record.serviceType),
          status: 'CONCLUIDO',
          valor: record.price,
          data_agendamento: record.date,
          descricao: record.serviceType,
          problema: record.notes,
          observacoes: obs
        });
        if (error) throw error;
        svcRow = createdRow;
      }
      if (svcRow?.id) record.id = svcRow.id;

      // orçamento aprovado que originou o atendimento → vincula a OS
      // (o faturamento usa o vínculo para não contar o mesmo valor 2x)
      const budgetOrigem = modalChecklist?.budgetId;
      if (budgetOrigem && isUuid(budgetOrigem)) {
        try { await SupabaseApi.updateBudget(budgetOrigem, { service_id: record.id }); } catch { /* segue sem vínculo */ }
      }

      if (appliance) {
        await SupabaseApi.updateAirConditionerMaintenance(appliance.id, record.date);
      }

      fireNotify('Ordem de Serviço concluída', `OS de ${record.serviceType} registrada para ${client?.name || 'o cliente'}.`);

      // ENVIO AUTOMATICO: OS em PDF para o WhatsApp do cliente
      try {
        const telefone = client?.phone;
        if (telefone && svcRow?.id) {
          const doc = await PdfService.buildServiceOrderPdf(client, appliance, record, profile);
          const base64 = doc.output('datauristring');
          AutoWhatsapp.ordemServico({
            telefone,
            osId: 'OS#' + svcRow.id.slice(0, 6).toUpperCase(),
            clienteNome: client.name,
            servico: record.serviceType,
            dataServico: record.date,
            garantiaDias: record.warrantyDays,
            pdfBase64: base64,
            nomePdf: 'OS_' + client.name.replace(/\s+/g, '_') + '.pdf',
            texto: msg('os_concluida', {
              cliente: client.name.split(' ')[0],
              os: 'OS#' + svcRow.id.slice(0, 6).toUpperCase(),
              servico: record.serviceType,
              data: record.date.split('-').reverse().join('/'),
              garantia: record.warrantyDays + ' dias'
            })
          }).then((ok) => {
            if (ok) fireNotify('OS enviada no WhatsApp', `O comprovante de ${client.name} foi enviado automaticamente.`);
          });
        }
      } catch { /* envio automatico e best-effort */ }

      // Histórico técnico completo (service_history)
      SupabaseApi.insertServiceHistory({
        service_id: svcRow?.id,
        cliente_id: record.clientId,
        aparelho_id: record.applianceId || undefined,
        data: record.date,
        descricao: record.serviceType,
        problema: record.notes,
        solucao: record.serviceType,
        pecas_utilizadas: record.partsUsed,
        observacoes: record.checklist ? JSON.stringify(record.checklist) : undefined,
        valor: record.price
      }).catch(() => {});

      await syncFromSupabase();
    } catch (err) {
      console.warn('Erro ao salvar serviço no Supabase:', err);
      setToast({ type: 'error', text: 'Não foi possível concluir no banco. Seus dados continuam no formulário; tente novamente.' });
      return;
    }

    setModalChecklist(null);
    if (client && appliance) {
      setModalOS({ client, appliance, record });
    }
  };

  const handleSaveAgendamento = async (record: MaintenanceRecord) => {
    try {
      const { data: svcRow, error } = await SupabaseApi.createService({
        cliente_id: record.clientId,
        aparelho_id: record.applianceId || undefined,
        tipo: mapServiceTypeToSupabase(record.serviceType),
        status: 'AGENDADO',
        valor: record.price,
        data_agendamento: record.date,
        hora_agendamento: record.scheduledTime,
        descricao: record.serviceType,
        observacoes: record.notes || 'Retorno agendado pelo painel técnico.'
      });
      if (error) throw error;
      if (svcRow?.id) {
        record.id = svcRow.id;
        await SupabaseApi.createAppointment({
          service_id: svcRow.id,
          cliente_id: record.clientId,
          data: record.date,
          hora: record.scheduledTime,
          status: 'AGENDADO',
          observacoes: record.notes
        }).catch(() => {});

        const cliente = clients.find((cl) => cl.id === record.clientId);
        fireNotify('Agendamento confirmado', `${record.serviceType} em ${record.date.split('-').reverse().join('/')} — consulte a Agenda.`);

        // ENVIO AUTOMATICO: confirmacao do agendamento para o cliente
        try {
          const clienteAg = clients.find((cl) => cl.id === record.clientId);
          if (clienteAg?.phone) {
            AutoWhatsapp.enviar({
              telefone: clienteAg.phone,
              texto: msg('agendamento_criado', {
                cliente: clienteAg.name.split(' ')[0],
                data: record.date.split('-').reverse().join('/'),
                servico: record.serviceType
              })
            }).then((ok) => { if (ok) fireNotify('Confirmação enviada no WhatsApp', `Agendamento enviado para ${clienteAg.name}.`); });
          }
        } catch { /* best-effort */ }
      }
      setAgendamentoTarget(null);
      await syncFromSupabase();
    } catch (err) {
      console.warn('Erro ao agendar no Supabase:', err);
      setToast({ type: 'error', text: 'Não foi possível salvar o agendamento. O formulário foi mantido para tentar novamente.' });
      return;
    }
  };

  const handleDeleteClient = async (clientId: string) => {
    const updatedClients = clients.filter((c) => c.id !== clientId);
    setClients(updatedClients);
    StorageService.saveClients(updatedClients);

    const updatedMaintenances = maintenances.filter((m) => m.clientId !== clientId);
    setMaintenances(updatedMaintenances);
    StorageService.saveMaintenances(updatedMaintenances);

    if (isUuid(clientId)) {
      try {
        await SupabaseApi.deleteCustomer(clientId);
        await syncFromSupabase();
      } catch (err) {
        console.warn('Erro ao excluir cliente no Supabase:', err);
      }
    }
  };

  const handleDeleteAppliance = async (clientId: string, applianceId: string) => {
    const updatedClients = clients.map((c) => {
      if (c.id === clientId) {
        return { ...c, appliances: (c.appliances || []).filter((a) => a.id !== applianceId) };
      }
      return c;
    });
    setClients(updatedClients);
    StorageService.saveClients(updatedClients);

    const updatedMaintenances = maintenances.filter((m) => m.applianceId !== applianceId);
    setMaintenances(updatedMaintenances);
    StorageService.saveMaintenances(updatedMaintenances);

    if (isUuid(applianceId)) {
      try {
        await SupabaseApi.deleteAirConditioner(applianceId);
        await syncFromSupabase();
      } catch (err) {
        console.warn('Erro ao excluir aparelho no Supabase:', err);
      }
    }
  };

  const handleDeleteMaintenance = async (recordId: string) => {
    const anterior = maintenances;
    const updated = maintenances.filter((m) => m.id !== recordId);
    setMaintenances(updated);
    StorageService.saveMaintenances(updated);

    if (isUuid(recordId)) {
      try {
        const { error } = await SupabaseApi.deleteService(recordId);
        if (error) throw error;
        await syncFromSupabase();
        setToast({ type: 'success', text: 'Serviço excluído do banco.' });
      } catch (err) {
        console.warn('Erro ao excluir serviço no Supabase (restaurado local):', err);
        setMaintenances(anterior);
        StorageService.saveMaintenances(anterior);
        setToast({ type: 'error', text: 'Não foi possível excluir do banco — item restaurado.' });
      }
    } else {
      setToast({ type: 'success', text: 'Serviço local excluído.' });
    }
  };

  const handleMarkContacted = async (clientId: string, applianceId: string) => {    const nowStr = format(new Date(), 'dd/MM HH:mm');
    const updated = maintenances.map((m) => {
      if (m.clientId === clientId && m.applianceId === applianceId) {
        return { ...m, contactedAt: nowStr };
      }
      return m;
    });
    setMaintenances(updated);
    StorageService.saveMaintenances(updated);

    // persiste o registro de contato no banco (servico vinculado ao aparelho)
    const registro = maintenances.find((m) => m.clientId === clientId && m.applianceId === applianceId);
    if (registro && isUuid(registro.id)) {
      try {
        // usa as observações BRUTAS do banco — notes da UI vem sem os marcadores internos
        const bruto = servicesList.find((s: any) => s.id === registro.id);
        const { error } = await SupabaseApi.updateService(registro.id, {
          observacoes: ((bruto?.observacoes || registro.notes || '') + ' | [Contato feito ' + nowStr + ']').slice(0, 900)
        });
        if (error) throw error;
      } catch (err) {
        console.warn('Contato mantido localmente:', err);
      }
    }
  };

  const handleSaveProfile = (newProfile: TechnicianProfile) => {
    setProfile(newProfile);
    StorageService.saveProfile(newProfile);
  };

  // PERSISTENCIA DO PERFIL: local + banco (/api/configuracoes) para valer em todas as plataformas
  const persistirPerfil = async (novo: TechnicianProfile) => {
    setProfile(novo);
    StorageService.saveProfile(novo);
    try {
      const { data: sessionData } = await supabase.auth.getSession();
      await fetch('/api/configuracoes', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${sessionData?.session?.access_token || ''}`
        },
        body: JSON.stringify(novo)
      });
    } catch { /* mantém local até reconectar */ }
  };

  // CADASTRO / EDICAO / EXCLUSAO COMPLETA DE QUALQUER TIPO DE SERVICO (fixos e personalizados)
  const handleAddTipo = (dados: DadosTipoServico) => {
    persistirPerfil({ ...profile, tiposServicosCustom: [...(profile.tiposServicosCustom || []), dados] });
    setToast({ type: 'success', text: 'Serviço "' + dados.nome + '" cadastrado no catálogo!' });
  };

  const handleEditTipo = (key: string, dados: DadosTipoServico) => {
    if (key.startsWith('fixo:')) {
      const tipo = key.slice(5);
      const lista = [...(profile.tiposFixosEditados || [])];
      const i = lista.findIndex((e) => e.tipo === tipo);
      if (i >= 0) lista[i] = { tipo, ...dados };
      else lista.push({ tipo, ...dados });
      persistirPerfil({ ...profile, tiposFixosEditados: lista });
      setToast({ type: 'success', text: 'Serviço "' + dados.nome + '" atualizado no catálogo!' });
    } else {
      const idx = Number(key.split(':')[1]);
      const lista = [...(profile.tiposServicosCustom || [])];
      if (!lista[idx]) return;
      lista[idx] = dados;
      persistirPerfil({ ...profile, tiposServicosCustom: lista });
      setToast({ type: 'success', text: 'Serviço "' + dados.nome + '" atualizado no catálogo!' });
    }
  };

  const handleDeleteTipo = (key: string) => {
    if (key.startsWith('fixo:')) {
      const tipo = key.slice(5);
      const removidos = [...(profile.tiposFixosRemovidos || [])];
      if (!removidos.includes(tipo)) removidos.push(tipo);
      persistirPerfil({ ...profile, tiposFixosRemovidos: removidos });
      setToast({ type: 'info', text: 'Serviço "' + tipo + '" removido do catálogo.' });
    } else {
      const idx = Number(key.split(':')[1]);
      const alvo = (profile.tiposServicosCustom || [])[idx];
      persistirPerfil({ ...profile, tiposServicosCustom: (profile.tiposServicosCustom || []).filter((_, i) => i !== idx) });
      setToast({ type: 'info', text: 'Serviço "' + (alvo?.nome || '') + '" removido do catálogo.' });
    }
  };

  // Catálogo único, sincronizado no perfil: criar, editar e excluir tipos em
  // Serviços de Campo atualiza imediatamente desktop, celular, OS, agenda e
  // orçamento, sem telas paralelas de configuração.
  const catalogoTipos = montarCatalogo(profile);

  const handleUpdateServiceStatus = async (serviceId: string, status: string) => {
    try {
      const hoje = format(new Date(), 'yyyy-MM-dd');
      const atual = servicesList.find((s: any) => s.id === serviceId);
      const ciclo: Record<string, string> = {};
      if (status === 'EM_ANDAMENTO') ciclo.data_inicio = hoje;
      if (status === 'CONCLUIDO') ciclo.data_conclusao = hoje;
      if (status === 'CANCELADO') {
        ciclo.data_cancelamento = hoje;
        ciclo.motivo_cancelamento = 'Cancelado pelo técnico';
      }
      const result = await SupabaseApi.updateService(serviceId, { status, ...ciclo });
      if (result.error) {
        // Compatibilidade com bases anteriores à migração: mantém o ciclo auditável nas observações.
        const marcador = status === 'CONCLUIDO'
          ? `[DATA_CONCLUSAO:${hoje}]`
          : status === 'EM_ANDAMENTO'
          ? `[DATA_INICIO:${hoje}]`
          : status === 'CANCELADO'
          ? `[DATA_CANCELAMENTO:${hoje}] [MOTIVO_CANCELAMENTO:Cancelado pelo técnico]`
          : '';
        const observacoes = marcador
          ? `${String(atual?.observacoes || '').replace(new RegExp('\\[DATA_' + (status === 'EM_ANDAMENTO' ? 'INICIO' : status === 'CONCLUIDO' ? 'CONCLUSAO' : 'CANCELAMENTO') + ':[^\\]]+\\]', 'g'), '').replace(/\[MOTIVO_CANCELAMENTO:[^\]]+\]/g, '').trim()} ${marcador}`.trim()
          : undefined;
        const fallback = await SupabaseApi.updateService(serviceId, { status, ...(observacoes !== undefined ? { observacoes } : {}) });
        if (fallback.error) throw fallback.error;
      }
      await syncFromSupabase();
      setToast({ type: 'success', text: status === 'EM_ANDAMENTO' ? 'Serviço iniciado!' : status === 'CONCLUIDO' ? 'Serviço concluído!' : status === 'CANCELADO' ? 'Serviço cancelado.' : 'Status atualizado.' });

      // ENVIO AUTOMATICO: avisa o cliente sobre o andamento do servico dele
      const svc = servicesList.find((s: any) => s.id === serviceId);
      const clienteSvc = clients.find((cl) => cl.id === svc?.cliente_id);
      if (svc && clienteSvc?.phone) {
        const tipo = String(svc.tipo || '').replace('_', ' ');
        const chaveStatus: Record<string, string> = {
          AGENDADO: 'status_agendado',
          EM_ANDAMENTO: 'status_em_andamento',
          CONCLUIDO: 'status_concluido',
          CANCELADO: 'status_cancelado'
        };
        const chave = chaveStatus[status];
        if (chave) {
          AutoWhatsapp.enviar({
            telefone: clienteSvc.phone,
            texto: msg(chave, { cliente: clienteSvc.name.split(' ')[0], servico: tipo })
          }).catch(() => {});
        }
      }
    } catch (e) {
      console.error('Erro ao atualizar status do serviço:', e);
      setToast({ type: 'error', text: 'Não foi possível atualizar a OS. Tente novamente.' });
    }
  };

  // Iniciar serviço a partir de card ou menu
  const handleOpenIniciarServico = (type?: ServiceType) => {
    setShowIniciarServico({ isOpen: true, initialType: type });
  };

  // FINALIZAR SERVIÇO (botão de fim): abre o checklist que conclui a MESMA OS
  // já iniciada/agendada — nunca gera uma segunda ordem para o mesmo pedido.
  const handleFinalizarServico = (serviceId: string) => {
    const svc = servicesList.find((s: any) => s.id === serviceId);
    const cl = clients.find((c) => c.id === svc?.cliente_id);
    if (!svc || !cl) return;
    const ap = cl.appliances?.find((a) => a.id === svc.aparelho_id) || cl.appliances?.[0];
    if (!ap) {
      setToast({ type: 'error', text: 'Cadastre o aparelho na ficha do cliente antes de finalizar.' });
      return;
    }
    const tipo = (svc.tipo === 'OUTRO' && svc.descricao ? svc.descricao : mapTipoSupabaseToLocal(svc.tipo)) as ServiceType;
    setModalChecklist({ client: cl, appliance: ap, serviceType: tipo, serviceId });
  };

  // GERAR ORÇAMENTO: se o cliente já tem orçamento em aberto, oferece EDITAR o
  // atual em vez de gerar outro documento — novo só quando for mesmo outro pedido.
  const abrirOrcamentoPara = (cl: Client) => {
    const aberto = budgets.find((b) => b.clientId === cl.id && b.status === 'pendente');
    if (aberto) {
      const editar = confirm(
        `Este cliente já tem o orçamento ${aberto.numero || 'em aberto'} (R$ ${(aberto.finalValue || 0).toFixed(2)}).\n\nOK = EDITAR o orçamento existente\nCancelar = gerar um NOVO orçamento`
      );
      if (editar) {
        setSelectedBudget(null);
        setEditBudget(aberto);
        setOrcamentoDeChamado(cl);
        setShowNovoOrcamento(true);
        return;
      }
    }
    setEditBudget(null);
    setOrcamentoDeChamado(cl);
    setShowNovoOrcamento(true);
  };

  const handleServiceSelected = async (client: Client, appliance: Appliance, serviceType: ServiceType, budget?: BudgetEstimate) => {
    try {
      const open = servicesList.filter((s: any) => s.cliente_id === client.id && s.aparelho_id === appliance.id && ['PENDENTE', 'AGENDADO', 'EM_ANDAMENTO'].includes(s.status));
      let service = budget?.serviceId ? servicesList.find((s: any) => s.id === budget.serviceId) : open.length === 1 ? open[0] : undefined;
      if (!service && open.length > 1) throw new Error('Há várias OS abertas. Escolha a ordem na aba Chamados da Central de Atendimento antes de iniciar.');
      const today = format(new Date(), 'yyyy-MM-dd');
      if (service) {
        if (!['PENDENTE', 'AGENDADO', 'EM_ANDAMENTO'].includes(service.status)) throw new Error('A OS vinculada já está encerrada.');
        const result = await SupabaseApi.updateService(service.id, { status: 'EM_ANDAMENTO', observacoes: [service.observacoes || '', '[DATA_INICIO:' + today + ']'].join(' ') });
        if (result.error) throw result.error;
      } else {
        const result = await SupabaseApi.createService({ cliente_id: client.id, aparelho_id: appliance.id, tipo: mapServiceTypeToSupabase(serviceType), descricao: serviceType, status: 'EM_ANDAMENTO', valor: budget?.finalValue ?? 0, data_agendamento: today, observacoes: '[DATA_INICIO:' + today + ']' });
        if (result.error) throw result.error;
        service = result.data;
      }
      if (budget && service?.id) { const linked = await SupabaseApi.updateBudget(budget.id, { service_id: service.id }); if (linked.error) throw linked.error; }
      await syncFromSupabase();
      setShowIniciarServico({ isOpen: false });
      setModalChecklist({ client, appliance, serviceType, serviceId: service.id, budgetId: budget?.id });
      fireNotify('Ordem de Serviço iniciada', 'A OS foi salva. Preencha o checklist para concluir; você pode continuar depois.');
    } catch (error: any) { setToast({ type: 'error', text: error.message || 'Não foi possível iniciar o serviço.' }); }
  };

  // CADASTRAR SERVICO DIRETO (sem checklist) — salvo no banco + agenda + aviso
  const handleNovoServico = async (dados: { clientId: string; applianceId: string; tipo: string; data: string; valor: number; status: string; observacoes: string }) => {
    try {
      const { data: svcRow, error } = await SupabaseApi.createService({
        cliente_id: dados.clientId,
        aparelho_id: dados.applianceId,
        tipo: mapServiceTypeToSupabase(dados.tipo),
        status: dados.status as any,
        valor: dados.valor,
        data_agendamento: dados.data,
        descricao: dados.tipo,
        observacoes: dados.observacoes || undefined
      });
      if (error) throw error;
      if (dados.status === 'AGENDADO' && svcRow?.id) {
        await SupabaseApi.createAppointment({ service_id: svcRow.id, cliente_id: dados.clientId, data: dados.data, status: 'AGENDADO', observacoes: dados.observacoes }).catch(() => {});
      }
      await syncFromSupabase();
      fireNotify('Serviço cadastrado', 'O serviço foi salvo no banco e atualizado na agenda quando aplicável.');
      const clienteN = clients.find((cl) => cl.id === dados.clientId);
      if (clienteN?.phone && dados.status !== 'CANCELADO') {
        AutoWhatsapp.enviar({
          telefone: clienteN.phone,
          texto: msg('servico_registrado', {
            cliente: clienteN.name.split(' ')[0],
            servico: dados.tipo,
            data: dados.data.split('-').reverse().join('/')
          })
        }).catch(() => {});
      }
    } catch (err) {
      console.warn('Erro ao cadastrar serviço:', err);
      setToast({ type: 'error', text: 'Erro ao cadastrar serviço.' });
    }
  };

  // REGISTRO RETROATIVO DE HISTORICO: OS concluída no passado (antes do app),
  // sem disparo de WhatsApp, iniciando o ciclo de retorno a partir da data informada
  const handleRegistrarHistorico = async (
    cliente: Client,
    aparelho: Appliance,
    dados: { data: string; tipo: string; valor: number; observacoes?: string; atualizarRetorno: boolean }
  ) => {
    setHistoricoModal(null);
    try {
      const { data: svcRow, error } = await SupabaseApi.createService({
        cliente_id: cliente.id,
        aparelho_id: aparelho.id || undefined,
        tipo: mapServiceTypeToSupabase(dados.tipo),
        status: 'CONCLUIDO',
        valor: dados.valor,
        data_agendamento: dados.data,
        descricao: dados.tipo,
        observacoes: ((dados.observacoes || '') + ' | Histórico anterior registrado manualmente.').trim()
      });
      if (error) throw error;

      if (dados.atualizarRetorno) {
        await SupabaseApi.updateAirConditionerMaintenance(aparelho.id, dados.data);
      }

      SupabaseApi.insertServiceHistory({
        cliente_id: cliente.id,
        aparelho_id: aparelho.id || undefined,
        data: dados.data,
        descricao: dados.tipo,
        observacoes: dados.observacoes,
        valor: dados.valor
      }).catch(() => {});

      await syncFromSupabase();
      fireNotify('Histórico registrado', 'O ciclo de retorno já está em contagem.');

      // abre a OS do histórico para anexar fotos / conferir o comprovante
      if (svcRow?.id) {
        const record: MaintenanceRecord = {
          id: svcRow.id,
          clientId: cliente.id,
          applianceId: aparelho.id,
          date: dados.data,
          returnDate: addMonths(new Date(dados.data + 'T00:00:00'), profile.defaultReturnMonths || 6).toISOString().slice(0, 10),
          serviceType: dados.tipo as ServiceType,
          price: dados.valor,
          paymentMethod: 'PIX',
          warrantyDays: profile.defaultWarrantyDays || 90,
          notes: dados.observacoes,
          status: 'concluido'
        };
        setModalOS({ client: cliente, appliance: aparelho, record });
      }
    } catch (err) {
      console.warn('Erro ao registrar histórico retroativo:', err);
      setToast({ type: 'error', text: 'Erro ao registrar o histórico — tente novamente.' });
    }
  };

  // EDITAR CLIENTE (salva no banco e sincroniza)
  const handleEditClient = async (updated: Client) => {
    try {
      const { error } = await SupabaseApi.updateCustomer(updated.id, {
        nome: updated.name,
        whatsapp: updated.phone,
        endereco: updated.address,
        bairro: updated.neighborhood,
        cidade: updated.city,
        observacoes: updated.notes
      });
      if (error) throw error;
      const updatedClients = clients.map((c) => (c.id === updated.id ? { ...c, ...updated } : c));
      setClients(updatedClients);
      StorageService.saveClients(updatedClients);
      await syncFromSupabase();
      setEditCliente(null);
      fireNotify('Cliente atualizado', 'Os dados do cliente foram sincronizados no banco.');
    } catch (err) {
      console.warn('Erro ao editar cliente:', err);
      setToast({ type: 'error', text: 'Erro ao atualizar cliente no banco.' });
    }
  };

  // EDITAR SERVICO (tipo, data, valor, obs — salva no banco e replica em agenda, OS e PDF)
  const handleEditService = async (id: string, dados: { tipo: string; data: string; valor: number; observacoes: string }) => {
    try {
      const atual = servicesList.find((s: any) => s.id === id);
      // Preserva marcadores de ciclo de vida/garantia/checklist que os documentos usam
      const marcadores = String(atual?.observacoes || '')
        .match(/\[(?:GARANTIA_DIAS|DATA_CONCLUSAO|DATA_INICIO|DATA_CANCELAMENTO|MOTIVO_CANCELAMENTO|PROXIMO_RETORNO|MAO_OBRA|PECAS):[^\]]+\]|\[CHECKLIST:\{[\s\S]*?\}\]/g)?.join(' ') || '';
      const obsFinal = [dados.observacoes || '', marcadores].filter(Boolean).join(' ').trim() || undefined;
      // A data muda o campo certo conforme o status (senão a edição não aparece em lugar nenhum)
      const upd: any = {
        tipo: mapServiceTypeToSupabase(dados.tipo),
        valor: dados.valor,
        descricao: dados.tipo,
        observacoes: obsFinal
      };
      if (atual?.status === 'CONCLUIDO') upd.data_conclusao = dados.data;
      else if (atual?.status === 'CANCELADO') upd.data_cancelamento = dados.data;
      else upd.data_agendamento = dados.data;
      let { error } = await SupabaseApi.updateService(id, upd);
      if (error && (upd.data_conclusao || upd.data_cancelamento)) {
        // Base sem a migration de datas: mantém a data nos marcadores das observações
        const marcador = upd.data_conclusao ? `[DATA_CONCLUSAO:${upd.data_conclusao}]` : `[DATA_CANCELAMENTO:${upd.data_cancelamento}]`;
        delete upd.data_conclusao;
        delete upd.data_cancelamento;
        upd.observacoes = [obsFinal || '', marcador].filter(Boolean).join(' ').trim() || undefined;
        ({ error } = await SupabaseApi.updateService(id, upd));
      }
      if (error) throw error;
      if (upd.data_agendamento) await SupabaseApi.updateAppointmentData(id, dados.data);
      const resultado = await syncFromSupabase();
      setEditServico(null);
      fireNotify('Serviço atualizado', 'A OS, agenda e documentos futuros usarão os dados atualizados.');
      refletemEdicaoNaOSAberta(resultado, id);
    } catch (err) {
      console.warn('Erro ao editar serviço:', err);
      setToast({ type: 'error', text: 'Erro ao atualizar serviço no banco.' });
    }
  };

  // Mantém a OS aberta coerente com o banco após qualquer edição (modal + PDF)
  const refletemEdicaoNaOSAberta = (resultado: { maintenances?: MaintenanceRecord[]; clients?: Client[] }, serviceId?: string) => {
    setModalOS((aberta) => {
      if (!aberta || (serviceId && aberta.record.id !== serviceId)) return aberta;
      const rec = resultado.maintenances?.find((m) => m.id === aberta.record.id) || aberta.record;
      const cl = resultado.clients?.find((c) => c.id === rec.clientId) || aberta.client;
      const ap = cl.appliances?.find((a) => a.id === rec.applianceId) || aberta.appliance;
      return { client: cl, appliance: ap, record: rec };
    });
  };

  // AGENDAR CHAMADO do cliente: data + hora -> AGENDADO + appointment + aviso WhatsApp
  const handleAgendarChamado = async (serviceId: string, data: string, hora: string, observacoes: string) => {
    try {
      const { error } = await SupabaseApi.updateService(serviceId, { status: 'AGENDADO', data_agendamento: data, hora_agendamento: hora });
      if (error) throw error;
      // atualiza appointment existente ou cria
      const appt = appointments.find((a: any) => a.service_id === serviceId);
      if (appt) {
        await SupabaseApi.updateAppointmentData(serviceId, data, hora);
      } else {
        const svc = servicesList.find((s: any) => s.id === serviceId);
        if (svc) {
          await SupabaseApi.createAppointment({ service_id: serviceId, cliente_id: svc.cliente_id, data, hora, status: 'AGENDADO', observacoes });
        }
      }
      await syncFromSupabase();
      setAgendarChamado(null);
      const dataBr = data.split('-').reverse().join('/');
      const clienteAgenda = clients.find((cl) => cl.id === servicesList.find((s: any) => s.id === serviceId)?.cliente_id);
      fireNotify('Atendimento agendado', dataBr + ' às ' + hora);

      // AVISO AUTOMATICO ao cliente
      const svc = servicesList.find((s: any) => s.id === serviceId);
      const clienteAg = clients.find((cl) => cl.id === svc?.cliente_id);
      if (clienteAg?.phone) {
        AutoWhatsapp.enviar({
          telefone: clienteAg.phone,
          texto: msg('chamado_agendado', {
            cliente: clienteAg.name.split(' ')[0],
            servico: String(svc?.tipo || '').replace('_', ' '),
            data: dataBr,
            hora
          })
        }).then((ok) => { if (ok) setToast({ type: 'success', text: 'Confirmação enviada no WhatsApp do cliente!' }); });
      }
    } catch (err) {
      console.warn('Erro ao agendar chamado:', err);
      setToast({ type: 'error', text: 'Erro ao agendar — tente novamente.' });
    }
  };

  // AGENDAR ORCAMENTO APROVADO: cria servico AGENDADO + agenda + avisa cliente
  const handleAgendarOrcamento = async (budget: BudgetEstimate, data: string, hora: string) => {
    try {
      const cliente = clients.find((cl) => cl.id === budget.clientId);
      const isUuid = (v: string) => /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(v || '');
      if (!cliente || !isUuid(cliente.id)) {
        setToast({ type: 'error', text: 'Cliente do orçamento não encontrado no banco.' });
        return;
      }
      // se o cliente tem chamado PENDENTE, é ele que vira o agendamento (sem orfãos)
      if (budget.status !== 'aprovado') throw new Error('Aprove o orçamento antes de agendar.');
      const chamadoPendente = budget.serviceId
        ? servicesList.find((s: any) => s.id === budget.serviceId && s.cliente_id === cliente.id)
        : servicesList.find((s: any) => s.cliente_id === cliente.id && s.status === 'PENDENTE' && !!budget.applianceId && s.aparelho_id === budget.applianceId);
      if (chamadoPendente && ['CONCLUIDO', 'CANCELADO'].includes(chamadoPendente.status)) throw new Error('A OS deste orçamento já foi encerrada.');
      let svcRow: any = null;
      if (chamadoPendente) {
        const { error: errUpd } = await SupabaseApi.updateService(chamadoPendente.id, {
          status: 'AGENDADO',
          data_agendamento: data,
          hora_agendamento: hora,
          valor: budget.finalValue,
          descricao: budget.items?.[0]?.description || budget.applianceDesc || 'Serviço aprovado (orçamento)'
        });
        if (errUpd) throw errUpd;
        await SupabaseApi.updateAppointmentData(chamadoPendente.id, data, hora);
        svcRow = { id: chamadoPendente.id };
      } else {
        const { data: createdSvc, error } = await SupabaseApi.createService({
          cliente_id: cliente.id,
          aparelho_id: budget.applianceId || (cliente.appliances?.length === 1 ? cliente.appliances[0].id : undefined),
          tipo: mapServiceTypeToSupabase(budget.items?.[0]?.description || 'OUTRO'),
          status: 'AGENDADO',
          valor: budget.finalValue,
          data_agendamento: data,
          hora_agendamento: hora,
          descricao: budget.items?.[0]?.description || budget.applianceDesc || 'Serviço aprovado (orçamento)',
          observacoes: 'Gerado do orçamento aprovado — valor R$ ' + budget.finalValue.toFixed(2)
        });
        if (error) throw error;
        svcRow = createdSvc;
        if (svcRow?.id) {
          await SupabaseApi.createAppointment({ service_id: svcRow.id, cliente_id: cliente.id, data, hora, status: 'AGENDADO' }).catch(() => {});
        }
      }
      await syncFromSupabase();
      setAgendarOrcamento(null);
      // vínculo orçamento→OS para o faturamento não contar o valor em dobro
      if (svcRow?.id && isUuid(budget.id)) {
        try { await SupabaseApi.updateBudget(budget.id, { service_id: svcRow.id }); } catch { /* best-effort */ }
      }
      const dataBr = data.split('-').reverse().join('/');
      fireNotify('Serviço agendado', dataBr + ' às ' + hora);

      // avisa o cliente com link
      if (cliente.phone) {
        AutoWhatsapp.enviar({
          telefone: cliente.phone,
          texto: msg('orcamento_agendado', {
            cliente: cliente.name.split(' ')[0],
            data: dataBr,
            hora,
            valor: 'R$ ' + budget.finalValue.toFixed(2)
          })
        }).then((ok) => { if (ok) setToast({ type: 'success', text: 'Cliente avisado no WhatsApp!' }); });
      }
    } catch (err) {
      console.warn('Erro ao agendar orçamento:', err);
      setToast({ type: 'error', text: 'Erro ao agendar o orçamento aprovado.' });
    }
  };

  // EDITAR OS por completo (status, data, valor, garantia) — salvo no banco
  const handleEditOS = async (serviceId: string, campos: { status?: string; data?: string; valor?: number; garantiaDias?: number; observacoes?: string }) => {
    try {
      const atual = servicesList.find((s: any) => s.id === serviceId);
      const upd: any = {};
      if (campos.status) upd.status = campos.status;
      if (campos.valor !== undefined) upd.valor = campos.valor;
      if (campos.garantiaDias !== undefined) {
        const base = String(atual?.observacoes || '').replace(/\[GARANTIA_DIAS:\d+\]/g, '').replace(/Garantia de \d+ dias\.?/gi, '').trim();
        upd.observacoes = `${base} [GARANTIA_DIAS:${Math.max(0, Number(campos.garantiaDias) || 0)}] Garantia de ${Math.max(0, Number(campos.garantiaDias) || 0)} dias.`.trim();
      }
      // A "data do serviço" tem campo próprio por status: agendada = data_agendamento,
      // concluída = data_conclusao, cancelada = data_cancelamento (replica em agenda, OS e PDF)
      const statusFinal = campos.status || (atual?.status === 'CONCLUIDO' ? 'CONCLUIDO' : atual?.status === 'CANCELADO' ? 'CANCELADO' : atual?.status === 'EM_ANDAMENTO' ? 'EM_ANDAMENTO' : 'AGENDADO');
      if (campos.data) {
        if (statusFinal === 'CONCLUIDO') upd.data_conclusao = campos.data;
        else if (statusFinal === 'CANCELADO') upd.data_cancelamento = campos.data;
        else upd.data_agendamento = campos.data;
      }
      let { error } = await SupabaseApi.updateService(serviceId, upd);
      if (error && (upd.data_conclusao || upd.data_cancelamento)) {
        // Base sem a migration de datas: grava a data como marcador nas observações
        const marcador = upd.data_conclusao ? `[DATA_CONCLUSAO:${upd.data_conclusao}]` : `[DATA_CANCELAMENTO:${upd.data_cancelamento}]`;
        delete upd.data_conclusao;
        delete upd.data_cancelamento;
        const obsBase = String(atual?.observacoes || '').replace(/\[DATA_(CONCLUSAO|INICIO|CANCELAMENTO):\d{4}-\d{2}-\d{2}\]/g, '').trim();
        upd.observacoes = [upd.observacoes || obsBase, marcador].filter(Boolean).join(' ').trim();
        ({ error } = await SupabaseApi.updateService(serviceId, upd));
      }
      if (error) throw error;
      if (upd.data_agendamento) await SupabaseApi.updateAppointmentData(serviceId, upd.data_agendamento);
      const resultado = await syncFromSupabase();
      refletemEdicaoNaOSAberta(resultado, serviceId);
      setToast({ type: 'success', text: 'OS atualizada no banco!' });
      if (campos.status === 'CONCLUIDO' && clienteNotificavel(serviceId)) {
        AutoWhatsapp.enviar({
          telefone: clienteNotificavel(serviceId)!.phone,
          texto: msg('os_status_atualizado', { cliente: clienteNotificavel(serviceId)!.name.split(' ')[0], status: campos.status })
        }).catch(() => {});
      }
    } catch (err) {
      console.warn('Erro ao editar OS:', err);
      setToast({ type: 'error', text: 'Erro ao salvar a edição da OS.' });
    }
  };

  const clienteNotificavel = (serviceId: string): Client | null => {
    const svc = servicesList.find((s: any) => s.id === serviceId);
    return clients.find((cl) => cl.id === svc?.cliente_id) || null;
  };

  // MARCAR ORCAMENTO COMO RECEBIDO — mescla com o JSON ATUAL do banco:
  // o cliente pode ter assinado depois do último sync e um rewrite cego do
  // snapshot local APAGARIA a assinatura digital dele.
  const handleMarcarRecebido = async (budget: BudgetEstimate) => {
    if (!isUuid(budget.id)) {
      setToast({ type: 'error', text: 'Orçamento local — salve-o no banco primeiro.' });
      return;
    }
    try {
      const { data: row } = await supabase
        .from('budgets')
        .select('descricao, service_id')
        .eq('id', budget.id)
        .maybeSingle();
      let meta: any = {};
      try { meta = typeof row?.descricao === 'string' ? JSON.parse(row.descricao) : (row?.descricao || {}); } catch { meta = {}; }
      const novo = {
        ...meta,
        // campos que faltavam em registros antigos
        numero: meta.numero || budget.numero || '',
        applianceId: meta.applianceId || budget.applianceId || null,
        service_id: (row as any)?.service_id || budget.serviceId || meta.service_id || null,
        // assinatura SEMPRE a do banco — o snapshot local nunca apaga a do cliente
        assinatura: meta.assinatura || budget.assinatura || null,
        assinatura_em: meta.assinatura_em || budget.assinaturaEm || null,
        pago: true,
        pago_em: new Date().toISOString(),
        valor_recebido: budget.finalValue
      };
      const { error } = await SupabaseApi.updateBudget(budget.id, { descricao: JSON.stringify(novo) });
      if (error) throw error;
      await syncFromSupabase();
      setToast({ type: 'success', text: '💰 R$ ' + budget.finalValue.toFixed(2) + ' marcado como recebido!' });
    } catch (err) {
      console.warn('Erro ao marcar recebido:', err);
      setToast({ type: 'error', text: 'Erro ao marcar como recebido.' });
    }
  };

  // Reagendar (altera a data do serviço + da agenda no banco — de qualquer plataforma)
  const handleReschedule = async (serviceId: string, novaData: string) => {
    try {
      const { error } = await SupabaseApi.updateServiceDate(serviceId, novaData);
      if (error) throw error;
      await SupabaseApi.updateAppointmentData(serviceId, novaData);
      await syncFromSupabase();
      const dataBr = novaData.split('-').reverse().join('/');
      fireNotify('Agendamento alterado', 'Nova data: ' + dataBr);

      // AVISO AUTOMATICO ao cliente: servico REAGENDADO
      const svc = servicesList.find((s: any) => s.id === serviceId);
      const clienteR = clients.find((cl) => cl.id === svc?.cliente_id);
      if (clienteR?.phone) {
        AutoWhatsapp.enviar({
          telefone: clienteR.phone,
          texto: msg('reagendado', {
            cliente: clienteR.name.split(' ')[0],
            servico: String(svc?.tipo || '').replace('_', ' '),
            data: dataBr
          })
        }).then((ok) => { if (ok) setToast({ type: 'success', text: 'Aviso de reagendamento enviado ao cliente!' }); });
      }
    } catch (err) {
      console.warn('Erro ao reagendar:', err);
      setToast({ type: 'error', text: 'Erro ao reagendar — tente novamente.' });
    }
  };

  // Budget handlers (tabela budgets no Supabase)
  const handleSaveBudget = async (newBudget: BudgetEstimate) => {
    setShowNovoOrcamento(false);

    try {
      // Resolve o cliente real no banco (FK obrigatória)
      let clienteId = isUuid(newBudget.clientId) && clients.some((c) => c.id === newBudget.clientId)
        ? newBudget.clientId
        : '';
      if (!clienteId) {
        const byName = clients.find(
          (c) => c.name.trim().toLowerCase() === newBudget.clientName.trim().toLowerCase()
        );
        if (byName) clienteId = byName.id;
      }
      if (!clienteId) {
        const { data: newCust, error: custErr } = await SupabaseApi.createCustomer({
          nome: newBudget.clientName,
          whatsapp: newBudget.clientPhone,
          ativo: true
        });
        if (custErr) throw custErr;
        clienteId = newCust!.id;
      }

      // MODO EDICAO: atualiza o orçamento existente no banco
      if (editBudget && editBudget.id === newBudget.id && isUuid(newBudget.id)) {
        const payloadEd = mapLocalBudgetToSupabase(newBudget, 'EDIT', clienteId, profile.name || 'Inovar Refrigeração');
        const { error: errUp } = await SupabaseApi.updateBudget(newBudget.id, {
          data: payloadEd.data,
          validade: payloadEd.validade,
          descricao: payloadEd.descricao,
          tipo_servico: payloadEd.tipo_servico,
          valor_mao_obra: payloadEd.valor_mao_obra,
          valor_material: payloadEd.valor_material,
          valor_total: payloadEd.valor_total,
          condicoes: payloadEd.condicoes
        });
        if (errUp) throw errUp;
        setEditBudget(null);
        await syncFromSupabase();
        setSelectedBudget(newBudget);
        fireNotify('Orçamento atualizado', 'A proposta foi sincronizada no banco.');
        return;
      }

      const numero = await SupabaseApi.proximoNumeroOrcamento();
      const payload = mapLocalBudgetToSupabase(newBudget, numero, clienteId, profile.name || 'Inovar Refrigeração');
      const { data, error } = await SupabaseApi.createBudget(payload);
      if (error) throw error;
      if (data?.id) newBudget.id = data.id;
      newBudget.numero = numero;

      await syncFromSupabase();
      setSelectedBudget(newBudget);
      fireNotify('Orçamento gerado', `Proposta nº ${numero} criada para ${newBudget.clientName}.`);

      // ENVIO AUTOMATICO: proposta em PDF para o WhatsApp do cliente
      try {
        const cliente = clients.find((cl) => cl.id === clienteId) || clients.find((cl) => cl.name.trim().toLowerCase() === newBudget.clientName.trim().toLowerCase());
        const telefone = newBudget.clientPhone || cliente?.phone;
        if (telefone) {
          const doc = await BudgetPdfService.buildBudgetPdf(newBudget, profile);
          const base64 = doc.output('datauristring');
          AutoWhatsapp.orcamento({
            telefone,
            numero,
            clienteNome: newBudget.clientName,
            total: newBudget.finalValue,
            pdfBase64: base64,
            nomePdf: 'ORCAMENTO_' + numero + '.pdf',
            texto: msg('orcamento_criado', {
              cliente: newBudget.clientName.split(' ')[0],
              numero,
              valor: 'R$ ' + newBudget.finalValue.toFixed(2)
            })
          }).then((ok) => {
            if (ok) fireNotify('Proposta enviada no WhatsApp', `O orçamento nº ${numero} foi enviado para ${newBudget.clientName}.`);
          });
        }
      } catch { /* envio automatico e best-effort */ }
    } catch (err) {
      console.warn('Erro ao salvar orçamento no Supabase (mantido local):', err);
      const updated = BudgetService.createBudget(newBudget);
      setBudgets(updated);
      setSelectedBudget(newBudget);
    }
  };

  const handleUpdateBudgetStatus = async (budgetId: string, status: 'pendente' | 'aprovado' | 'recusado') => {
    const updated = BudgetService.updateBudgetStatus(budgetId, status);
    setBudgets(updated);
    if (selectedBudget && selectedBudget.id === budgetId) {
      setSelectedBudget({ ...selectedBudget, status });
    }

    if (isUuid(budgetId)) {
      const dbStatus = status === 'aprovado' ? 'APROVADO' : status === 'recusado' ? 'RECUSADO' : 'ENVIADO';
      try {
        await SupabaseApi.updateBudgetStatus(budgetId, dbStatus);
        await syncFromSupabase();
      } catch (err) {
        console.warn('Erro ao atualizar status no Supabase:', err);
      }
    }
  };

  const handleDeleteBudget = async (budgetId: string) => {
    const updated = BudgetService.deleteBudget(budgetId);
    setBudgets(updated);
    setSelectedBudget(null);

    if (isUuid(budgetId)) {
      try {
        await SupabaseApi.deleteBudget(budgetId);
        await syncFromSupabase();
      } catch (err) {
        console.warn('Erro ao excluir orçamento no Supabase:', err);
      }
    }
  };

  const handleIniciarServicoAprovado = async (budget: BudgetEstimate) => {
    setSelectedBudget(null);
    // cliente exato do orçamento (e o aparelho vinculado, se houver) — nunca um
    // clients[0] qualquer, que anexaria o serviço ao cliente errado
    const client = clients.find((c) => c.id === budget.clientId);
    const appliance = client?.appliances?.find((a) => a.id === budget.applianceId) || (client?.appliances?.length === 1 ? client.appliances[0] : undefined);
    if (client && appliance) {
      const descOrcamento = budget.items?.[0]?.description || '';
      const tipoCatalogo = catalogoTipos.find((t) => t.nome === descOrcamento || t.fixo === descOrcamento);
      await handleServiceSelected(client, appliance, ((tipoCatalogo?.fixo || tipoCatalogo?.nome || 'Instalação') as ServiceType), budget);
    } else {
      setToast({ type: 'error', text: 'Cadastre o cliente e o aparelho antes de iniciar o serviço deste orçamento.' });
      setShowIniciarServico({ isOpen: true });
    }
  };

  // Render content
  const renderContent = () => {
    // Carregando sessão
    if (!authChecked) {
      return (
        <div className="flex flex-col items-center justify-center py-24 text-slate-400 gap-3">
          <div className="w-10 h-10 border-3 border-inovar-yellow border-t-transparent rounded-full animate-spin" style={{ borderWidth: 3 }}></div>
          <span className="text-sm font-medium">Conectando ao Inovar...</span>
        </div>
      );
    }

    // Sem sessão: tela de boas-vindas (dados reais só aparecem logados — RLS)
    if (!userProfile) {
      return (
        <div className="flex flex-col items-center justify-center py-16 px-4 text-center">
          <div className="w-full max-w-sm bg-slate-900/95 rounded-3xl border border-slate-800 shadow-2xl p-8">
            <div className="flex justify-center mb-5">
              <Logo size="lg" invertido={true} />
            </div>
            <h2 className="text-xl font-extrabold text-white tracking-tight">
              Bem-vindo ao InovarApp
            </h2>
            <p className="text-xs text-slate-400 mt-2 leading-relaxed">
              Acompanhe seus aparelhos, solicite atendimentos e consulte garantias.
              Clientes e técnicos entram com o mesmo login usado no site da Inovar.
            </p>

            <div className="mt-6 space-y-2.5">
              <button
                onClick={() => { setShowAuthModalSignup(false); setShowAuthModal(true); }}
                className="w-full py-3 bg-inovar-yellow hover:brightness-105 active:scale-[0.99] text-inovar-navy font-bold rounded-xl text-sm shadow-lg transition-all flex items-center justify-center gap-2"
              >
                <LogIn className="w-4 h-4" />
                <span>Entrar no App</span>
              </button>
              <button
                onClick={() => { setShowAuthModalSignup(true); setShowAuthModal(true); }}
                className="w-full py-3 bg-slate-800 hover:bg-slate-700 active:scale-[0.99] text-slate-100 font-bold rounded-xl text-sm border border-slate-700 transition-all"
              >
                Sou Cliente • Criar Minha Conta
              </button>
            </div>

            <div className="mt-6 pt-4 border-t border-slate-800/80 grid grid-cols-3 gap-2 text-[10px] text-slate-500">
              <div className="flex flex-col items-center gap-1">
                <ShieldCheck className="w-4 h-4 text-emerald-500" />
                <span>Dados protegidos</span>
              </div>
              <div className="flex flex-col items-center gap-1">
                <Wind className="w-4 h-4 text-sky-400" />
                <span>Todos os serviços</span>
              </div>
              <div className="flex flex-col items-center gap-1">
                <Smartphone className="w-4 h-4 text-inovar-yellow" />
                <span>Site + App sincronizados</span>
              </div>
            </div>
          </div>
        </div>
      );
    }

    if (userProfile?.tipo === 'CLIENTE') {
      return (
        <ClientePortal
          profile={userProfile}
          onOpenAgendamento={() => setShowNovoCliente(true)}
          onRefresh={syncFromSupabase}
        />
      );
    }

    const chamadosContent = (<SolicitacoesTab
            services={servicesList}
            onUpdateStatus={handleUpdateServiceStatus}
            onAgendar={(ch) => setAgendarChamado(ch)}
            onGerarOrcamento={(cl) => abrirOrcamentoPara(cl)}
            onExcluirServico={(id) => handleDeleteMaintenance(id)}
            onOpenChecklist={(c, a) => setModalChecklist({ client: c, appliance: a, serviceType: 'Limpeza de Ar' })}
          />);
    const historicoContent = (<div className="bg-white rounded-2xl border border-slate-200 shadow-xl overflow-hidden text-slate-900">
            <div className="bg-slate-50 p-4 border-b border-slate-200 flex items-center justify-between flex-wrap gap-2">
              <div className="flex items-center gap-2">
                <div className="p-1.5 bg-blue-100 text-blue-700 rounded-lg">
                  <FileText className="w-4 h-4" />
                </div>
                <h3 className="font-bold text-sm text-slate-800">Histórico de Ordens de Serviço & Garantias</h3>
              </div>
              <div className="flex items-center gap-2 flex-wrap">
                <button
                  onClick={() => setHistoricoModal({})}
                  className="px-3 py-1.5 bg-violet-600 hover:bg-violet-500 text-white rounded-lg text-[11px] font-extrabold flex items-center gap-1.5 transition-colors active:scale-95"
                  title="Registrar serviço feito antes do app (importação de histórico)"
                >
                  <History className="w-3.5 h-3.5" />
                  Registrar OS Anterior
                </button>
                <span className="text-xs font-semibold text-slate-500 bg-slate-200 px-2 py-0.5 rounded-full">
                  {maintenances.length} OS emitidas
                </span>
              </div>
            </div>

            <div className="divide-y divide-slate-100">
              {maintenances.length === 0 ? (
                <div className="p-8 text-center text-slate-400 text-xs">
                  Nenhuma Ordem de Serviço registrada ainda. Conclua um checklist de serviço para gerar a primeira OS!
                </div>
              ) : (
                maintenances.map((m) => {
                  const client = clients.find((c) => c.id === m.clientId);
                  const appliance = client?.appliances?.find((a) => a.id === m.applianceId);
                  if (!client || !appliance) return null;
                  // Datas opcionais (retorno só existe em OS concluída) — sem isso a aba quebra
                  const dataRealizada = m.date ? format(parseISO(m.date), 'dd/MM/yyyy') : '—';
                  const dataRetorno = m.returnDate ? format(parseISO(m.returnDate), 'dd/MM/yyyy') : '—';
                  const rotuloStatus = m.status === 'concluido' ? 'Concluída' : m.status === 'em_andamento' ? 'Em andamento' : m.status === 'cancelado' ? 'Cancelada' : 'Agendada';
                  const classeStatus = m.status === 'concluido' ? 'bg-emerald-100 text-emerald-800' : m.status === 'em_andamento' ? 'bg-violet-100 text-violet-800' : m.status === 'cancelado' ? 'bg-red-100 text-red-700' : 'bg-blue-100 text-blue-800';

                  return (
                    <div
                      key={m.id}
                      onClick={() => setModalOS({ client, appliance, record: m })}
                      className="p-3.5 hover:bg-slate-50 cursor-pointer flex items-center justify-between transition-colors text-xs"
                    >
                      <div>
                        <div className="flex items-center gap-2">
                          <span className="font-bold text-slate-800 text-sm">{client.name}</span>
                          <span className="px-2 py-0.5 rounded-full text-[10px] font-semibold bg-emerald-100 text-emerald-800">
                            OS #{m.id.toUpperCase().slice(0, 8)}
                          </span>
                          <span className={`px-2 py-0.5 rounded-full text-[10px] font-semibold ${classeStatus}`}>
                            {rotuloStatus}
                          </span>
                        </div>
                        <p className="text-slate-500 mt-0.5">
                          {appliance.brand} {appliance.capacityBtu} BTUs ({appliance.room}) • {m.serviceType}
                        </p>
                        <p className="text-[11px] text-slate-400 mt-0.5">
                          Realizado em: {dataRealizada} • Retorno: {dataRetorno}
                        </p>
                        {(() => {
                          const partes = [
                            m.scheduledDate ? `Agendado: ${format(parseISO(m.scheduledDate), 'dd/MM/yyyy')}` : '',
                            m.startedAt ? `Início: ${format(parseISO(m.startedAt), 'dd/MM/yyyy')}` : '',
                            m.completedAt ? `Conclusão: ${format(parseISO(m.completedAt), 'dd/MM/yyyy')}` : '',
                            m.cancelledAt ? `Cancelamento: ${format(parseISO(m.cancelledAt), 'dd/MM/yyyy')}` : ''
                          ].filter(Boolean);
                          return partes.length > 0 ? (
                            <p className="text-[10px] text-slate-400">{partes.join(' • ')}</p>
                          ) : null;
                        })()}
                      </div>

                      <div className="text-right shrink-0">
                        <span className="font-extrabold text-sm text-slate-900 block">
                          R$ {m.price.toFixed(2)}
                        </span>
                        <span className="text-[10px] font-semibold text-emerald-600 bg-emerald-50 px-1.5 py-0.5 rounded">
                          Garantia {m.warrantyDays} dias
                        </span>
                        {/* Ações rápidas de status direto na linha da OS */}
                        {(m.status === 'agendado' || m.status === 'em_andamento') && (
                          <div className="mt-1.5 flex items-center justify-end gap-1 flex-wrap" onClick={(e) => e.stopPropagation()}>
                            {m.status === 'agendado' && (
                              <button
                                onClick={() => handleUpdateServiceStatus(m.id, 'EM_ANDAMENTO')}
                                title="Iniciar execução agora"
                                className="px-2 py-1 bg-purple-50 hover:bg-purple-100 text-purple-700 rounded-lg text-[10px] font-bold border border-purple-100 transition-colors"
                              >
                                ▶ Iniciar
                              </button>
                            )}
                            <button
                              onClick={() => handleFinalizarServico(m.id)}
                              title="Finalizar: checklist, valor e garantia nesta mesma OS"
                              className="px-2 py-1 bg-emerald-50 hover:bg-emerald-100 text-emerald-700 rounded-lg text-[10px] font-bold border border-emerald-100 transition-colors"
                            >
                              ✓ Finalizar
                            </button>
                            <button
                              onClick={() => { if (confirm('Cancelar esta OS?')) handleUpdateServiceStatus(m.id, 'CANCELADO'); }}
                              title="Cancelar OS"
                              className="px-2 py-1 bg-red-50 hover:bg-red-100 text-red-600 rounded-lg text-[10px] font-bold border border-red-100 transition-colors"
                            >
                              ✕ Cancelar
                            </button>
                          </div>
                        )}
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </div>);

    return (
      <div className="space-y-3.5">
        {activeTab === 'inicio' && (
          <div className="space-y-3.5">
            {/* KPIs do painel */}
            <StatsBar
              stats={stats}
              totalOS={maintenances.length}
              faturamento={faturamentoTotal}
              onNavegar={(destino) => {
                if (!destino) return;
                const abrirFila = (section: 'fila' | 'chamados' | 'historico', filter: typeof filaFilter = 'todos') => {
                  setActiveTab('inicio');
                  setFilaSection(section);
                  setFilaFilter(filter);
                  setTimeout(() => document.getElementById('fila-de-retornos')?.scrollIntoView({ behavior: 'smooth' }), 50);
                };
                if (destino === 'fila-chamados') {
                  abrirFila('chamados', 'chamados');
                  return;
                }
                if (destino === 'fila-historico') {
                  abrirFila('historico');
                  return;
                }
                if (destino === 'retornos-atrasados') {
                  abrirFila('fila', 'atrasado');
                  return;
                }
                if (destino === 'retornos-semana') {
                  abrirFila('fila', 'esta_semana');
                  return;
                }
                if (destino === 'retornos-sem-historico') {
                  abrirFila('fila', 'sem_historico');
                  return;
                }
                if (destino === 'chamados-fila') {
                  abrirFila('fila', 'chamados');
                  return;
                }
                if (destino === 'proximos-retornos') {
                  abrirFila('fila');
                  return;
                }
                setActiveTab(destino as any);
                window.scrollTo({ top: 0, behavior: 'smooth' });
              }}
            />

            {/* CARDS DE ACESSO RÁPIDO A SERVIÇOS NO TOPO */}
            <div className="bg-white rounded-2xl p-3 border border-slate-200 shadow-sm text-slate-900">
              <div className="flex items-center justify-between mb-2.5 px-1">
                <div className="flex items-center gap-1.5">
                  <div className="p-1 bg-sky-500/20 text-sky-400 rounded-md">
                    <Wrench className="w-3.5 h-3.5" />
                  </div>
                  <span className="text-[11px] font-bold text-slate-700 tracking-wide uppercase">
                    Acesso Rápido a Serviços de Campo
                  </span>
                </div>
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => setShowNovoOrcamento(true)}
                    className="text-[10px] text-inovar-yellow hover:underline font-bold flex items-center gap-1"
                  >
                    <Calculator className="w-3 h-3" />
                    <span>+ Novo Orçamento</span>
                  </button>
                  <button
                    onClick={() => setActiveTab('servicos')}
                    className="text-[10px] text-sky-400 hover:text-sky-300 font-semibold flex items-center gap-0.5"
                  >
                    Ver Todos →
                  </button>
                </div>
              </div>

              <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs">
                {/* Atalhos seguem o catálogo editável (tipos fixos + personalizados) */}
                {catalogoTipos.slice(0, 4).map((t) => {
                  const Icon = t.card?.icon || Wrench;
                  const cor = t.card?.color || 'sky';
                  const corBox =
                    cor === 'emerald' ? 'bg-emerald-500/20 text-emerald-400 border-emerald-500/30'
                    : cor === 'amber' ? 'bg-amber-500/20 text-amber-400 border-amber-500/30'
                    : cor === 'purple' ? 'bg-purple-500/20 text-purple-400 border-purple-500/30'
                    : cor === 'blue' ? 'bg-blue-500/20 text-blue-400 border-blue-500/30'
                    : cor === 'slate' ? 'bg-slate-500/20 text-slate-300 border-slate-500/30'
                    : 'bg-sky-500/20 text-sky-400 border-sky-500/30';
                  return (
                    <button
                      key={t.key}
                      onClick={() => handleOpenIniciarServico((t.fixo || t.nome) as ServiceType)}
                      className="min-h-14 p-2.5 bg-slate-50 hover:bg-sky-50 border border-slate-200 hover:border-sky-300 rounded-xl flex items-center gap-2.5 transition-all active:scale-95 group text-left shadow-sm"
                    >
                      <div className={`w-9 h-9 rounded-lg flex items-center justify-center shrink-0 group-hover:scale-105 transition-transform border ${corBox}`}>
                        <Icon className="w-4 h-4" />
                      </div>
                      <div className="min-w-0">
                        <span className="font-bold text-slate-800 block text-[11px] truncate">{t.nome}</span>
                        <span className="text-[10px] text-slate-600 block truncate">
                          {t.preco > 0 ? 'R$ ' + t.preco.toLocaleString('pt-BR') : t.card?.badge || 'Personalizado'}
                        </span>
                      </div>
                    </button>
                  );
                })}
              </div>
            </div>

            {/* Dashboard principal: fila de contatos. Agendamentos e retornos têm telas próprias. */}
            <div className="space-y-4">
                <QuemPrecisoChamar
                  queueItems={sortedQueue}
                  chamadosContent={chamadosContent}
                  historicoContent={historicoContent}
                  chamadosCount={servicesList.filter((s: any) => s.status === "PENDENTE").length}
                  historicoCount={maintenances.length}
                  profile={profile}
                  maintenances={maintenances}
                  onOpenChecklist={(c, a) => setModalChecklist({ client: c, appliance: a, serviceType: 'Limpeza de Ar' })}
                  onOpenFicha={(c, a) => setModalFicha({ client: c, appliance: a })}
                  onOpenAgendamento={(c, a) => setAgendamentoTarget({ client: c, appliance: a })}
                  onMarkContacted={handleMarkContacted}
                  onGerarOrcamento={(cl) => abrirOrcamentoPara(cl)}
                  onAbrirOS={(cl, ap, rec) => setModalOS({ client: cl, appliance: ap, record: rec })}
                  onRegistrarHistorico={(c, a) => setHistoricoModal({ client: c, appliance: a })}
                  servicesLive={servicesList}
                  budgets={budgets}
                  onAbrirOrcamento={(b) => setSelectedBudget(b)}
                  onUpdateStatus={handleUpdateServiceStatus}
                  onFinalizarServico={handleFinalizarServico}
                  onAgendarOrcamento={(b) => setAgendarOrcamento(b)}
                  activeFilter={filaFilter}
                  onFilterChange={setFilaFilter}
                  sectionRequest={filaSection}
                  onAtenderChamado={(serviceId) => {
                    const s: any = servicesList.find((x: any) => x.id === serviceId);
                    if (!s) return;
                    const cl = clients.find((c) => c.id === s.cliente_id);
                    const ap = cl?.appliances?.find((a) => a.id === s.aparelho_id);
                    setAgendarChamado({
                      id: serviceId,
                      cliente: cl?.name || 'Cliente',
                      servico: String(s.descricao || s.tipo || 'Serviço'),
                      aparelho: ap ? `${ap.brand} ${ap.capacityBtu} BTUs` : 'Aparelho do cliente',
                      telefone: cl?.phone
                    });
                  }}
                />
                <div className="grid grid-cols-1 xl:grid-cols-2 gap-4 items-start">
                  <ProximosAgendamentos events={appointmentEvents}
                    onSelectEvent={(c, a, m) => setModalOS({ client: c, appliance: a, record: m })}
                    onIniciar={(id) => handleUpdateServiceStatus(id, 'EM_ANDAMENTO')}
                    onFinalizar={handleFinalizarServico}
                    onCancelar={(id) => handleUpdateServiceStatus(id, 'CANCELADO')} />
                  <ProximosRetornos events={returnEvents}
                    onSelectEvent={(c, a) => setAgendamentoTarget({ client: c, appliance: a })}
                    onOpenChecklist={(c, a) => setModalChecklist({ client: c, appliance: a, serviceType: 'Limpeza de Ar' })} />
                </div>
            </div>
          </div>
        )}

        {activeTab === 'proximos-agendamentos' && (
          <ProximosAgendamentos
            events={appointmentEvents}
            onSelectEvent={(c, a, m) => setModalOS({ client: c, appliance: a, record: m })}
            onIniciar={(id) => handleUpdateServiceStatus(id, 'EM_ANDAMENTO')}
            onFinalizar={handleFinalizarServico}
            onCancelar={(id) => handleUpdateServiceStatus(id, 'CANCELADO')}
          />
        )}

        {activeTab === 'proximos-retornos' && (
          <ProximosRetornos events={returnEvents}
            onSelectEvent={(c, a) => setAgendamentoTarget({ client: c, appliance: a })}
            onOpenChecklist={(c, a) => setModalChecklist({ client: c, appliance: a, serviceType: 'Limpeza de Ar' })} />
        )}

          {activeTab === 'agenda' && (
          <AgendaTab
            maintenances={maintenances}
            clients={clients}
            appointments={appointments}
            onUpdateStatus={handleUpdateServiceStatus}
            onReschedule={handleReschedule}
            onFinalizar={handleFinalizarServico}
            calendarConnected={googleCalendar.connected}
            calendarSyncing={googleCalendarBusy}
            calendarLastSync={googleCalendar.lastSync}
            onGoogleCalendarSync={handleGoogleCalendarSync}
            onEditServico={(item) => setEditServico(item)}
            onExcluirServico={(id) => handleDeleteMaintenance(id)}
          />
        )}

        {activeTab === 'financeiro' && (
          <FinanceiroTab
            services={servicesList}
            budgets={budgets}
            clients={clients}
            profile={profile}
            onMarcarRecebido={handleMarcarRecebido}
            onAbrirOrcamento={(b) => setSelectedBudget(b)}
            onUpdateStatus={handleUpdateServiceStatus}
            onFinalizar={handleFinalizarServico}
            onAbrirOS={(serviceId) => {
              const svc = servicesList.find((s: any) => s.id === serviceId);
              const cl = clients.find((c) => c.id === svc?.cliente_id);
              const ap = cl?.appliances?.find((a) => a.id === svc?.aparelho_id) || cl?.appliances?.[0];
              const rec = maintenances.find((m) => m.id === serviceId);
              if (cl && ap && rec) setModalOS({ client: cl, appliance: ap, record: rec });
            }}
          />
        )}

        {activeTab === 'orcamentos' && (
          <OrcamentosTab
            budgets={budgets}
            clients={clients}
            profile={profile}
            onOpenNovoOrcamento={() => setShowNovoOrcamento(true)}
            onSelectBudget={(b) => setSelectedBudget(b)}
            onUpdateStatusOrc={handleUpdateBudgetStatus}
            onMarcarRecebido={handleMarcarRecebido}
          />
        )}

        {activeTab === 'servicos' && (
          <ServicosTab
            onIniciarServico={(type) => handleOpenIniciarServico(type)}
            maintenances={maintenances}
            services={servicesList}
            clients={clients}
            catalogo={catalogoTipos}
            onAddTipo={handleAddTipo}
            onEditTipo={handleEditTipo}
            onDeleteTipo={handleDeleteTipo}
            onEditServico={(item) => setEditServico(item)}
            onDeleteServico={(id) => handleDeleteMaintenance(id)}
            onNovoServicoDireto={() => setShowNovoServico(true)}
            onUpdateStatus={handleUpdateServiceStatus}
            onFinalizar={handleFinalizarServico}
          />
        )}

        {activeTab === 'clientes' && (
          <ClientesTab
            clients={clients}
            budgets={budgets}
            onOpenNovoCliente={() => setShowNovoCliente(true)}
            onOpenFicha={(c, a) => setModalFicha({ client: c, appliance: a })}
            onOpenChecklist={(c, a) => setModalChecklist({ client: c, appliance: a, serviceType: 'Limpeza de Ar' })}
            onAddApplianceToClient={(c) => setShowNovoAparelho(c)}
            onDeleteClient={handleDeleteClient}
            onDeleteAppliance={handleDeleteAppliance}
            onEditClient={(cl) => setEditCliente(cl)}
          />
        )}

      </div>
    );
  };

  // Faturamento consolidado: serviços concluídos + orçamentos aprovados que AINDA
  // não viraram OS concluída (vínculo service_id evita contar o mesmo dinheiro 2x)
  const faturamentoTotal =
    servicesList
      .filter((s: any) => s.status === 'CONCLUIDO')
      .reduce((acc, s: any) => acc + (Number(s.valor) || 0), 0) +
    budgets
      .filter((b) => {
        if (b.status !== 'aprovado') return false;
        const svc = b.serviceId ? servicesList.find((x: any) => x.id === b.serviceId) : null;
        return !svc || svc.status !== 'CONCLUIDO';
      })
      .reduce((acc, b) => acc + (b.finalValue || 0), 0);

  return (
    <div className="inovar-page min-h-screen text-slate-100 flex flex-col font-sans antialiased selection:bg-inovar-yellow selection:text-inovar-navy">
      <PwaStatus />
      {googleSyncError && userProfile && <div role="status" className="bg-amber-50 text-amber-950 p-3 text-sm flex flex-wrap gap-2 justify-center"><span>Agenda Google: {googleSyncError}</span><button onClick={() => setShowAccount(true)} className="font-bold underline">Ver conexões</button></div>}
      {showAccount && userProfile && <ContaModal onClose={() => setShowAccount(false)} equipe={userProfile.tipo !== 'CLIENTE'} />}
      {(passwordRecovery || passwordChangeRequired) && <RecuperarSenhaModal obrigatoria={passwordChangeRequired} onClose={() => { setPasswordRecovery(false); setPasswordChangeRequired(false); window.history.replaceState({}, '', window.location.pathname); }} />}
      {/* Toast global */}
      {toast && (
        <div
          className={`fixed top-14 left-1/2 -translate-x-1/2 z-[60] px-4 py-2.5 rounded-xl text-xs font-semibold shadow-2xl border max-w-[92vw] flex items-center gap-2 ${
            toast.type === 'success'
              ? 'bg-emerald-950/95 border-emerald-600 text-emerald-200'
              : toast.type === 'error'
                ? 'bg-red-950/95 border-red-600 text-red-200'
                : 'bg-slate-900/95 border-slate-600 text-slate-200'
          }`}
        >
          {toast.type === 'success' ? <CheckCircle2 className="w-4 h-4 text-emerald-400 shrink-0" /> : <AlertCircle className="w-4 h-4 text-red-400 shrink-0" />}
          <span>{toast.text}</span>
          <button onClick={() => setToast(null)} className="ml-1 text-slate-400 hover:text-white font-bold">✕</button>
        </div>
      )}

      {/* Header */}
      <Header
        activeTab={activeTab}
        setActiveTab={setActiveTab}
        onOpenNovoCliente={() => setShowNovoCliente(true)}
        onOpenNovoAgendamento={() => setAgendamentoTarget({})}
        onOpenNovoOrcamento={() => setShowNovoOrcamento(true)}
        onOpenAccount={() => setShowAccount(true)}
        onOpenConfig={() => setShowConfiguracoes(true)}
        onSignOut={handleSignOut}
        userProfile={userProfile}
        stats={stats}
      />

      {/* Main Container */}
      <main className="flex-1 w-full mx-auto max-w-screen-2xl 2xl:max-w-[1600px] px-3 py-3 sm:px-4 sm:py-4 lg:px-6 lg:py-6 pb-28 sm:pb-8 text-slate-800">
        <div className="w-full mx-auto">{renderContent()}</div>
      </main>

      {/* BARRA DE NAVEGAÇÃO INFERIOR NATIVA (Mobile Bottom Bar) */}
      {userProfile && userProfile.tipo !== 'CLIENTE' && (
        <div className="sm:hidden fixed bottom-0 left-0 right-0 z-40 bg-slate-950/95 backdrop-blur-md border-t border-slate-800 px-1 py-2 flex items-center justify-around text-[11px] font-semibold text-slate-400 shadow-2xl">
          {/* 1. Início */}
          <button
            onClick={() => setActiveTab('inicio')}
            className={`flex flex-col items-center gap-0.5 py-1 px-1.5 rounded-lg transition-colors ${
              activeTab === 'inicio' ? 'text-inovar-yellow font-bold' : 'hover:text-white'
            }`}
          >
            <Clock className="w-5 h-5" />
            <span>Início</span>
          </button>

          {/* 2. Agenda */}
          <button
            onClick={() => setActiveTab('agenda')}
            className={`flex flex-col items-center gap-0.5 py-1 px-1.5 rounded-lg transition-colors ${
              activeTab === 'agenda' ? 'text-inovar-yellow font-bold' : 'hover:text-white'
            }`}
          >
            <Calendar className="w-5 h-5" />
            <span>Agenda</span>
          </button>

          {/* 3. Orçamentos */}
          <button
            onClick={() => setActiveTab('orcamentos')}
            className={`flex flex-col items-center gap-0.5 py-1 px-1.5 rounded-lg transition-colors ${
              activeTab === 'orcamentos' ? 'text-inovar-yellow font-bold' : 'hover:text-white'
            }`}
          >
            <Calculator className="w-5 h-5" />
            <span>Orçamen.</span>
          </button>

          {/* 4. Menu principal: disponível sobre qualquer tela do PWA */}
          <button
            onClick={() => setShowMobileActionSheet(true)}
            className="flex flex-col items-center justify-center -mt-7 bg-inovar-yellow text-inovar-navy w-14 h-14 rounded-full shadow-lg shadow-yellow-500/20 border-2 border-slate-950 active:scale-90 transition-transform font-bold"
            title="Menu principal e ações rápidas"
            aria-label="Abrir menu principal e ações rápidas"
          >
            <Menu className="w-7 h-7 stroke-[3]" />
          </button>

          {/* 5. Clientes */}
          <button
            onClick={() => setActiveTab('clientes')}
            className={`flex flex-col items-center gap-0.5 py-1 px-1.5 rounded-lg transition-colors ${
              activeTab === 'clientes' ? 'text-inovar-yellow font-bold' : 'hover:text-white'
            }`}
          >
            <Users className="w-5 h-5" />
            <span>Clientes</span>
          </button>

          {/* 6. Financeiro */}
          <button
            onClick={() => setActiveTab('financeiro')}
            className={`flex flex-col items-center gap-0.5 py-1 px-1.5 rounded-lg transition-colors ${
              activeTab === 'financeiro' ? 'text-inovar-yellow font-bold' : 'hover:text-white'
            }`}
          >
            <Wallet className="w-5 h-5" />
            <span>Financ.</span>
          </button>
        </div>
      )}

      {/* MODALS */}
      {showAuthModal && (
        <AuthModal
          isOpen={showAuthModal}
          initialMode={showAuthModalSignup ? 'signup' : 'login'}
          onClose={() => setShowAuthModal(false)}
          onSuccess={() => {
            syncFromSupabase();
          }}
        />
      )}

      {modalFicha && (
        <FichaAparelhoModal
          client={modalFicha.client}
          appliance={modalFicha.appliance}
          maintenances={maintenances}
          onClose={() => setModalFicha(null)}
          onSaveAppliance={handleSaveAppliance}
          onOpenChecklist={(c, a) => setModalChecklist({ client: c, appliance: a, serviceType: 'Limpeza de Ar' })}
          onRegistrarHistorico={(c, a) => { setModalFicha(null); setHistoricoModal({ client: c, appliance: a }); }}
        />
      )}

      {modalChecklist && (
        <ChecklistLimpezaModal
          client={modalChecklist.client}
          appliance={modalChecklist.appliance}
          profile={profile}
          initialServiceType={modalChecklist.serviceType || 'Limpeza de Ar'}
          catalogo={catalogoTipos}
          onClose={() => setModalChecklist(null)}
          onSaveMaintenance={handleSaveMaintenance}
        />
      )}

      {modalOS && (
        <OrdemServicoModal
          client={modalOS.client}
          appliance={modalOS.appliance}
          maintenance={modalOS.record}
          profile={profile}
          onClose={() => setModalOS(null)}
          onEditOS={handleEditOS}
          onDeleteMaintenance={handleDeleteMaintenance}
        />
      )}

      {showNovoCliente && (
        <NovoClienteModal
          onClose={() => setShowNovoCliente(false)}
          onSave={handleSaveClient}
        />
      )}

      {agendamentoTarget !== null && (
        <NovoAgendamentoModal
          clients={clients}
          profile={profile}
          catalogo={catalogoTipos}
          initialClient={agendamentoTarget.client}
          initialAppliance={agendamentoTarget.appliance}
          onClose={() => setAgendamentoTarget(null)}
          onSave={handleSaveAgendamento}
        />
      )}

      {showNovoOrcamento && (
        <NovoOrcamentoModal
          clients={clients}
          profile={profile}
          catalogo={catalogoTipos}
          initialClient={orcamentoDeChamado || undefined}
          editing={editBudget || undefined}
          onClose={() => { setShowNovoOrcamento(false); setOrcamentoDeChamado(null); setEditBudget(null); }}
          onSave={handleSaveBudget}
        />
      )}

      {showNovoAparelho && (
        <NovoAparelhoModal
          client={showNovoAparelho}
          onClose={() => setShowNovoAparelho(null)}
          onSave={handleAddApplianceToClient}
        />
      )}

      {showConfiguracoes && (
        <ConfiguracoesModal
          profile={profile}
          ehAdmin={userProfile?.tipo === 'ADMIN'}
          onOpenMensagens={() => { setShowConfiguracoes(false); setShowMensagens(true); }}
          onClose={() => setShowConfiguracoes(false)}
          onSaveProfile={handleSaveProfile}
          onDataReload={loadData}
        />
      )}

      {/* Central de Mensagens do WhatsApp (só admin) */}
      {showMensagens && (
        <MensagensWhatsModal
          profile={profile}
          onClose={() => setShowMensagens(false)}
          onSalvar={handleSalvarMensagens}
          onSalvarAjustes={(campos) => persistirPerfil({ ...profile, ...campos })}
        />
      )}

      {/* Iniciar Serviço Modal */}
      {showIniciarServico.isOpen && (
        <IniciarServicoModal
          clients={clients}
          initialServiceType={showIniciarServico.initialType}
          catalogo={catalogoTipos}
          onClose={() => setShowIniciarServico({ isOpen: false })}
          onSelect={handleServiceSelected}
        />
      )}

      {/* EDITAR CLIENTE */}
      {editCliente && (
        <EditarClienteModal
          client={editCliente}
          onClose={() => setEditCliente(null)}
          onSave={handleEditClient}
        />
      )}

      {/* AGENDAR CHAMADO OU ORCAMENTO APROVADO */}
      {agendarChamado && (
        <AgendarChamadoModal
          chamado={agendarChamado}
          onClose={() => setAgendarChamado(null)}
          onSave={handleAgendarChamado}
        />
      )}
      {showNovoServico && (
        <NovoServicoModal
          clients={clients}
          catalogo={catalogoTipos}
          onClose={() => setShowNovoServico(false)}
          onSave={(dados) => {
            handleNovoServico(dados);
            setShowNovoServico(false);
          }}
        />
      )}

      {/* REGISTRO RETROATIVO DE HISTÓRICO (ficha, fila ou aba OS) */}
      {historicoModal && (
        <NovoHistoricoModal
          clients={clients}
          catalogo={catalogoTipos}
          profile={profile}
          target={historicoModal.client && historicoModal.appliance ? historicoModal as { client: Client; appliance: Appliance } : undefined}
          onClose={() => setHistoricoModal(null)}
          onSalvar={handleRegistrarHistorico}
        />
      )}

      {agendarOrcamento && (
        <AgendarChamadoModal
          chamado={{ id: agendarOrcamento.id, cliente: agendarOrcamento.clientName, servico: agendarOrcamento.items?.[0]?.description || 'Serviço aprovado', aparelho: agendarOrcamento.applianceDesc || '' }}
          onClose={() => setAgendarOrcamento(null)}
          onSave={(id, data, hora) => handleAgendarOrcamento(agendarOrcamento, data, hora)}
        />
      )}

      {/* EDITAR SERVICO */}
      {editServico && (
        <EditarServicoModal
          catalogo={catalogoTipos}
          servico={editServico}
          onClose={() => setEditServico(null)}
          onSave={handleEditService}
        />
      )}

      {/* Mobile Action Sheet Modal */}
        {showMobileActionSheet && (
          <MobileActionSheet
            isOpen={showMobileActionSheet}
            onClose={() => setShowMobileActionSheet(false)}
            catalogo={catalogoTipos}
            onOpenIniciarServico={(type) => handleOpenIniciarServico(type)}
            onOpenNovoCliente={() => setShowNovoCliente(true)}
            onOpenNovoAgendamento={() => setAgendamentoTarget({})}
            onOpenNovoOrcamento={() => setShowNovoOrcamento(true)}
            onOpenInicio={() => setActiveTab('inicio')}
            onOpenServicos={() => setActiveTab('servicos')}
            onOpenAgenda={() => setActiveTab('agenda')}
            onOpenOrcamentos={() => setActiveTab('orcamentos')}
            onOpenClientes={() => setActiveTab('clientes')}
            onOpenFinanceiro={() => setActiveTab('financeiro')}
            activeTab={activeTab}
          />
        )}

      {/* Novo Orçamento Modal (com modo edição) — ver bloco em cima */}

      {/* Visualizar Orçamento Modal */}
      {selectedBudget && (
        <VisualizarOrcamentoModal
          budget={selectedBudget}
          profile={profile}
          clients={clients}
          onClose={() => setSelectedBudget(null)}
          onUpdateStatus={handleUpdateBudgetStatus}
          onDeleteBudget={handleDeleteBudget}
          onIniciarServicoAprovado={handleIniciarServicoAprovado}
          onAgendarOrcamento={(b) => { setSelectedBudget(null); setAgendarOrcamento(b); }}
          onMarcarRecebido={handleMarcarRecebido}
          onEditar={(b) => { setSelectedBudget(null); setEditBudget(b); setShowNovoOrcamento(true); }}
        />
      )}
    </div>
  );
}

export default App;
