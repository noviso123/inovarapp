package domain

import (
	"regexp"
	"strings"
)

type WhatsAppTemplate struct {
	Key, Title, When, Default string
	Placeholders              []string
}

const AppPublicURL = "https://inovarapp.vercel.app"

var whatsappPlaceholder = regexp.MustCompile(`\{\{(\w+)\}\}`)

var whatsappTemplates = []WhatsAppTemplate{
	{Key: "boas_vindas_autocadastro", Title: "Boas-vindas (auto-cadastro)", When: "Cliente cria a própria conta no app", Placeholders: []string{"{{cliente}}", "{{email}}", "{{empresa}}", "{{app}}"}, Default: "🎉 *Bem-vindo(a) à {{empresa}}*, {{cliente}}!\n\nSua conta no *InovarApp* foi criada com o e-mail:\n📧 {{email}}\n\n📲 *Acesse:* {{app}}\n\nNo app você acompanha seus aparelhos, solicita atendimentos, recebe orçamentos com assinatura e consulta suas garantias! ❄️"},
	{Key: "boas_vindas_tecnico", Title: "Boas-vindas (conta criada pelo técnico)", When: "Técnico cadastra cliente com acesso ao app", Placeholders: []string{"{{cliente}}", "{{email}}", "{{senha}}", "{{empresa}}", "{{app}}"}, Default: "🎉 *Bem-vindo(a) à {{empresa}}*, {{cliente}}!\n\nSua conta de acesso ao *InovarApp* foi criada:\n📧 *Login:* {{email}}\n🔑 *Senha:* {{senha}}\n\n📲 *Acesse aqui:* {{app}}\n\n*Como entrar:*\n1. Abra o link acima\n2. Toque em *Entrar no App*\n3. Use o login e a senha acima\n\n💡 No app você acompanha seus aparelhos, orçamentos, agendamentos e garantias. Recomendamos trocar a senha em *Meus Dados → Alterar Senha*.\n\n*{{empresa}}* ❄️"},
	{Key: "orcamento_criado", Title: "Orçamento criado (com PDF anexo)", When: "Técnico gera um orçamento", Placeholders: []string{"{{cliente}}", "{{tecnico}}", "{{empresa}}", "{{numero}}", "{{valor}}", "{{itens}}", "{{equipamento}}", "{{data}}", "{{validade}}", "{{desconto}}", "{{pagamento}}", "{{prazo}}", "{{garantia}}", "{{observacoes}}", "{{app}}"}, Default: "Olá, *{{cliente}}*! Tudo bem? 😊\n\nAqui é *{{tecnico}}* da *{{empresa}}*.\n\nSegue a sua *Proposta/Orçamento nº #{{numero}}*\n❄️ *Equipamento:* {{equipamento}}\n📅 *Data:* {{data}} | *Validade:* {{validade}}\n\n🛠️ *Itens do orçamento:*\n{{itens}}\n{{desconto}}💰 *Valor total:* *{{valor}}*\n💳 *Forma de pagamento:* {{pagamento}}\n⏱️ *Prazo de execução:* {{prazo}}\n🛡️ *Garantia:* {{garantia}}\n{{observacoes}}\n\n📄 A proposta completa em PDF está anexada.\n📲 Acompanhe tudo pelo app: {{app}}\n\nPodemos agendar a execução para esta semana? Fico à disposição para tirar qualquer dúvida! ❄️🤝"},
	{Key: "os_concluida", Title: "OS concluída (comprovante + garantia em PDF)", When: "Serviço de campo é concluído", Placeholders: []string{"{{cliente}}", "{{os}}", "{{servico}}", "{{data}}", "{{garantia}}", "{{empresa}}", "{{app}}"}, Default: "Olá, {{cliente}}! ✅\n\nSua *Ordem de Serviço {{os}}* foi concluída com sucesso!\n\n❄️ Serviço: {{servico}}\n📅 Data: {{data}}\n🛡️ *Garantia: {{garantia}}*\n\n📄 A OS completa em PDF (comprovante e garantia) está anexada.\n\nObrigado pela confiança! *{{empresa}}* ❄️\n\n📲 Acesse seu portal: {{app}}"},
	{Key: "agendamento_criado", Title: "Agendamento criado (retorno pelo painel)", When: "Técnico agenda um retorno", Placeholders: []string{"{{cliente}}", "{{data}}", "{{servico}}", "{{empresa}}"}, Default: "Olá, {{cliente}}! 📅 Seu atendimento ficou *AGENDADO* para *{{data}}* — {{servico}}. Qualquer imprevisto, é só nos chamar! *{{empresa}}* ❄️"},
	{Key: "chamado_agendado", Title: "Chamado agendado (data e hora)", When: "Técnico agenda um chamado do cliente", Placeholders: []string{"{{cliente}}", "{{servico}}", "{{data}}", "{{hora}}", "{{empresa}}", "{{app}}"}, Default: "📅 *Atendimento AGENDADO!*\n\nOlá, {{cliente}}! Seu {{servico}} foi agendado para *{{data}} às {{hora}}*.\n\n📲 Acompanhe tudo pelo app: {{app}}\n\n*{{empresa}}* ❄️"},
	{Key: "orcamento_agendado", Title: "Orçamento aprovado → agendado", When: "Serviço do orçamento aprovado é agendado", Placeholders: []string{"{{cliente}}", "{{data}}", "{{hora}}", "{{valor}}", "{{empresa}}", "{{app}}"}, Default: "📅 *Atendimento AGENDADO!*\n\nOlá, {{cliente}}! Seu serviço do orçamento aprovado foi agendado para *{{data}} às {{hora}}* (valor {{valor}}).\n\n📲 Acompanhe pelo app: {{app}}\n\n*{{empresa}}* ❄️"},
	{Key: "servico_registrado", Title: "Serviço registrado (cadastro direto)", When: "Técnico cadastra um serviço sem checklist", Placeholders: []string{"{{cliente}}", "{{servico}}", "{{data}}", "{{empresa}}"}, Default: "📅 *Serviço registrado!* Olá, {{cliente}}! Seu {{servico}} ficou agendado para *{{data}}*. *{{empresa}}* ❄️"},
	{Key: "status_agendado", Title: "Status: AGENDADO", When: "Status do serviço muda para agendado", Placeholders: []string{"{{cliente}}", "{{servico}}", "{{empresa}}"}, Default: "📅 Seu serviço ({{servico}}) foi *AGENDADO* pela {{empresa}}. Em breve confirmaremos a data exata! ❄️"},
	{Key: "status_em_andamento", Title: "Status: EM ANDAMENTO", When: "Técnico está a caminho / executando", Placeholders: []string{"{{cliente}}", "{{servico}}", "{{empresa}}"}, Default: "🔧 *Técnico a caminho / em execução!* Seu serviço ({{servico}}) está em andamento agora. ❄️"},
	{Key: "status_concluido", Title: "Status: CONCLUÍDO", When: "Serviço marcado como concluído", Placeholders: []string{"{{cliente}}", "{{servico}}", "{{empresa}}"}, Default: "✅ Seu serviço ({{servico}}) foi *CONCLUÍDO* com garantia ativa. Obrigado pela confiança! ❄️"},
	{Key: "status_cancelado", Title: "Status: CANCELADO", When: "Serviço cancelado", Placeholders: []string{"{{cliente}}", "{{servico}}", "{{empresa}}"}, Default: "⚠️ Seu serviço ({{servico}}) foi cancelado. Fale conosco para reagendarmos! ❄️"},
	{Key: "os_status_atualizado", Title: "OS atualizada (edição da OS)", When: "Técnico edita a OS e conclui por lá", Placeholders: []string{"{{cliente}}", "{{status}}", "{{empresa}}"}, Default: "✅ Seu serviço foi atualizado: *{{status}}* — novo status no app. *{{empresa}}* ❄️"},
	{Key: "reagendado", Title: "Atendimento reagendado", When: "Data ou horário do atendimento é alterado", Placeholders: []string{"{{cliente}}", "{{servico}}", "{{data}}", "{{hora}}", "{{empresa}}"}, Default: "📅 *Atendimento REAGENDADO*\n\nOlá, {{cliente}}! Seu atendimento ({{servico}}) foi reagendado para *{{data}} às {{hora}}*.\n\nQualquer dúvida, é só nos chamar! *{{empresa}}* ❄️"},
	{Key: "orcamento_aprovado_cliente", Title: "Orçamento aprovado (confirmação ao cliente)", When: "Cliente aprova a proposta com assinatura", Placeholders: []string{"{{cliente}}", "{{empresa}}", "{{numero}}", "{{valor}}", "{{app}}"}, Default: "✅ *Recebemos sua aprovação*, {{cliente}}!\n\nProposta nº {{numero}} ({{valor}}) confirmada com assinatura digital.\n\nNossa equipe vai entrar em contato para agendar o serviço. Qualquer dúvida, chame por aqui! ❄️\n\n📲 Acompanhe pelo app: {{app}}\n\n*{{empresa}}*"},
	{Key: "orcamento_aprovado_tecnico", Title: "Aviso: orçamento aprovado (para a empresa)", When: "Cliente aprova — avisa o técnico/empresa", Placeholders: []string{"{{cliente}}", "{{numero}}", "{{empresa}}"}, Default: "✅ *Orçamento aprovado e assinado!*\n\nO cliente *{{cliente}}* aprovou a proposta (assinatura registrada). Hora de agendar o serviço!\n\n— InovarApp"},
	{Key: "orcamento_recusado_tecnico", Title: "Aviso: orçamento recusado (para a empresa)", When: "Cliente recusa a proposta", Placeholders: []string{"{{cliente}}", "{{numero}}", "{{empresa}}"}, Default: "⚠️ *Orçamento recusado.*\n\nO cliente *{{cliente}}* recusou a proposta.\n\n— InovarApp"},
	{Key: "lembrete_ciclo_vencido", Title: "Lembrete: ciclo de manutenção vencido", When: "Agendador Supabase a cada 15 min — clientes com ciclo em atraso/vencendo, respeitando o intervalo configurado", Placeholders: []string{"{{cliente}}", "{{empresa}}", "{{equipamento}}", "{{data_ultima}}", "{{situacao}}", "{{meses}}"}, Default: "Olá, {{cliente}}! Tudo bem? 😊\n\nAqui é a *{{empresa}}*.\n\n❄️ Pela nossa ficha técnica, a manutenção/limpeza de ar do seu *{{equipamento}}* realizada em {{data_ultima}} já {{situacao}} (ciclo recomendado de {{meses}} meses).\n\nManter o ciclo evita fungos, bactérias, mau cheiro e maior consumo de energia. Posso agendar sua visita? 🗓️\n\n{{empresa}}"},
	{Key: "lembrete_uma_hora", Title: "Lembrete: atendimento em aproximadamente 1 hora", When: "Agendador Supabase a cada 15 min — serviço agendado entre 45 e 75 min", Placeholders: []string{"{{cliente}}", "{{empresa}}", "{{data}}", "{{hora}}", "{{servico}}"}, Default: "Olá, {{cliente}}! ⏰\n\nPassando para lembrar que seu atendimento da *{{empresa}}* está previsto para hoje, às *{{hora}}* ({{data}}).\n\n🔧 Serviço: {{servico}}\n\nSe houver algum imprevisto, responda por aqui. Até breve! ❄️"},
	{Key: "lembrete_vespera", Title: "Lembrete: atendimento é amanhã", When: "Cron diário 09h — véspera do atendimento", Placeholders: []string{"{{cliente}}", "{{empresa}}", "{{data}}", "{{hora}}", "{{servico}}", "{{equipamento}}"}, Default: "Olá, {{cliente}}! Tudo bem? 😊\n\n*{{empresa}}* passando para lembrar:\n\n📅 Seu atendimento está *AGENDADO PARA AMANHÃ* ({{data}}{{hora}}).\n❄️ Serviço: {{servico}}\n🏠 Equipamento: {{equipamento}}\n\nQualquer imprevisto, é só nos chamar por aqui! Estamos à disposição. ❄️"},
}

func WhatsAppTemplates() []WhatsAppTemplate {
	result := make([]WhatsAppTemplate, len(whatsappTemplates))
	for i, model := range whatsappTemplates {
		model.Placeholders = append([]string(nil), model.Placeholders...)
		result[i] = model
	}
	return result
}

func WhatsAppTemplateByKey(key string) (WhatsAppTemplate, bool) {
	for _, model := range whatsappTemplates {
		if model.Key == key {
			model.Placeholders = append([]string(nil), model.Placeholders...)
			return model, true
		}
	}
	return WhatsAppTemplate{}, false
}

func ResolveWhatsAppTemplate(messages map[string]string, key string) string {
	if value := strings.TrimSpace(messages[key]); value != "" {
		return messages[key]
	}
	model, ok := WhatsAppTemplateByKey(key)
	if !ok {
		return ""
	}
	return model.Default
}

func ApplyWhatsAppPlaceholders(message string, values map[string]string) string {
	return whatsappPlaceholder.ReplaceAllStringFunc(message, func(match string) string {
		key := match[2 : len(match)-2]
		return values[key]
	})
}
