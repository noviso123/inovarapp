# InovarApp — Escopo Completo do Projeto (v3)

**Inovar Refrigeração**
URL oficial: **https://inovarapp.vercel.app** (site + app, responsivo)
Atualização: 05/09/2026 — validação massiva 36/37 itens

## 1. Visão Geral
Sistema completo de gestão: painel técnico/admin (desktop e mobile) + portal do
cliente, 100% sincronizados com o banco Supabase (PostgreSQL + Auth + RLS).
- **Fluxo fechado:** cliente solicita → chamado cai para o técnico → orçamento
  com número oficial → cliente aprova **assinando digitalmente** → agendamento
  com data/hora → execução → OS em PDF com garantia → lembretes automáticos.
- **Login único:** técnicos (Gabriel, Jhonatan — ADMIN) e clientes no mesmo banco.
- **Comunicação automática:** WhatsApp pelo serviço próprio em Go (pareamento e envios), e-mail (Gmail
  SMTP) e notificações no app (sino + push do navegador).

## 2. Segurança (RLS em todas as tabelas)
- ADMIN/TECNICO: veem e editam tudo. CLIENTE: só o próprio cadastro/aparelhos/
  serviços/orçamentos. Anônimo: nada. Validado com bateria automatizada (36 itens).

## 3. Banco (Supabase ycpswioserctavijhnre)
profiles, customers, technicians, air_conditioners, services, appointments,
budgets, service_history — com triggers (novo usuário → perfil+cadastro),
número automático de orçamento, updated_at automático e funções auxiliares.
Documentos (OS/orçamentos em PDF) no Storage privado com links assinados.

## 4. Painel Técnico (todas com edição e exclusão sincronizadas)
- **Início:** KPIs (clientes, atrasados, vencendo, chamados, orçamentos, OS,
  faturamento consolidado), acesso rápido, fila "Quem preciso chamar?" e
  próximos retornos (dashboard em 2 colunas no desktop).
- **Agenda:** todos os atendimentos por data — **editar** (tipo, data, valor,
  observações), concluir, cancelar e **excluir**; reabrir OS concluída e reagendar.
- **Orçamentos:** criar (itens rápidos com preços padrão + tipos customizados),
  visualizar, aprovar/recusar/excluir, **PDF com assinatura digital do cliente**,
  envio automático e manual no WhatsApp.
- **Serviços de Campo:** catálogo com tipos fixos + **tipos cadastrados pelo
  admin**; checklists técnicos por tipo; OS em PDF com **assinatura do técnico**.
- **Clientes:** cadastrar (com **criação automática de conta de acesso** — senha
  inicial 123456), **editar**, excluir; ficha técnica de aparelhos com edição
  sincronizada; ligar e WhatsApp.
- **Chamados:** solicitações dos clientes em tempo real — **Agendar Serviço**
  (data + hora, cria agenda e avisa o cliente no WhatsApp), concluir, excluir.
- **Histórico de OS:** todas as OS do banco com garantias; excluir.
- **Configurações:** perfil (Gabriel), empresa (Inovar Refrigeração), **PIX**,
  preços padrão, **assinatura do técnico** (sai na OS), **tipos de serviços
  customizados**, **Central de Conexão WhatsApp** (uma instância ativa por vez,
  status, QR/código por telefone, desconectar e trocar o número conectado) e e-mail (Gmail).
- **Notificações:** sino com histórico + push do navegador (novos chamados,
  orçamentos aprovados, mudanças de status).

## 5. Portal do Cliente (login próprio, layout exclusivo)
- Meus Aparelhos (cadastrar/editar), Solicitar Atendimento (todos os tipos),
  **Meus Orçamentos** (aprovar **assinando com o dedo/mouse** ou recusar),
  Minhas OS com status e garantias, alerta automático de manutenção preventiva,
  alterar senha, meus dados, notificações em tempo real.

## 6. Comunicação Automática
| Evento | Canal |
|---|---|
| Conta criada pelo técnico | WhatsApp com login, senha, link do app e instruções |
| Auto-cadastro do cliente | Boas-vindas no WhatsApp |
| Orçamento criado | Proposta em PDF anexada no WhatsApp do cliente |
| Orçamento aprovado/recusado | Aviso imediato ao técnico |
| OS concluída | Comprovante + garantia em PDF ao cliente |
| Agendamento/reagendamento | Confirmação com data e hora ao cliente |
| Ciclo de manutenção vencido | Lembrete diário automático (09h) |
| Tudo acima | Notificação no app + push do navegador |

## 7. Produção
- App: https://inovarapp.vercel.app (Vercel, projeto inovarapp)
- Cron diário: /api/cron/alertas (09h, protegido por secret)
- Funções serverless (7): contas, whatsapp, email, documentos, configuracoes,
  orcamento-resposta, cron/alertas — service_role nunca exposto ao navegador
- Código espelhado: arcondicapp e clima-hvac-app (git, 100% idênticos)
- E-mail: Gmail SMTP (senha de app — qualquer destinatário, sem domínio)
- WhatsApp: o app usa uma única sessão (`WHATSAPP_OWN_SESSION`, padrão `inovar`) no serviço próprio em Go, configurado no servidor por `WHATSAPP_OWN_URL` e `WHATSAPP_OWN_TOKEN`. É possível desconectar e reconectar o mesmo número ou trocar o número na mesma sessão. O handler Go do app roda na Vercel; a sessão do WhatsApp precisa de processo e armazenamento persistentes e ainda não está configurada em produção neste repositório. O fluxo integrado cobre QR/código por telefone e envios de texto/PDF; caixa de entrada sincronizada e eventos recebidos ainda não estão ligados ao app.

## 8. Validação executada (05/09/2026)
36/37 testes automatizados ✅ — anon bloqueado (8), admin CRUD completo (12),
isolamento do cliente (9), APIs serverless (6), WhatsApp "open" com disparo real,
e-mail Gmail→hotmail entregue, boas-vindas entregue, reagendamento persistido.
Dados de teste removidos; banco com clientes reais (Gabriel, Jhonatan,
Clínica OdontoVix, Marcelo, Camila, Lorraine).
