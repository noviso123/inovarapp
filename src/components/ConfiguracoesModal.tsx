import React, { useState, useEffect } from 'react';
import { TechnicianProfile } from '../types';
import { StorageService } from '../services/storage';
import { supabase } from '../services/supabase';
import {
  X,
  Settings,
  Save,
  Download,
  Upload,
  Check,
  Loader2,
  PenTool,
  Smartphone,
  Wifi,
  WifiOff
} from 'lucide-react';

interface ConfiguracoesModalProps {
  profile: TechnicianProfile;
  ehAdmin?: boolean;
  onOpenMensagens?: () => void;
  onClose: () => void;
  onSaveProfile: (updated: TechnicianProfile) => void;
  onDataReload: () => void;
}

export const ConfiguracoesModal: React.FC<ConfiguracoesModalProps> = ({
  profile,
  ehAdmin,
  onOpenMensagens,
  onClose,
  onSaveProfile,
  onDataReload
}) => {
  const [formData, setFormData] = useState<TechnicianProfile>({ ...profile });
  const [successMsg, setSuccessMsg] = useState('');
  const [salvando, setSalvando] = useState(false);
  const [waStatus, setWaStatus] = useState<any>(null);
  const [waQr, setWaQr] = useState<any>(null);
  const [waPhone, setWaPhone] = useState('');
  const [waPairingActive, setWaPairingActive] = useState(false);
  const [metaSetupOpen, setMetaSetupOpen] = useState(false);
  const [verificando, setVerificando] = useState(false);
  const [waConectadoSalvo, setWaConectadoSalvo] = useState(false);
  const [mostrarAvancado, setMostrarAvancado] = useState(false);

  // Durante o pareamento, acompanha o estado e busca novamente o QR se ele
  // ainda não estava pronto quando a solicitação inicial foi atendida.
  useEffect(() => {
    if (!waPairingActive) return;
    let checks = 0;
    let pending = false;
    const intervalo = setInterval(async () => {
      if (pending) return;
      checks++;
      pending = true;
      try {
        const r = await fetch('/api/whatsapp', { method: 'POST', headers: { ...(await authHeader()), 'Content-Type': 'application/json' }, body: JSON.stringify({ acao: 'status' }) });
        const j = await r.json();
        if (j.conectado) {
          setWaQr(null);
          setWaStatus(j);
          setWaConectadoSalvo(true);
          setWaPairingActive(false);
          NotificarConexao();
        } else {
          setWaStatus(j.pairingCode ? { ...j, mensagem: 'No celular, abra Aparelhos conectados → Conectar aparelho → Conectar com número de telefone e digite o código acima.' } : j);
          setWaConectadoSalvo(false);
          if (j.qr || j.pairingCode) setWaQr((current: any) => {
            if (current?.qr === j.qr && current?.pairingCode === j.pairingCode) return current;
            return { ...current, ...j };
          });
          if (['logged_out', 'temporarily_banned', 'client_outdated', 'stream_replaced', 'connection_failed', 'pairing_failed', 'passkey_required', 'passkey_processing', 'passkey_confirmation', 'credenciais_invalidas', 'nao_encontrada'].includes(j.estado)) {
            setWaQr(null);
            setWaPairingActive(false);
          }
        }
      } catch { /* mantém aguardando */ }
      finally { pending = false; }
      if (checks >= 150) {
        clearInterval(intervalo);
        setWaPairingActive(false);
        setWaStatus((current: any) => ({ ...current, mensagem: 'O pareamento não foi confirmado após 10 minutos. Atualize o estado e, se necessário, gere um novo QR ou código.' }));
      }
    }, 4000);
    return () => clearInterval(intervalo);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [waPairingActive]);

  const NotificarConexao = () => {
    try {
      if (typeof Notification !== 'undefined' && Notification.permission === 'granted') {
        new Notification('InovarApp', { body: 'WhatsApp conectado e salvo! Disparos automáticos ativos.' });
      }
    } catch { /* ignore */ }
  };

  const gerenciarConexao = async (modo: 'logout' | 'apagar') => {
    setVerificando(true);
    try {
      const { data: sessionData } = await supabase.auth.getSession();
      const response = await fetch('/api/whatsapp', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${sessionData?.session?.access_token || ''}` },
        body: JSON.stringify({ acao: 'desconectar', modo })
      });
      const result = await response.json().catch(() => ({}));
      if (!response.ok) {
        setWaStatus({ conectado: true, mensagem: result.error || 'Não foi possível alterar a conexão WhatsApp.' });
        return;
      }
      setWaConectadoSalvo(false);
      setWaQr(null);
      setWaPairingActive(false);
      const r = await fetch('/api/whatsapp', { method: 'POST', headers: { ...(await authHeader()), 'Content-Type': 'application/json' }, body: JSON.stringify({ acao: 'status' }) });
      setWaStatus(await r.json());
    } catch {
      setWaStatus({ conectado: false, mensagem: 'Falha ao desconectar.' });
    } finally {
      setVerificando(false);
    }
  };

  const authHeader = async () => {
    const { data: sessionData } = await supabase.auth.getSession();
    return { Authorization: `Bearer ${sessionData?.session?.access_token || ''}` };
  };

  const verificarWhatsapp = async () => {
    setVerificando(true);
    try {
      const r = await fetch('/api/whatsapp', { method: 'POST', headers: { ...(await authHeader()), 'Content-Type': 'application/json' }, body: JSON.stringify({ acao: 'status' }) });
      const j = await r.json();
      setWaStatus(j);
      setWaConectadoSalvo(!!j.conectado);
      if (j.conectado) {
        setWaConectadoSalvo(true);
        setWaQr(null);
        setWaPairingActive(false);
      } else if (j.qr || j.pairingCode) {
        setWaQr((current: any) => {
          if (current?.qr === j.qr && current?.pairingCode === j.pairingCode) return current;
          return { ...current, ...j };
        });
        setWaPairingActive(true);
      } else if (j.estado === 'connecting') {
        setWaPairingActive(true);
      } else if (['logged_out', 'temporarily_banned', 'client_outdated', 'stream_replaced', 'connection_failed', 'pairing_failed', 'passkey_required', 'passkey_processing', 'passkey_confirmation', 'credenciais_invalidas', 'nao_encontrada'].includes(j.estado)) {
        setWaQr(null);
        setWaPairingActive(false);
      }
    } catch {
      setWaStatus({ conectado: false, mensagem: 'Falha ao verificar conexão.' });
    } finally {
      setVerificando(false);
    }
  };

  useEffect(() => {
    void verificarWhatsapp();
    // Verifica a sessão persistida sempre que as configurações são abertas.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const conectarWhatsapp = async (phone = '') => {
    setVerificando(true);
    setWaStatus(null);
    setWaQr(null);
    setWaPairingActive(false);
    try {
      const r = await fetch('/api/whatsapp', { method: 'POST', headers: { ...(await authHeader()), 'Content-Type': 'application/json' }, body: JSON.stringify({ acao: 'conectar', ...(phone ? { telefone: phone } : {}) }) });
      const j = await r.json();
      if (j.conectado) {
        setWaQr(null);
        setWaPairingActive(false);
        setWaStatus(j);
        setWaConectadoSalvo(true);
      } else if (r.ok) {
        setWaQr(j);
        setWaPairingActive(true);
        setWaStatus({ conectado: false, mensagem: j.pairingCode ? 'No WhatsApp, abra Aparelhos conectados → Conectar aparelho → Conectar com número de telefone e informe o código.' : 'Escaneie o QR com o WhatsApp. A conexão será detectada automaticamente.' });
      } else {
        setWaQr(null);
        setWaPairingActive(false);
        setWaStatus({ ...j, conectado: false });
      }
    } catch {
      setWaStatus({ conectado: false, mensagem: 'Falha ao gerar o QR de conexão.' });
    } finally {
      setVerificando(false);
    }
  };

  // Carrega as configurações do BANCO (sincronizadas entre site, app e celulares)
  useEffect(() => {
    (async () => {
      try {
        const { data: sessionData } = await supabase.auth.getSession();
        const r = await fetch('/api/configuracoes', {
          headers: { Authorization: `Bearer ${sessionData?.session?.access_token || ''}` }
        });
        const j = await r.json();
        if (j.ok && j.config) {
          setFormData((prev) => ({ ...prev, ...j.config }));
        }
      } catch { /* mantém perfil local como fallback */ }
    })();
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSalvando(true);
    try {
      const { data: sessionData } = await supabase.auth.getSession();
      const r = await fetch('/api/configuracoes', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${sessionData?.session?.access_token || ''}`
        },
        body: JSON.stringify(formData)
      });
      const j = await r.json().catch(() => ({}));
      if (r.ok && j.ok) {
        onSaveProfile(j.config);
        setSuccessMsg('Salvo no banco! Sincronizado em todos os dispositivos.');
      } else {
        onSaveProfile(formData);
        setSuccessMsg(j.error ? `Aviso: ${j.error}` : 'Salvo localmente (sem conexão com o banco).');
      }
    } catch {
      onSaveProfile(formData);
      setSuccessMsg('Salvo localmente (sem conexão com o banco).');
    } finally {
      setSalvando(false);
      setTimeout(() => setSuccessMsg(''), 3000);
    }
  };

  const handleExportBackup = () => {
    const json = StorageService.exportBackup();
    const blob = new Blob([json], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `backup_arcondicapp_${new Date().toISOString().split('T')[0]}.json`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const handleImportBackup = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    const reader = new FileReader();
    reader.onload = (event) => {
      const content = event.target?.result as string;
      const ok = StorageService.importBackup(content);
      if (ok) {
        onDataReload();
        setSuccessMsg('Backup restaurado com sucesso!');
        setTimeout(() => setSuccessMsg(''), 2500);
      } else {
        alert('Arquivo de backup inválido.');
      }
    };
    reader.readAsText(file);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-2 sm:p-4 bg-slate-950/70 backdrop-blur-sm">
      <div role="dialog" aria-modal="true" aria-labelledby="configuracoes-titulo" className="bg-white text-slate-900 rounded-2xl w-full max-w-2xl max-h-[calc(100dvh-1rem)] sm:max-h-[92dvh] flex flex-col shadow-2xl border border-slate-200 overflow-hidden">
        <div className="bg-sky-50 border-b border-sky-100 p-4 text-slate-900 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <div className="p-2 bg-sky-100 text-sky-700 rounded-xl border border-sky-200">
              <Settings className="w-5 h-5" />
            </div>
            <div>
              <h3 id="configuracoes-titulo" className="font-bold text-sm text-slate-900">Configurações & Perfil do Técnico</h3>
              <p className="text-[11px] text-slate-600">Dados profissionais, agenda e integrações</p>
            </div>
          </div>
          <button onClick={onClose} className="p-2 text-slate-500 hover:text-slate-900 rounded-lg hover:bg-white">
            <X className="w-5 h-5" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="flex-1 min-h-0 overflow-y-auto overscroll-contain p-4 sm:p-5 space-y-4 text-xs">
          {successMsg && (
            <div className="p-3 bg-emerald-50 text-emerald-800 border border-emerald-200 rounded-xl font-semibold flex items-center gap-2">
              <Check className="w-4 h-4 text-emerald-600" />
              {successMsg}
            </div>
          )}

          {/* Dados Profissionais */}
          <div>
            <h4 className="font-bold text-slate-800 uppercase tracking-wider mb-2 text-[11px]">
              Identificação Profissional (Sai nas Ordens de Serviço e Recibos)
            </h4>
            <div className="space-y-2.5">
              <div>
                <label className="block font-semibold text-slate-700 mb-1">Seu Nome Profissional</label>
                <input
                  type="text"
                  required
                  value={formData.name}
                  onChange={e => setFormData({ ...formData, name: e.target.value })}
                  className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
                />
              </div>

              <div>
                <label className="block font-semibold text-slate-700 mb-1">Nome da Empresa / Fantasia</label>
                <input
                  type="text"
                  value={formData.businessName}
                  onChange={e => setFormData({ ...formData, businessName: e.target.value })}
                  className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
                />
              </div>

              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className="block font-semibold text-slate-700 mb-1">CNPJ da empresa</label>
                  <input type="text" value={formData.cnpj || ''} onChange={e => setFormData({ ...formData, cnpj: e.target.value })} placeholder="00.000.000/0000-00" className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs" />
                </div>
                <div>
                  <label className="block font-semibold text-slate-700 mb-1">Endereço da empresa</label>
                  <input type="text" value={formData.address || ''} onChange={e => setFormData({ ...formData, address: e.target.value })} placeholder="Cidade / endereço" className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs" />
                </div>
              </div>

              <div>
                <label className="block font-semibold text-slate-700 mb-1">WhatsApp de Contato</label>
                <input
                  type="tel"
                  required
                  value={formData.phone}
                  onChange={e => setFormData({ ...formData, phone: e.target.value })}
                  className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
                />
              </div>
            </div>
          </div>

          <hr className="border-slate-200" />

          {/* Dados Financeiros e PIX */}
          <div>
            <h4 className="font-bold text-slate-800 uppercase tracking-wider mb-2 text-[11px]">
              Dados de Cobrança & PIX
            </h4>
            <div className="grid grid-cols-3 gap-2">
              <div className="col-span-2">
                <label className="block font-semibold text-slate-700 mb-1">Chave PIX</label>
                <input
                  type="text"
                  value={formData.pixKey}
                  onChange={e => setFormData({ ...formData, pixKey: e.target.value })}
                  placeholder="Seu CPF, CNPJ, e-mail ou celular"
                  className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs"
                />
              </div>

              <div>
                <label className="block font-semibold text-slate-700 mb-1">Tipo de Chave</label>
                <select
                  value={formData.pixType}
                  onChange={e => setFormData({ ...formData, pixType: e.target.value as any })}
                  className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs font-medium"
                >
                  <option value="cpf">CPF</option>
                  <option value="cnpj">CNPJ</option>
                  <option value="email">E-mail</option>
                  <option value="telefone">Telefone</option>
                  <option value="aleatoria">Aleatória</option>
                </select>
              </div>
            </div>
          </div>

          <div className="grid grid-cols-3 gap-2">
            <div>
              <label className="block font-semibold text-slate-700 mb-1">Preço Médio Limpeza (R$)</label>
              <input
                type="number"
                value={formData.defaultPrice}
                onChange={e => setFormData({ ...formData, defaultPrice: Number(e.target.value) })}
                className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs font-bold text-emerald-700"
              />
            </div>

            <div>
              <label className="block font-semibold text-slate-700 mb-1">Ciclo Padrão (Meses)</label>
              <select
                value={formData.defaultReturnMonths}
                onChange={e => setFormData({ ...formData, defaultReturnMonths: Number(e.target.value) })}
                className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs font-medium"
              >
                <option value={3}>3 meses</option>
                <option value={6}>6 meses (Recomendado)</option>
                <option value={12}>12 meses</option>
              </select>
            </div>

            <div>
              <label className="block font-semibold text-slate-700 mb-1">Garantia padrão (dias)</label>
              <input type="number" min="0" value={formData.defaultWarrantyDays || 0} onChange={e => setFormData({ ...formData, defaultWarrantyDays: Math.max(0, Number(e.target.value)) })} className="w-full p-2 bg-slate-50 border border-slate-300 rounded-lg text-xs font-bold text-emerald-700" />
            </div>
          </div>

          <hr className="border-slate-200" />

          <hr className="border-slate-200" />

          {/* Assinatura do Técnico */}
          <div>
            <h4 className="font-bold text-slate-800 uppercase tracking-wider mb-2 text-[11px] flex items-center gap-1.5">
              <PenTool className="w-3.5 h-3.5 text-indigo-600" />
              Assinatura profissional do técnico/admin
            </h4>
            <div className="bg-slate-50 border border-slate-200 rounded-xl p-3">
              <div className="flex items-center gap-3 rounded-lg border border-slate-200 bg-white p-2.5">
                {formData.assinatura && <img src={formData.assinatura} alt="Assinatura cadastrada" className="h-12 w-44 object-contain object-left" />}
                <div className="min-w-0 flex-1">
                  <p className="font-bold text-slate-800">Aplicada automaticamente</p>
                  <p className="mt-0.5 text-[10px] leading-relaxed text-slate-500">A assinatura cadastrada entra automaticamente nas Ordens de Serviço e orçamentos. O cliente continua assinando apenas quando necessário.</p>
                </div>
              </div>
            </div>
          </div>

          <hr className="border-slate-200" />

          {/* CENTRAL DE CONEXÃO WHATSAPP */}
          <div className="rounded-2xl border border-slate-200 overflow-hidden bg-white">
            {/* Conexão direta: detalhes de servidor só aparecem se o administrador precisar deles. */}
            <div className={waStatus?.conectado ? 'p-4 flex items-center justify-between gap-3 bg-emerald-50 border-b border-emerald-200' : 'p-4 flex items-center justify-between gap-3 bg-sky-50 border-b border-sky-200'}>
              <div className="flex items-center gap-3 min-w-0">
                <div className={waStatus?.conectado ? 'w-11 h-11 rounded-2xl flex items-center justify-center shrink-0 bg-emerald-600 text-white' : 'w-11 h-11 rounded-2xl flex items-center justify-center shrink-0 bg-sky-600 text-white'}>
                  {verificando && !waStatus ? <Loader2 className="w-5 h-5 animate-spin" /> : waStatus?.conectado ? <Wifi className="w-5 h-5" /> : <WifiOff className="w-5 h-5" />}
                </div>
                <div className="min-w-0">
                  <span className="block text-slate-900 font-extrabold text-sm">
                    {verificando && !waStatus ? 'Verificando conexão...' : waStatus?.conectado ? 'WhatsApp conectado' : 'Conectar WhatsApp'}
                  </span>
                  <span className="block text-[11px] text-slate-600 truncate">
                    {waStatus?.conectado ? 'Disparos automáticos ativos • ' + (waStatus.instancia || 'inovar') : waStatus?.mensagem || 'Conecte para enviar orçamentos, OS e alertas automaticamente'}
                  </span>
                </div>
              </div>
              <button
                type="button"
                onClick={verificarWhatsapp}
                disabled={verificando}
                className="px-3 py-2 bg-white hover:bg-slate-50 border border-slate-300 text-slate-700 rounded-xl text-[11px] font-bold shrink-0 disabled:opacity-50"
              >
                Atualizar
              </button>
            </div>

            {/* Corpo da conexão */}
            <div className="p-4 bg-white space-y-3">
              {waConectadoSalvo && (
                <div className="p-3 bg-emerald-50 border border-emerald-200 rounded-xl text-emerald-800 text-xs font-bold flex items-center gap-2">
                  <Check className="w-4 h-4" /> Conta conectada e salva! Você pode desconectar esta sessão ou gerar um novo QR para outra conta.
                </div>
              )}

              {!waStatus?.conectado && (
                <div className="space-y-2">
                  <button
                    type="button"
                    onClick={() => conectarWhatsapp()}
                    disabled={verificando}
                    className="w-full py-3.5 bg-emerald-600 hover:bg-emerald-500 active:scale-[0.99] text-white font-extrabold rounded-xl text-sm shadow-lg flex items-center justify-center gap-2 disabled:opacity-50 transition-all"
                  >
                    {verificando ? <Loader2 className="w-5 h-5 animate-spin" /> : <Smartphone className="w-5 h-5" />}
                    <span>Conectar pelo QR Code</span>
                  </button>
                  <label className="block text-xs font-semibold text-slate-700">
                    Ou conecte com número de telefone
                    <input type="tel" value={waPhone} onChange={(event) => setWaPhone(event.target.value)} placeholder="55 + DDD + telefone" className="mt-1 w-full rounded-xl border border-slate-300 px-3 py-3 text-sm" />
                  </label>
                  <button type="button" onClick={() => conectarWhatsapp(waPhone)} disabled={verificando || !waPhone.trim()} className="w-full py-3 bg-slate-100 hover:bg-slate-200 border border-slate-300 text-slate-800 font-bold rounded-xl text-sm disabled:opacity-50">
                    Gerar código de pareamento
                  </button>
                </div>
              )}

              {waQr && !waStatus?.conectado && (
                <div className="p-4 bg-white border-2 border-emerald-300 rounded-2xl flex flex-col items-center gap-3">
                  {waQr.qr && <img src={waQr.qr} alt="QR Code WhatsApp" className="w-64 h-64" />}
                  <ol className="text-[11px] text-slate-600 space-y-1 w-full">
                    <li><b>1.</b> Abra o WhatsApp no celular</li>
                    <li><b>2.</b> Toque em <b>Aparelhos conectados</b> → <b>Conectar aparelho</b></li>
                    {waQr.qr && <li><b>3.</b> Aponte a câmera para este QR</li>}
                    {waQr.pairingCode && <li><b>3.</b> Escolha conectar com número de telefone e informe o código abaixo</li>}
                  </ol>
                  {waQr.pairingCode && (
                    <div className="text-center">
                      <span className="text-[10px] text-slate-500 block">Código de pareamento:</span>
                      <span className="text-2xl font-black tracking-[0.25em] text-emerald-700">{waQr.pairingCode}</span>
                    </div>
                  )}
                  <div className="flex items-center gap-2 text-[11px] text-sky-600 font-bold">
                    <Loader2 className="w-3.5 h-3.5 animate-spin" /> Aguardando a leitura do QR... a conexão é detectada sozinha!
                  </div>
                </div>
              )}

              {waStatus?.mensagem && !waQr && (
                <div className={waStatus.conectado ? 'p-3 rounded-xl text-xs font-semibold bg-emerald-50 text-emerald-700 border border-emerald-200' : waStatus.configurado === false ? 'p-3 rounded-xl text-xs font-semibold bg-slate-100 text-slate-600 border border-slate-200' : 'p-3 rounded-xl text-xs font-semibold bg-amber-50 text-amber-700 border border-amber-200'}>
                  <span>{waStatus.mensagem}</span>
                </div>
              )}

              {waStatus?.conectado && (
                <div className="grid grid-cols-2 gap-2">
                  <button
                    type="button"
                    onClick={() => { if (confirm('Desconectar o WhatsApp? As credenciais ficam salvas para reconectar este mesmo número. Para usar outro número, escolha “Trocar número”.')) gerenciarConexao('logout'); }}
                    className="py-2.5 bg-white border border-slate-300 hover:border-red-300 text-slate-700 rounded-xl text-xs font-bold transition-colors"
                  >
                    Desconectar
                  </button>
                  <button
                    type="button"
                    onClick={() => { if (confirm('Trocar o número conectado? O app usa uma única instância. A conta atual será desconectada e removida; depois, você poderá parear outro número no mesmo lugar.')) gerenciarConexao('apagar'); }}
                    className="py-2.5 bg-white border border-slate-300 hover:border-amber-300 text-slate-700 rounded-xl text-xs font-bold transition-colors"
                  >
                    Trocar número
                  </button>
                </div>
              )}

            </div>
          </div>

          <div className="rounded-2xl border border-blue-200 bg-blue-50 p-4">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <h4 className="text-sm font-extrabold text-slate-900">Integração oficial da Meta</h4>
                  <span className="rounded-full border border-blue-200 bg-white px-2 py-0.5 text-[10px] font-bold text-blue-800">Opção futura</span>
                </div>
                <p className="mt-1 text-xs leading-relaxed text-slate-700">Uma alternativa para substituir o provedor Go quando a conta Business e todos os fluxos estiverem configurados.</p>
              </div>
              <button
                type="button"
                aria-expanded={metaSetupOpen}
                onClick={() => setMetaSetupOpen((open) => !open)}
                className="shrink-0 rounded-xl border border-blue-300 bg-white px-3 py-2.5 text-xs font-extrabold text-blue-900 hover:bg-blue-100"
              >
                {metaSetupOpen ? 'Fechar detalhes' : 'Preparar ativação Meta'}
              </button>
            </div>
            {metaSetupOpen && (
              <div className="mt-3 space-y-2 border-t border-blue-200 pt-3 text-xs leading-relaxed text-slate-700">
                <p className="font-bold text-slate-900">A Meta ainda não está ativa. O WhatsApp Go continua selecionado.</p>
                <p>Antes da troca, a integração precisa cobrir e validar:</p>
                <ul className="list-disc space-y-1 pl-5">
                  <li>Conexão da conta WhatsApp Business, número, WABA e credenciais guardadas no servidor.</li>
                  <li>Envio de texto, documentos/PDF e modelos aprovados pela Meta.</li>
                  <li>Webhooks autenticados para mensagens recebidas e estados de envio, entrega e leitura.</li>
                  <li>Alternância completa do provedor para orçamentos, ordens de serviço, alertas e mensagens manuais.</li>
                </ul>
                <p className="rounded-lg border border-blue-200 bg-white p-2">A conexão Meta usa o fluxo Business da Meta; ela não pareia por QR ou código de telefone. A troca só deve ser ativada após configurar o app Meta, as permissões e os webhooks no servidor e validar os fluxos.</p>
              </div>
            )}
          </div>

          <hr className="border-slate-200" />

          {/* Central de Mensagens do WhatsApp — somente ADMIN */}
          {ehAdmin && onOpenMensagens && (
            <>
              <button
                type="button"
                onClick={onOpenMensagens}
                className="w-full p-3.5 bg-white hover:bg-emerald-50 border border-emerald-200 rounded-xl flex items-center justify-between gap-3 text-left transition-colors"
              >
                <div className="flex items-center gap-2.5">
                  <div className="p-2 bg-emerald-100 text-emerald-700 rounded-lg border border-emerald-200 text-base">
                    💬
                  </div>
                  <div>
                    <span className="text-xs font-extrabold text-slate-900 block">Central de Mensagens do WhatsApp</span>
                    <span className="text-[10px] text-slate-500">
                      Editar mensagem por mensagem os envios automáticos — salvo no banco
                    </span>
                  </div>
                </div>
                <span className="text-emerald-400 text-xs font-bold">Abrir →</span>
              </button>
              <hr className="border-slate-200" />
            </>
          )}

          {/* Backup & Dados */}
          <div>
            <h4 className="font-bold text-slate-800 uppercase tracking-wider mb-2 text-[11px]">
              Cópia de Segurança (Offline)
            </h4>
            <div className="grid grid-cols-2 gap-2">
              <button
                type="button"
                onClick={handleExportBackup}
                className="p-2.5 bg-slate-100 hover:bg-slate-200 text-slate-800 rounded-xl font-semibold flex items-center justify-center gap-1.5 transition-colors"
              >
                <Download className="w-4 h-4 text-sky-600" />
                <span>Exportar Backup (JSON)</span>
              </button>

              <label className="p-2.5 bg-slate-100 hover:bg-slate-200 text-slate-800 rounded-xl font-semibold flex items-center justify-center gap-1.5 cursor-pointer transition-colors">
                <Upload className="w-4 h-4 text-emerald-600" />
                <span>Importar Backup</span>
                <input type="file" accept=".json" onChange={handleImportBackup} className="hidden" />
              </label>
            </div>
          </div>

          {/* Submit */}
          <div className="pt-2">
            <button
              type="submit"
              disabled={salvando}
              className="w-full py-2.5 bg-sky-600 hover:bg-sky-500 text-white rounded-xl font-bold flex items-center justify-center gap-1.5 shadow-md shadow-sky-900/20 disabled:opacity-50"
            >
              {salvando ? <Loader2 className="w-4 h-4 animate-spin" /> : <Save className="w-4 h-4" />}
              <span>Salvar no Banco & Sincronizar</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
