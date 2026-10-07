# Verificação e deploy — 13/09/2026

Produção: https://inovarapp.vercel.app
Deployment: `dpl_66vCo38bx2ZVUkbAPKquGn4BN3ur` — pronto e publicado em `https://inovarapp.vercel.app`.

Bateria final em produção: **74 verificações aprovadas e 0 falhas**.

## Alterações nesta execução
- Restaurados os painéis de próximos agendamentos e próximos retornos no dashboard.
- Tela Próximos retornos agora apresenta os retornos preventivos, com ação para agendar.
- Retornos de aparelhos já agendados ou em execução não são repetidos como pendentes.
- Serviços em andamento preservados na lista de agendamentos quando têm data.
- Busca passa a ser aplicada também no filtro Chamados.
- Detalhes expandidos da fila usam dados atuais após sincronização.
- Busca tolera campos ausentes em cadastros antigos.
- Ajustados cabeçalho responsivo e altura da fila; removido valor fictício de R$ 180 dos retornos.
- Arquivos locais de credenciais e testes explicitamente excluídos do upload.

## Evidências
- TypeScript: sem erros.
- Build local e build remoto: concluídos; aviso de bundle principal grande permanece.
- Migrações de ciclo de vida e acesso técnico aplicadas ao Supabase.
- Conta administrativa de suporte configurada com troca obrigatória de senha no primeiro acesso.
- ADMIN e TÉCNICO podem gerar uma senha temporária para clientes vinculados; a troca é obrigatória no próximo acesso.
- Cobertura da bateria: RLS, isolamento, autenticação, APIs, assinatura/aprovação/recusa de orçamento, calendário, assets e limpeza dos usuários de teste.
- Produção: página 200; configurações sem autenticação 401; calendário com token inválido 404; cron sem autenticação 401.
- Navegador: página inicial e formulário de entrada renderizados e inspecionados visualmente.
- jsPDF permanece em chunk separado, importado sob demanda no código. Geração/download real de PDF não foi exercitada nesta execução.

## Pendência e limites
O plano Hobby da Vercel permite cron apenas diário; lembretes exatos de uma hora com o app totalmente fechado exigem agendador externo ou plano Pro. O PWA também executa o lembrete local enquanto estiver aberto.

Alterações salvas diretamente na pasta principal solicitada. Sem commit nesta execução.
