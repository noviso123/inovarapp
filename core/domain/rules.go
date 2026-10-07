package domain

import (
	"strings"
	"time"
)

// ReturnStatusFor compara datas civis, ignorando horário e fuso, como a regra de calendário do cliente.
func ReturnStatusFor(target, today time.Time) ReturnStatus {
	targetDay := civilDayNumber(target)
	todayDay := civilDayNumber(today)
	days := targetDay - todayDay
	switch {
	case days < 0:
		return ReturnOverdue
	case days <= 7:
		return ReturnThisWeek
	case days <= 30:
		return ReturnSoon
	default:
		return ReturnOnTime
	}
}

// ReturnStatusForDate aplica o comportamento do armazenamento atual: retorno vazio
// significa sem histórico e uma data inválida mantém o cliente em dia.
func ReturnStatusForDate(returnDate string, today time.Time) ReturnStatus {
	if returnDate == "" {
		return ReturnNoHistory
	}
	target, err := parseLegacyISODate(returnDate, today.Location())
	if err != nil {
		return ReturnOnTime
	}
	return ReturnStatusFor(target, today)
}

func parseLegacyISODate(value string, location *time.Location) (time.Time, error) {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return parsed, nil
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, nil
	}
	if location == nil {
		location = time.UTC
	}
	return time.ParseInLocation("2006-01-02T15:04:05.999999999", value, location)
}

// ReturnStatusLabel is the shared Portuguese copy used by the legacy client
// for return-state badges and filters.
func ReturnStatusLabel(status ReturnStatus) string {
	switch status {
	case ReturnOverdue:
		return "Atrasado"
	case ReturnThisWeek:
		return "Esta semana"
	case ReturnSoon:
		return "Em breve"
	case ReturnNoHistory:
		return "Sem histórico"
	default:
		return "Em dia"
	}
}

type ReturnStatusStyle struct {
	BadgeBackground string
	BadgeText       string
	Border          string
	Accent          string
	Gradient        string
}

// ReturnStatusPresentation keeps the visual tokens from StorageService
// available to all Go frontends without copying status-color decisions.
func ReturnStatusPresentation(status ReturnStatus) ReturnStatusStyle {
	switch status {
	case ReturnOverdue:
		return ReturnStatusStyle{"bg-red-500/20 text-red-400 border border-red-500/40", "text-red-400", "border-l-red-500", "text-red-400", "from-red-950/40 via-slate-900 to-slate-900"}
	case ReturnThisWeek:
		return ReturnStatusStyle{"bg-amber-500/20 text-amber-300 border border-amber-500/40", "text-amber-300", "border-l-amber-400", "text-amber-300", "from-amber-950/40 via-slate-900 to-slate-900"}
	case ReturnSoon:
		return ReturnStatusStyle{"bg-sky-500/20 text-sky-300 border border-sky-500/40", "text-sky-300", "border-l-sky-400", "text-sky-300", "from-sky-950/40 via-slate-900 to-slate-900"}
	case ReturnNoHistory:
		return ReturnStatusStyle{"bg-violet-500/20 text-violet-300 border border-violet-500/40", "text-violet-300", "border-l-violet-400", "text-violet-300", "from-violet-950/40 via-slate-900 to-slate-900"}
	default:
		return ReturnStatusStyle{"bg-emerald-500/20 text-emerald-400 border border-emerald-500/40", "text-emerald-400", "border-l-emerald-500", "text-emerald-400", "from-emerald-950/40 via-slate-900 to-slate-900"}
	}
}

func civilDayNumber(value time.Time) int64 {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Unix() / 86400
}

// AddMonthsClamped reproduz a semântica date-fns: se o dia não existir no mês alvo,
// usa o último dia daquele mês, preservando horário e localização.
func AddMonthsClamped(value time.Time, months int) time.Time {
	first := time.Date(value.Year(), value.Month(), 1, value.Hour(), value.Minute(), value.Second(), value.Nanosecond(), value.Location())
	first = first.AddDate(0, months, 0)
	lastDay := time.Date(first.Year(), first.Month()+1, 0, value.Hour(), value.Minute(), value.Second(), value.Nanosecond(), value.Location()).Day()
	day := value.Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(first.Year(), first.Month(), day, value.Hour(), value.Minute(), value.Second(), value.Nanosecond(), value.Location())
}

// NormalizeServiceName reconhece os rótulos legados com acentos ou corrupção de codificação.
func NormalizeServiceName(name string) string {
	trimmed := strings.TrimSpace(name)
	lower := strings.ToLower(trimmed)
	key := make([]rune, 0, len(lower))
	for _, r := range lower {
		if r == '\uFFFD' {
			continue
		}
		if ascii := portugueseASCII(r); ascii != 0 {
			key = append(key, ascii)
			continue
		}
		if r >= 'a' && r <= 'z' {
			key = append(key, r)
		}
	}
	compact := string(key)
	if compact == "" {
		return "Outro"
	}
	if strings.Contains(compact, "avali") && (strings.Contains(compact, "tecn") || strings.Contains(compact, "tcn")) {
		return string(ServiceTechnicalAssessment)
	}
	if compact == "avaliacao" {
		return string(ServiceTechnicalAssessment)
	}
	if compact == "limpeza" || compact == "limpezadearcompleta" {
		return "Limpeza de Ar"
	}
	return trimmed
}

func portugueseASCII(r rune) rune {
	switch r {
	case 'á', 'à', 'â', 'ã', 'ä':
		return 'a'
	case 'ç':
		return 'c'
	case 'é', 'è', 'ê', 'ë':
		return 'e'
	case 'í', 'ì', 'î', 'ï':
		return 'i'
	case 'ó', 'ò', 'ô', 'õ', 'ö':
		return 'o'
	case 'ú', 'ù', 'û', 'ü':
		return 'u'
	default:
		return 0
	}
}
