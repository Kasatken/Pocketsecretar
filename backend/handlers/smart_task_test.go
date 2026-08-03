package handlers

import (
	"strings"
	"testing"

	"pocketsecret/config"
)

func TestParseAIJSONTasks(t *testing.T) {
	valid := `[{"title":"Отчёт","date":"2031-01-02","time":"10:00","duration_minutes":60,"kind":"task"}]`

	cases := []struct {
		name    string
		in      string
		wantN   int
		wantErr bool
	}{
		{"чистый массив", valid, 1, false},
		{"массив с пробелами и BOM", "﻿  " + valid + "  ", 1, false},
		{"markdown-фенс с языком", "```json\n" + valid + "\n```", 1, false},
		{"текст до фенса", "Вот план на день:\n```json\n" + valid + "\n```", 1, false},
		{"текст до и после массива", "Конечно! " + valid + " Удачи!", 1, false},
		{"одиночный объект вместо массива", `{"title":"Звонок","date":"2031-01-02","time":"12:00"}`, 1, false},
		{"два объекта", `[{"title":"А","date":"2031-01-02","time":"09:00"},{"title":"Б","date":"2031-01-02","time":"11:00"}]`, 2, false},
		{"мусор без JSON", "извините, не могу распланировать", 0, true},
		{"пустая строка", "", 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tasks, err := parseAIJSONTasks(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("ожидалась ошибка, получено %d задач", len(tasks))
				}
				return
			}
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if len(tasks) != c.wantN {
				t.Fatalf("ожидалось %d задач, получено %d", c.wantN, len(tasks))
			}
		})
	}
}

func TestNormalizeTaskDates(t *testing.T) {
	const today = "2031-01-01"

	// Прошедшая дата подтягивается к todayStr (время не проверяем: для date==today
	// normalizeTask может сдвинуть его относительно текущих часов).
	got := normalizeTask(aiTaskRaw{Title: "X", Date: "2030-12-25", Time: "10:00"}, "X", today, 0, config.AppLocation())
	if got.Date != today {
		t.Errorf("прошедшая дата: ожидалось %s, получено %s", today, got.Date)
	}

	got = normalizeTask(aiTaskRaw{Title: "X", Date: "завтра", Time: "10:00"}, "X", today, 0, config.AppLocation())
	if got.Date != "2031-01-02" {
		t.Errorf("завтра: ожидалось 2031-01-02, получено %s", got.Date)
	}
	if got.Time != "10:00" {
		t.Errorf("завтра: время не должно меняться, получено %s", got.Time)
	}

	got = normalizeTask(aiTaskRaw{Title: "X", Date: "2031-01-05", Time: "25:99"}, "X", today, 0, config.AppLocation())
	if got.Time != "09:00" {
		t.Errorf("невалидное время: ожидалось 09:00, получено %s", got.Time)
	}
}

func TestNormalizeTaskDurationsAndEndTime(t *testing.T) {
	const today = "2031-01-01"
	const future = "2031-01-05" // != today, чтобы не сработал сдвиг «время уже прошло»

	// end_time раньше начала — сбрасывается
	got := normalizeTask(aiTaskRaw{Title: "X", Date: future, Time: "10:00", EndTime: "09:00"}, "X", today, 0, config.AppLocation())
	if got.EndTime != "" {
		t.Errorf("end_time <= start: ожидался сброс, получено %q", got.EndTime)
	}

	// duration по умолчанию и клампы
	durCases := []struct {
		kind string
		in   int
		want int
	}{
		{"task", 0, 60},
		{"task", 5, 15},
		{"task", 90, 90},
		{"break", 0, 10},
		{"break", 3, 5},
		{"break", 45, 20},
	}
	for _, c := range durCases {
		got := normalizeTask(aiTaskRaw{Title: "X", Date: future, Time: "10:00", DurationMinutes: c.in, Kind: c.kind}, "X", today, 0, config.AppLocation())
		if got.DurationMinutes != c.want {
			t.Errorf("kind=%s duration=%d: ожидалось %d, получено %d", c.kind, c.in, c.want, got.DurationMinutes)
		}
	}

	// title обрезается до 60 рун (не байт)
	long := strings.Repeat("ю", 70)
	got = normalizeTask(aiTaskRaw{Title: long, Date: future, Time: "10:00"}, "X", today, 0, config.AppLocation())
	if n := len([]rune(got.Title)); n != 60 {
		t.Errorf("title: ожидалось 60 рун, получено %d", n)
	}
}

func TestNormalizeTaskBreakDefaults(t *testing.T) {
	got := normalizeTask(aiTaskRaw{Kind: "BREAK", Date: "2031-01-05", Time: "13:00", Subtasks: []string{"a"}}, "X", "2031-01-01", 0, config.AppLocation())
	if got.Kind != "break" {
		t.Fatalf("kind: ожидалось break, получено %q", got.Kind)
	}
	if got.Title != "Перерыв" {
		t.Errorf("title: ожидалось Перерыв, получено %q", got.Title)
	}
	if got.Subtasks != nil {
		t.Errorf("subtasks у перерыва должны сбрасываться, получено %v", got.Subtasks)
	}
	if got.Context != "health" || got.Methodology != "Тайм-блокинг" || got.EnergyLevel != "low" || got.Priority != "low" {
		t.Errorf("дефолты перерыва: context=%q methodology=%q energy=%q priority=%q", got.Context, got.Methodology, got.EnergyLevel, got.Priority)
	}
}

func TestCapBreakTasksPerDay(t *testing.T) {
	tasks := []aiTaskRaw{
		{Title: "A", Date: "2031-01-05", Kind: "task"},
		{Title: "Чай", Date: "2031-01-05", Kind: "break"},
		{Title: "Глаза", Date: "2031-01-05", Kind: "break"},
		{Title: "Лишний", Date: "2031-01-05", Kind: "break"},
		{Title: "Другой день", Date: "2031-01-06", Kind: "break"},
	}
	out := capBreakTasksPerDay(tasks, 2)
	breaks05, breaks06 := 0, 0
	for i := range out {
		if isBreakTask(&out[i]) {
			if out[i].Date == "2031-01-05" {
				breaks05++
			} else {
				breaks06++
			}
		}
	}
	if breaks05 != 2 || breaks06 != 1 {
		t.Errorf("ожидалось 2 перерыва на 05 и 1 на 06, получено %d и %d", breaks05, breaks06)
	}
}

func TestCapSmartTasksRespectingQuota(t *testing.T) {
	tasks := []aiTaskRaw{
		{Title: "A", Date: "2031-01-05", Time: "09:00", Kind: "task"},
		{Title: "Перерыв", Date: "2031-01-05", Time: "10:00", Kind: "break"},
		{Title: "B", Date: "2031-01-05", Time: "11:00", Kind: "task"},
		{Title: "C", Date: "2031-01-05", Time: "14:00", Kind: "task"},
	}

	out, truncated := capSmartTasksRespectingQuota(tasks, 2)
	if !truncated {
		t.Error("ожидалась обрезка")
	}
	if smartTaskNonBreakCount(out) != 2 {
		t.Errorf("ожидалось 2 обычные задачи, получено %d", smartTaskNonBreakCount(out))
	}
	gotBreak := false
	for i := range out {
		if isBreakTask(&out[i]) {
			gotBreak = true
		}
	}
	if !gotBreak {
		t.Error("перерыв не должен съедать лимит и должен остаться")
	}
	// после обрезки — сортировка по дате и времени начала
	for i := 1; i < len(out); i++ {
		if out[i-1].Date == out[i].Date && out[i-1].Time > out[i].Time {
			t.Errorf("нарушен порядок: %s %s после %s %s", out[i-1].Date, out[i-1].Time, out[i].Date, out[i].Time)
		}
	}

	out, truncated = capSmartTasksRespectingQuota(tasks, 0)
	if out != nil || !truncated {
		t.Errorf("maxNonBreak=0: ожидалось nil/true, получено %d задач/%v", len(out), truncated)
	}
}

func TestNormalizeHM(t *testing.T) {
	cases := map[string]string{
		"9:05":   "09:05",
		"23:59":  "23:59",
		"24:00":  "09:00",
		"10:60":  "09:00",
		"abc":    "09:00",
		"":       "09:00",
		" 8:30 ": "08:30",
	}
	for in, want := range cases {
		if got := normalizeHM(in); got != want {
			t.Errorf("normalizeHM(%q): ожидалось %q, получено %q", in, want, got)
		}
	}
}

func TestCalendarColorID(t *testing.T) {
	cases := []struct {
		name string
		task aiTaskRaw
		want string
	}{
		{"перерыв", aiTaskRaw{Kind: "break", Priority: "high"}, "3"},
		{"high приоритет", aiTaskRaw{Priority: "high"}, "11"},
		{"контекст home", aiTaskRaw{Priority: "medium", Context: "home"}, "1"},
		{"бытовой title без context", aiTaskRaw{Priority: "low", Title: "Купить продукты по списку"}, "1"},
		{"medium", aiTaskRaw{Priority: "medium", Title: "Созвон"}, "5"},
		{"без приоритета", aiTaskRaw{Title: "Задача"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := calendarColorID(&c.task); got != c.want {
				t.Errorf("ожидалось %q, получено %q", c.want, got)
			}
		})
	}
}
