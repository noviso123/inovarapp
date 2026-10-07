import { Client, MaintenanceRecord, TechnicianProfile, ReturnStatus } from '../types';
import { differenceInCalendarDays, addMonths, format, parseISO, isSameWeek, isPast, isToday } from 'date-fns';
import { normalizarNomeServico } from './catalogo';

const CLIENTS_STORAGE_KEY = 'inovarapp_clients_v2';
const SEEDS_LIMPOS = 'inovarapp_seeds_limpos_v1';
const MAINTENANCES_STORAGE_KEY = 'inovarapp_maintenances_v2';
const PROFILE_STORAGE_KEY = 'inovarapp_profile_v2';

const DEFAULT_PROFILE: TechnicianProfile = {
  name: 'Técnico Responsável',
  businessName: 'Inovar Refrigeração',
  phone: '27999999999',
  pixKey: 'contato@inovar.com',
  pixType: 'email',
  defaultReturnMonths: 6,
  defaultWarrantyDays: 90,
  defaultPrice: 250,
  cnpj: '36.020.014/0001-14',
  address: 'Serra',
  // Assinatura profissional já cadastrada para o administrador Gabriel.
  // É usada automaticamente nos documentos e pode ser substituída nas Configurações.
  assinatura: '/inovar-brand/INOVAR_SIGNATURE_GABRIEL.png',
};



export const StorageService = {
  getClients(): Client[] {
    // limpa seeds antigos de demonstracao uma unica vez
    if (!localStorage.getItem(SEEDS_LIMPOS)) {
      try {
        const raw = localStorage.getItem(CLIENTS_STORAGE_KEY);
        if (raw) {
          const parsed = JSON.parse(raw);
          const filtrado = parsed.filter((cl: Client) => !['c20f1489-24ed-4881-8f1d-0cece2d7c4f8'].includes(cl.id));
          localStorage.setItem(CLIENTS_STORAGE_KEY, JSON.stringify(filtrado));
        }
        localStorage.setItem(SEEDS_LIMPOS, '1');
      } catch { /* ignore */ }
    }
    const raw = localStorage.getItem(CLIENTS_STORAGE_KEY);
    if (!raw) return [];
    try {
      const parsed = JSON.parse(raw);
      // normaliza: importações/backups antigos podem vir sem a lista de aparelhos
      return (Array.isArray(parsed) ? parsed : []).map((cl: any) => ({
        ...cl,
        appliances: Array.isArray(cl.appliances) ? cl.appliances : []
      }));
    } catch (e) {
      console.error('Erro ao ler clientes do localStorage', e);
      return [];
    }
  },

  saveClients(clients: Client[]): void {
    localStorage.setItem(CLIENTS_STORAGE_KEY, JSON.stringify(clients));
  },

  getMaintenances(): MaintenanceRecord[] {
    const raw = localStorage.getItem(MAINTENANCES_STORAGE_KEY);
    if (!raw) return [];
    try {
      const parsed = JSON.parse(raw);
      // normaliza: garante campos numéricos e datas para registros antigos/importados
      return (Array.isArray(parsed) ? parsed : []).map((m: any) => ({
        ...m,
        price: Number(m.price) || 0,
        warrantyDays: Number(m.warrantyDays) || 0,
        date: typeof m.date === 'string' ? m.date : '',
        returnDate: typeof m.returnDate === 'string' ? m.returnDate : '',
        serviceType: normalizarNomeServico(m.serviceType) as MaintenanceRecord['serviceType'],
        status: m.status || 'agendado'
      }));
    } catch (e) {
      console.error('Erro ao ler manutenções', e);
      return [];
    }
  },

  saveMaintenances(maintenances: MaintenanceRecord[]): void {
    const normalizados = maintenances.map((record) => ({
      ...record,
      serviceType: normalizarNomeServico(record.serviceType) as MaintenanceRecord['serviceType']
    }));
    localStorage.setItem(MAINTENANCES_STORAGE_KEY, JSON.stringify(normalizados));
  },

  getProfile(): TechnicianProfile {
    const raw = localStorage.getItem(PROFILE_STORAGE_KEY);
    if (!raw) {
      this.saveProfile(DEFAULT_PROFILE);
      return DEFAULT_PROFILE;
    }
    try {
      const parsed = JSON.parse(raw);
      if (parsed.name === 'Rodrigo Climatização') {
        this.saveProfile(DEFAULT_PROFILE);
        return DEFAULT_PROFILE;
      }
      // A assinatura oficial é comum à operação Gabriel/Admin/Técnico e não
      // depende de o dispositivo já ter sincronizado com o servidor.
      if ((/gabriel/i.test(parsed.name || '') || /inovar/i.test(parsed.businessName || '')) && parsed.assinatura !== DEFAULT_PROFILE.assinatura) {
        const atualizado = { ...parsed, assinatura: DEFAULT_PROFILE.assinatura };
        this.saveProfile(atualizado);
        return atualizado;
      }
      return parsed;
    } catch (e) {
      return DEFAULT_PROFILE;
    }
  },

  saveProfile(profile: TechnicianProfile): void {
    localStorage.setItem(PROFILE_STORAGE_KEY, JSON.stringify(profile));
  },

  exportBackup(): string {
    const data = {
      clients: this.getClients(),
      maintenances: this.getMaintenances(),
      profile: this.getProfile(),
      version: '1.0'
    };
    return JSON.stringify(data, null, 2);
  },

  importBackup(jsonString: string): boolean {
    try {
      const data = JSON.parse(jsonString);
      if (data.clients && Array.isArray(data.clients)) {
        this.saveClients(data.clients);
      }
      if (data.maintenances && Array.isArray(data.maintenances)) {
        this.saveMaintenances(data.maintenances);
      }
      if (data.profile) {
        this.saveProfile(data.profile);
      }
      return true;
    } catch (e) {
      return false;
    }
  },

  getReturnStatus(returnDate: string): ReturnStatus {
    try {
      const targetDate = parseISO(returnDate);
      const today = new Date();
      const daysDiff = differenceInCalendarDays(targetDate, today);

      if (daysDiff < 0) {
        return 'atrasado';
      }
      if (daysDiff <= 7) {
        return 'esta_semana';
      }
      if (daysDiff <= 30) {
        return 'em_breve';
      }
      return 'em_dia';
    } catch (e) {
      return 'em_dia';
    }
  },

  getStatusLabel(status: ReturnStatus): string {
    switch (status) {
      case 'atrasado':
        return 'Atrasado';
      case 'esta_semana':
        return 'Esta semana';
      case 'em_breve':
        return 'Em breve';
      case 'em_dia':
        return 'Em dia';
      case 'sem_historico':
        return 'Sem histórico';
    }
  },

  getStatusColor(status: ReturnStatus): {
    badgeBg: string;
    badgeText: string;
    border: string;
    accent: string;
    gradient: string;
  } {
    switch (status) {
      case 'atrasado':
        return {
          badgeBg: 'bg-red-500/20 text-red-400 border border-red-500/40',
          badgeText: 'text-red-400',
          border: 'border-l-red-500',
          accent: 'text-red-400',
          gradient: 'from-red-950/40 via-slate-900 to-slate-900'
        };
      case 'esta_semana':
        return {
          badgeBg: 'bg-amber-500/20 text-amber-300 border border-amber-500/40',
          badgeText: 'text-amber-300',
          border: 'border-l-amber-400',
          accent: 'text-amber-300',
          gradient: 'from-amber-950/40 via-slate-900 to-slate-900'
        };
      case 'em_breve':
        return {
          badgeBg: 'bg-sky-500/20 text-sky-300 border border-sky-500/40',
          badgeText: 'text-sky-300',
          border: 'border-l-sky-400',
          accent: 'text-sky-300',
          gradient: 'from-sky-950/40 via-slate-900 to-slate-900'
        };
      case 'em_dia':
        return {
          badgeBg: 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/40',
          badgeText: 'text-emerald-400',
          border: 'border-l-emerald-500',
          accent: 'text-emerald-400',
          gradient: 'from-emerald-950/40 via-slate-900 to-slate-900'
        };
      case 'sem_historico':
        return {
          badgeBg: 'bg-violet-500/20 text-violet-300 border border-violet-500/40',
          badgeText: 'text-violet-300',
          border: 'border-l-violet-400',
          accent: 'text-violet-300',
          gradient: 'from-violet-950/40 via-slate-900 to-slate-900'
        };
    }
  }
};
