package handlers

import "strings"

// ValidChronotype reports whether s is an allowed stored chronotype key (empty = unset).
func ValidChronotype(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return true
	}
	switch s {
	case "morning", "midday", "evening", "night", "balanced":
		return true
	default:
		return false
	}
}

// ChronotypePlannerRu — короткая инструкция для LLM (рус.).
func ChronotypePlannerRu(key string) string {
	switch strings.TrimSpace(strings.ToLower(key)) {
	case "morning":
		return "предпочитает требовательную работу утром (примерно до 12:00), тяжёлые задачи ближе к первой половине дня"
	case "midday":
		return "пик продуктивности днём (примерно 12:00–17:00)"
	case "evening":
		return "пик продуктивности вечером (примерно 17:00–22:00)"
	case "night":
		return "лучше соображает поздно вечером и ночью (после 22:00)"
	case "balanced":
		return "без выраженного пика — можно распределять задачи по дню равномерно с учётом занятых слотов"
	default:
		return ""
	}
}
