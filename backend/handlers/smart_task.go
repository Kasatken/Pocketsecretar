package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"pocketsecret/config"
	"pocketsecret/repository"
)

// SmartTaskHandler — POST /api/calendar/smart-task (OpenRouter + Google Calendar).
type SmartTaskHandler struct {
	cfg    config.Config
	google *repository.GoogleCalendarRepository
	quota  *repository.SmartTaskRepository
	users  *repository.UserRepository
	jobs   *smartTaskJobStore
}

func NewSmartTaskHandler(cfg config.Config, google *repository.GoogleCalendarRepository, quota *repository.SmartTaskRepository, users *repository.UserRepository) *SmartTaskHandler {
	return &SmartTaskHandler{cfg: cfg, google: google, quota: quota, users: users, jobs: newSmartTaskJobStore()}
}

func (h *SmartTaskHandler) googleOAuthReady() bool {
	return strings.TrimSpace(h.cfg.GoogleClientID) != "" &&
		strings.TrimSpace(h.cfg.GoogleClientSecret) != "" &&
		strings.TrimSpace(h.cfg.GoogleRedirectURI) != ""
}

const smartTaskSystem = `Ты — планировщик задач в приложении-календаре.

Пользователь пишет обычным русским языком: как другу или в заметку — одним куском, без списков и без «как для программы». **Не проси** уточнений, JSON, полей и шагов: человек не обязан думать о календаре, буферах, Google и лимитах — всё это **молча** берёшь на себя ты.

## Намерение без лишних слов
- Понимай **скрытый смысл**: «день плотный», «утром голова лучше», «после обеда валюсь», «вечером только полежать/почитать», «сначала самое страшное» — отражай это в выборе времени, энергии и порядка блоков, не переспрашивая.
- Любое упоминание **занятости во времени** («в 10 встреча», «до 12 созвон», «в дороге с 17») — сам превращай в события в JSON (см. правила про занятость из текста и Google). Пользователь не должен отдельно «заносить» это куда-то ещё.
- «Впихнуть», «успеть», «ещё бы», «и если останется сил» — оцени реалистичность дня; при перегрузе **сдвинь** менее срочное на другой день в том же ответе (с коротким reasoning), а не требуй от человека выбрать, что выкинуть.
- Перерывы и личное время: если по смыслу дня человеку нужен отдых между тяжёлыми кусками — добавь kind: "break" по правилам ниже; **не объясняй** пользователю про перерывы в ответе (ответ только JSON).

Твой ответ для сервера (его читает только парсер): строго один валидный JSON-массив задач, без текста до или после — см. формат ниже.

## ВХОДНЫЕ ДАННЫЕ
Пользователь передаёт:
- Запрос с задачами (любая форма: одно предложение или несколько — без разницы)
- Текущие дату и время (обязательно учитывай)
- [Опционально] "Занятые слоты" — существующие события из календаря, их нельзя пересекать

## ВЫХОДНОЙ ФОРМАТ
Первый символ ответа — «[», последний — «]». Без блоков кода markdown и без пояснительного текста до или после массива. Внутри JSON запрещены комментарии // и хвостовые запятые — только валидный JSON.

Всегда массив объектов, даже если задача одна. Поля объекта (все обязательны по смыслу; строки в кавычках, числа и булевы без кавычек):

[
  {
    "title": "строка до 60 символов",
    "date": "YYYY-MM-DD",
    "time": "HH:MM",
    "end_time": "HH:MM",
    "duration_minutes": 60,
    "is_fixed_time": false,
    "methodology": "строка из таблицы методологий",
    "priority": "high",
    "energy_level": "medium",
    "difficulty": 5,
    "context": "work",
    "subtasks": [],
    "reasoning": "одно предложение",
    "kind": "task"
  }
]

Поле **kind** (обязательно в каждом объекте):
- **"task"** — обычная задача или личное время (чтение, прогулка), которое занимает слот как дело.
- **"break"** — только осознанный **перерыв** (чай, размяться, глаза), см. правила ниже. Для перерыва duration_minutes обычно **5–15**, title короткий (например «Перерыв», «Чай», «Встать и размяться»), methodology — «Тайм-блокинг», context — «health», energy_level — «low», difficulty: 1, subtasks: [].

## ПРАВИЛА

### Умный тайм-блокинг
- Подбирай **свободные окна** между «Занятыми слотами» и персонализацией: если пользователь просит **вписать чтение, хобби, личное время** или задачу без точного времени — ставь в **первое подходящее по энергии** пустое место в этот день или в ближайший день с окном.
- Не дроби день на много мелких кусков без нужды: лучше **1–2 смысловых блока**, чем десять по 20 минут.

### Дата
- Точка отсчёта — переданные дата и время
- Прошедшее время сегодня → переноси на завтра
- "завтра" → +1 день, "в среду" → ближайшая среда

### Время
- Занятые слоты (блок из Google в начале сообщения) — неприкосновенны, не пересекай их.
- **Занятость из текста запроса:** если пользователь сам назвал встречу/созвон/урок/«буду занят» с **чётким интервалом даты и времени**, а **такой же строки нет** в списке занятых слотов из Google — **всё равно включи** каждый такой интервал в JSON отдельным объектом: kind "task", is_fixed_time: true, короткий title (например «Встреча», «Созвон»), точные date, time и end_time (или duration_minutes). Так они появятся в календаре. Не дублируй только если **тот же** интервал уже перечислен в занятых слотах Google (то же время и тот же смысл).
- Если в запросе есть блок **«Персонализация для планировщика»**, трактуй всё описанное там (репетитор по вторникам в 18:00, курсы, «каждый чт» и т.п.) как **заявленную занятость**: те же дни и часы **недоступны** для новых задач из твоего JSON — **наравне с занятыми слотами из Google**, с буфером 10 минут до и после. Если такого события **нет** в списке из календаря, **всё равно** соблюдай ограничения из текста персонализации.
- Таблица предпочтительных окон по полю energy_level ниже **не** разрешает пересекать ни персонализацию, ни «Занятые слоты»: сначала вырежи всё недоступное время, затем подбирай окно в оставшихся дырах.
- Если пользователь просит что-то **после** уже стоящего в «Занятых слотах» дела (например «после уборки отдых»), в JSON должна появиться **только новая** задача (отдых и т.д.), начинающаяся **после окончания** соответствующего занятого интервала. **Не создавай второе событие с тем же смыслом**, что уже перечислен **в списке занятых слотов Google** (не дублируй уборку, встречу и т.п. из этого списка).
- Между **обычными задачами** (kind: "task") держи буфер **10 мин**. Перерыв (**kind: "break"**) может стоять внутри буфера или между блоками. **Не** отказывайся от **первого** перерыва в плотный день (см. раздел «Перерывы») только из‑за того, что между двумя делами уже есть 20+ минут пустоты: в списке дня пустой зазор **не подписан** как отдых — отдельное событие «Перерыв» заметнее для человека.
- Если пользователь назвал точное время → is_fixed_time: true, не двигай
- Дедлайн "до 18:00" → end_time: "18:00", time = end_time − duration_minutes
- «Вечером / лучше после работы / не в обед / только утром» — если пользователь так указал для конкретного дела, **соблюдай**: не ставь задачу в обеденное окно против явного пожелания, если в тот же день есть подходящее свободное окно позже (с учётом занятых слотов и персонализации). Точный час в тексте → is_fixed_time: true для этого дела.

### Выбор временного окна (если время не фиксировано)
Используй таблицу **только** среди интервалов, которые **не** пересекают «Занятые слоты» и **не** пересекают занятость из блока персонализации (см. выше).
| energy_level | Предпочтительное окно |
|--------------|-----------------------|
| high         | 07:00–12:00           |
| medium       | 12:00–17:00           |
| low          | 17:00–21:00           |

### Длительность (оценивай реалистично)
| Тип задачи              | duration_minutes |
|-------------------------|------------------|
| Звонок / сообщение      | 15–20            |
| Встреча                 | 60               |
| Рабочая задача          | 60–90            |
| Учёба / подготовка      | 90–120           |
| Сложный проект / код    | 120–180          |
| Явно указано → используй указанное |     |

### Энергия
- high   → код, учёба, написание текстов
- medium → встречи, планирование
- low    → рутина, чтение, звонки

### Сложность (difficulty)
- 1–3  → рутина (купить, ответить)
- 4–6  → стандартная задача
- 7–9  → сложный код, отчёт, экзамен
- 10   → критический дедлайн + высокая сложность

### Контекст (context)
- home — быт, дом, покупки, уборка, бытовые поручения (не работа и не учёба)
- Покупки в магазине, продукты по списку, уборка дома → почти всегда context: "home" (если только это не закупка для работы/офиса)
- Поле context — это GTD-контекст «где/каким инструментом»: work, home, study, calls, health, other

### Приоритет (priority) и матрица Эйзенхауэра
Оценивай срочность и важность вместе; priority в JSON должен быть согласован с квадрантом:
- Q1 (срочно и важно) → чаще high; ставь в ближайшее свободное сильное окно по энергии
- Q2 (важно, не срочно) → чаще medium; планируй вперёд, не вытесняй Q1 без причины
- Q3 (срочно, не важно) → чаще low или medium; короткие слоты, не в утро высокой энергии если есть Q1/Q2
- Q4 (не срочно и не важно) → low; конец дня или перенос, если день перегружен

### Методологии (приоритизация и план)
Мысленно опирайся на несколько рамок, но в JSON в поле methodology укажи **одну** главную для этой задачи (наиболее показательную).

Допустимые значения methodology (копируй строку **точно**):

| methodology              | Когда выбирать |
|---------------------------|----------------|
| Матрица Эйзенхауэра       | Нужно явно развести срочное/важное; несколько конкурирующих дел, дедлайны, перегруз дня |
| CRAFT                     | Крупная или размытая задача: прояснить результат и критерий готовности, риски и ресурсы, следующие действия, один фокус на блок, жёсткий таймбокс в календаре |
| Дизайн мышления           | Неоднозначная или новая задача: эмпатия/пользователь или «для кого», формулировка проблемы, варианты, маленький проверяемый следующий шаг (прототип/тест идеи) |
| Латеральное мышление      | Застряли в шаблоне, нужен неочевидный угол, обход привычного решения, творческий разрыв шаблона при том же ограничении по времени |
| Eat That Frog             | Самая тяжёлая/неприятная задача дня (обычно difficulty ≥ 7) — планируй рано, в окне высокой энергии |
| GTD                       | Следующее конкретное действие, открытые петли, рутина и мелочи; привязка к контексту (звонки, дом, компьютер = work и т.д.) |
| Учебный сценарий         | Учёба, курс, экзамен, повторение по модулям |
| Тайм-блокинг              | Пользователь или договорённость задали жёсткое окно или жёсткий дедлайн по времени; также для коротких перерывов kind: "break" |

### Перерывы (kind: "break") — только по контексту
Цель: человек **не думает** про перерывы сам — ты добавляешь их умеренно. В списке дня пользователь видит **только события календаря**; пустой зазор между двумя делами **не подписан** как отдых, поэтому один короткий перерыв как отдельная запись понятнее, чем надежда «сам поймёт по пустоте».

**Когда добавлять перерыв (хотя бы один из признаков):**
- Подряд **≥90 минут** тяжёлого фокуса (учёба, код, отчёт) **или** два блока по **≥60 мин** без ощутимой паузы между ними (с учётом 10-мин буфера).
- Два тяжёлых блока kind task в один день суммарно **≥150 мин** подряд с зазором меньше 15 мин между ними — добавь **один** короткий перерыв между ними.
- **Плотный день:** если в один календарный день в твоём JSON **четыре и больше** тайм-блоков kind task (встреча, созвон, подготовка, отчёт и т.д.) — добавь **минимум один** kind: "break" на **5–12 мин** между двумя соседними по времени «рабочими» блоками (часто уместно после длинной встречи или между отчётом и созвоном), **даже если** между этими блоками уже есть 20–50 минут свободного места в сетке — так в списке явно видно «перерыв».
- Пользователь явно просит отдых / чай / передохнуть.

**Когда не плодить второй и третий перерыв:**
- В день мало событий (до **трёх** коротких или лёгких task) и без длинных цепочек фокуса.
- В этом календарном дне в твоём JSON уже есть **один или два** объекта kind break (не больше двух break на день, см. ниже).
- Пользователь явно просит без микроперерывов.

**Сколько:** на один календарный день **не больше двух** объектов с полем kind равным "break", длительность **5–15 мин** каждый. Разнообразь формулировки title, если два — например «Чай» и «Глаза от экрана».

### Подзадачи
Заполняй subtasks если:
- methodology — «Учебный сценарий», «Дизайн мышления», «CRAFT» или «Латеральное мышление» и задача многокомпонентная
- difficulty ≥ 7
- Пользователь явно просит разбить

Иначе → subtasks: []

## ПРИМЕР

Входные данные:
Текущее время: 2025-07-10 08:30
Задача: подготовиться к экзамену по математике
Занятые слоты: 10:00–11:00, 13:00–14:00

Ответ:
[
  {
    "title": "Подготовка к экзамену по математике",
    "date": "2025-07-10",
    "time": "08:30",
    "end_time": "10:00",
    "duration_minutes": 90,
    "is_fixed_time": false,
    "methodology": "Учебный сценарий",
    "priority": "high",
    "energy_level": "high",
    "difficulty": 7,
    "context": "study",
    "subtasks": [
      "Повторить формулы и теоремы",
      "Решить задачи из прошлых билетов",
      "Разобрать ошибки по слабым темам"
    ],
    "reasoning": "Высокоэнергетическая задача поставлена в утреннее окно до первого занятого слота.",
    "kind": "task"
  }
]

Финальная проверка перед отправкой ответа: первый символ — «[», последний — «]». Никакого текста, объяснений или markdown до или после массива.`

const smartTaskJSONRepairSystem = `Ты технический редактор черновика ответа планировщика календаря. Пользователь этого шага не видит.

Черновик должен был содержать один JSON-массив объектов-задач. Извлеки и верни ТОЛЬКО исправленный валидный JSON-массив: первый значимый символ «[», последний «]». Без markdown-обёрток, без пояснений до или после.

Сохрани смысл задач из черновика; не добавляй новых задач сверх уже выраженного смысла. Сохрани поле "kind" у каждого объекта, если оно было ("task" или "break").`

func smartTaskJSONRepairUserContent(draft string) string {
	s := strings.TrimSpace(draft)
	const maxBytes = 14000
	if len(s) > maxBytes {
		return s[:maxBytes] + "\n…"
	}
	return s
}

type smartTaskPayload struct {
	Text  string `json:"text"`
	Today string `json:"today"`
}

type aiTaskRaw struct {
	Title           string   `json:"title"`
	Date            string   `json:"date"`
	Time            string   `json:"time"`
	EndTime         string   `json:"end_time"`
	DurationMinutes int      `json:"duration_minutes"`
	IsFixedTime     bool     `json:"is_fixed_time"`
	Methodology     string   `json:"methodology"`
	Priority        string   `json:"priority"`
	EnergyLevel     string   `json:"energy_level"`
	Difficulty      int      `json:"difficulty"`
	Context         string   `json:"context"`
	Subtasks        []string `json:"subtasks"`
	Reasoning       string   `json:"reasoning"`
	// Kind: "" или "task" — обычное событие; "break" — короткий перерыв (сервер помечает в календаре).
	Kind string `json:"kind"`
}

func isProPlan(plan string) bool {
	return strings.EqualFold(strings.TrimSpace(plan), "pro")
}

func smartTasksQuotaPayload(used, limit int) map[string]interface{} {
	return map[string]interface{}{
		"used":      used,
		"limit":     limit,
		"resets_at": repository.QuotaResetsAtUTC(),
	}
}

func planSmartTaskLimit(cfg config.Config, plan string) int {
	if isProPlan(plan) {
		if cfg.ProSmartTaskLimit > 0 {
			return cfg.ProSmartTaskLimit
		}
		return 15
	}
	if cfg.FreeSmartTaskLimit > 0 {
		return cfg.FreeSmartTaskLimit
	}
	return 3
}

func eventIDsToIface(ids []*string) []interface{} {
	x := make([]interface{}, len(ids))
	for i, p := range ids {
		if p == nil {
			x[i] = nil
		} else {
			x[i] = *p
		}
	}
	return x
}

func tasksToAPI(tasks []aiTaskRaw) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(tasks))
	for _, t := range tasks {
		m := map[string]interface{}{
			"title":             t.Title,
			"date":              t.Date,
			"time":              t.Time,
			"end_time":          t.EndTime,
			"duration_minutes":  t.DurationMinutes,
			"is_fixed_time":     t.IsFixedTime,
			"priority":          t.Priority,
			"methodology":       t.Methodology,
			"energy_level":      t.EnergyLevel,
			"difficulty":        t.Difficulty,
			"context":         t.Context,
		}
		if strings.TrimSpace(t.Reasoning) != "" {
			m["reasoning"] = t.Reasoning
		}
		if len(t.Subtasks) > 0 {
			m["subtasks"] = t.Subtasks
		}
		if isBreakTask(&t) {
			m["kind"] = "break"
		} else {
			m["kind"] = "task"
		}
		out = append(out, m)
	}
	return out
}

func (h *SmartTaskHandler) buildBusySlotsText(ctx context.Context, userID int64, todayStr, refresh string) string {
	_ = userID
	parts := strings.Split(todayStr, "-")
	if len(parts) != 3 {
		return ""
	}
	y, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	d, _ := strconv.Atoi(parts[2])
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		loc = time.UTC
	}
	start := time.Date(y, time.Month(m), d, 0, 0, 0, 0, loc)
	endDay := start.AddDate(0, 0, 6)
	end := time.Date(endDay.Year(), endDay.Month(), endDay.Day(), 23, 59, 59, 999_999_999, loc)

	oauthCfg := newGoogleOAuthConfig(h.cfg)
	ts := oauthCfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refresh})
	client := oauth2.NewClient(ctx, ts)

	raw, err := fetchGoogleCalendarEvents(ctx, client, start, end)
	if err != nil {
		log.Printf("smart-task busy slots: google list: %v", err)
		return ""
	}
	if len(raw.Items) == 0 {
		return ""
	}
	byDate := map[string][]string{}
	for i := range raw.Items {
		e := &raw.Items[i]
		mapped, ok := mapGcalEvent(e)
		if !ok {
			continue
		}
		day, slotLine, ok2 := busySlotEntryForPlanner(mapped, loc)
		if !ok2 || day == "" || slotLine == "" {
			continue
		}
		byDate[day] = append(byDate[day], slotLine)
	}
	if len(byDate) == 0 {
		return ""
	}
	var keys []string
	for k := range byDate {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("Занятые слоты (не планируй на эти времена):\n")
	for _, ds := range keys {
		b.WriteString(ds)
		b.WriteString(": ")
		b.WriteString(strings.Join(byDate[ds], "; "))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}

// busySlotEntryForPlanner — дата и строка интервала в Europe/Moscow (как в остальном приложении),
// чтобы ИИ видел те же часы, что и пользователь в календаре.
func busySlotEntryForPlanner(m map[string]interface{}, loc *time.Location) (dayISO string, line string, ok bool) {
	summary, _ := m["title"].(string)
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = "Без названия"
	}
	ad, _ := m["all_day"].(bool)
	st, stOk := m["start"].(string)
	if !stOk || strings.TrimSpace(st) == "" {
		return "", "", false
	}
	if ad {
		// Всесуточные события (дни рождения, праздники) не блокируют рабочий день —
		// пропускаем, чтобы ИИ не считал весь день занятым.
		return "", "", false
	}
	en, _ := m["end"].(string)
	tStart, err := time.Parse(time.RFC3339, st)
	if err != nil {
		return "", "", false
	}
	var tEnd time.Time
	if strings.TrimSpace(en) != "" {
		tEnd, err = time.Parse(time.RFC3339, en)
		if err != nil {
			tEnd = tStart.Add(time.Hour)
		}
	} else {
		tEnd = tStart.Add(time.Hour)
	}
	ls := tStart.In(loc)
	le := tEnd.In(loc)
	dayISO = ls.Format("2006-01-02")
	hm0 := ls.Format("15:04")
	hm1 := le.Format("15:04")
	ly, lm, ld := le.Date()
	sy, sm, sd := ls.Date()
	if ly != sy || lm != sm || ld != sd {
		hm1 = "23:59"
	}
	return dayISO, hm0 + "-" + hm1 + " " + summary, true
}

func isBreakTask(t *aiTaskRaw) bool {
	if t == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(t.Kind), "break")
}

// capBreakTasksPerDay оставляет не больше maxPerDay перерывов на каждую дату (лишние отбрасываются по порядку).
func capBreakTasksPerDay(tasks []aiTaskRaw, maxPerDay int) []aiTaskRaw {
	if maxPerDay < 0 || len(tasks) == 0 {
		return tasks
	}
	perDay := map[string]int{}
	out := make([]aiTaskRaw, 0, len(tasks))
	for _, t := range tasks {
		if isBreakTask(&t) {
			d := strings.TrimSpace(t.Date)
			if perDay[d] >= maxPerDay {
				continue
			}
			perDay[d]++
		}
		out = append(out, t)
	}
	return out
}

func smartTaskNonBreakCount(tasks []aiTaskRaw) int {
	n := 0
	for i := range tasks {
		if !isBreakTask(&tasks[i]) {
			n++
		}
	}
	return n
}

func sortSmartTasksByStart(tasks []aiTaskRaw) {
	sort.Slice(tasks, func(i, j int) bool {
		di := strings.TrimSpace(tasks[i].Date)
		dj := strings.TrimSpace(tasks[j].Date)
		if di != dj {
			return di < dj
		}
		hi, mi := parseHM(tasks[i].Time)
		hj, mj := parseHM(tasks[j].Time)
		return hi*60+mi < hj*60+mj
	})
}

// capSmartTasksRespectingQuota оставляет все перерывы (kind: break), а лимит Free применяет только к остальным объектам; при обрезке сортирует по дате и времени начала.
func capSmartTasksRespectingQuota(tasks []aiTaskRaw, maxNonBreak int) ([]aiTaskRaw, bool) {
	if maxNonBreak < 0 {
		return tasks, false
	}
	if maxNonBreak == 0 {
		return nil, smartTaskNonBreakCount(tasks) > 0
	}
	var breaks []aiTaskRaw
	var core []aiTaskRaw
	for _, t := range tasks {
		if isBreakTask(&t) {
			breaks = append(breaks, t)
		} else {
			core = append(core, t)
		}
	}
	truncated := false
	if len(core) > maxNonBreak {
		core = core[:maxNonBreak]
		truncated = true
	}
	out := append(append([]aiTaskRaw{}, core...), breaks...)
	sortSmartTasksByStart(out)
	return out, truncated
}

func normalizeTask(one aiTaskRaw, userText, todayStr string, idx int) aiTaskRaw {
	t := one
	t.Kind = strings.TrimSpace(strings.ToLower(one.Kind))
	if t.Kind != "break" {
		t.Kind = "task"
	}
	if strings.TrimSpace(t.Title) == "" && !isBreakTask(&t) {
		parts := strings.Split(userText, ",")
		if idx < len(parts) {
			t.Title = strings.TrimSpace(parts[idx])
		}
		if t.Title == "" {
			t.Title = "Задача"
		}
	}
	dateRe := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	dl := strings.ToLower(strings.TrimSpace(t.Date))
	if dateRe.MatchString(strings.TrimSpace(t.Date)) {
		ds := strings.TrimSpace(t.Date)
		if ds < todayStr {
			t.Date = todayStr
		} else {
			t.Date = ds
		}
	} else if strings.Contains(dl, "завтра") || dl == "tomorrow" {
		d, _ := time.Parse("2006-01-02", todayStr)
		t.Date = d.AddDate(0, 0, 1).Format("2006-01-02")
	} else {
		t.Date = todayStr
	}
	hmRe := regexp.MustCompile(`^\d{1,2}:\d{2}$`)
	if !hmRe.MatchString(strings.TrimSpace(t.Time)) {
		t.Time = "09:00"
	}
	t.Time = normalizeHM(t.Time)
	// Если задача приходится на сегодня, а время уже прошло — сдвигаем на сейчас+15 мин.
	if t.Date == todayStr {
		nowMoscow := time.Now().In(func() *time.Location {
			l, err := time.LoadLocation("Europe/Moscow")
			if err != nil {
				return time.UTC
			}
			return l
		}())
		nowMin := nowMoscow.Hour()*60 + nowMoscow.Minute()
		th, tm := parseHM(t.Time)
		if th*60+tm <= nowMin {
			bump := nowMin + 15
			if bump >= 24*60 {
				bump = 23*60 + 45
			}
			t.Time = fmt.Sprintf("%02d:%02d", bump/60, bump%60)
		}
	}
	if hmRe.MatchString(strings.TrimSpace(t.EndTime)) {
		t.EndTime = normalizeHM(t.EndTime)
		sh, sm := parseHM(t.Time)
		eh, em := parseHM(t.EndTime)
		if eh*60+em <= sh*60+sm {
			t.EndTime = ""
		}
	} else {
		t.EndTime = ""
	}
	if t.DurationMinutes <= 0 {
		if isBreakTask(&t) {
			t.DurationMinutes = 10
		} else {
			t.DurationMinutes = 60
		}
	} else if isBreakTask(&t) {
		if t.DurationMinutes < 5 {
			t.DurationMinutes = 5
		} else if t.DurationMinutes > 20 {
			t.DurationMinutes = 20
		}
	} else if t.DurationMinutes < 15 {
		t.DurationMinutes = 15
	}
	titleRunes := []rune(strings.TrimSpace(t.Title))
	if len(titleRunes) > 60 {
		t.Title = string(titleRunes[:60])
	}
	if isBreakTask(&t) {
		if strings.TrimSpace(t.Title) == "" || t.Title == "Задача" {
			t.Title = "Перерыв"
		}
		t.Subtasks = nil
		if t.Difficulty < 1 {
			t.Difficulty = 1
		} else if t.Difficulty > 3 {
			t.Difficulty = 1
		}
		if strings.TrimSpace(t.Context) == "" {
			t.Context = "health"
		}
		if strings.TrimSpace(t.Methodology) == "" {
			t.Methodology = "Тайм-блокинг"
		}
		if strings.TrimSpace(t.EnergyLevel) == "" {
			t.EnergyLevel = "low"
		}
		if strings.TrimSpace(t.Priority) == "" {
			t.Priority = "low"
		}
	}
	return t
}

func normalizeHM(s string) string {
	s = strings.TrimSpace(s)
	p := strings.Split(s, ":")
	if len(p) != 2 {
		return "09:00"
	}
	h, err1 := strconv.Atoi(strings.TrimSpace(p[0]))
	mi, err2 := strconv.Atoi(strings.TrimSpace(p[1]))
	if err1 != nil || err2 != nil || h < 0 || h > 23 || mi < 0 || mi > 59 {
		return "09:00"
	}
	return fmt.Sprintf("%02d:%02d", h, mi)
}

func parseHM(s string) (h, m int) {
	p := strings.Split(strings.TrimSpace(s), ":")
	if len(p) != 2 {
		return 9, 0
	}
	h, _ = strconv.Atoi(p[0])
	m, _ = strconv.Atoi(p[1])
	return h, m
}

func parseAIJSONTasks(content string) ([]aiTaskRaw, error) {
	s := normalizeAIJSONInput(content)
	var arr []aiTaskRaw
	if err := json.Unmarshal([]byte(s), &arr); err == nil && len(arr) > 0 {
		return arr, nil
	}
	var one aiTaskRaw
	if err := json.Unmarshal([]byte(s), &one); err == nil {
		if one.Title != "" || one.Date != "" {
			return []aiTaskRaw{one}, nil
		}
	}
	if m, ok := extractJSONArray(s); ok {
		if err := json.Unmarshal([]byte(m), &arr); err == nil && len(arr) > 0 {
			return arr, nil
		}
	}
	if m, ok := extractJSONObject(s); ok {
		if err := json.Unmarshal([]byte(m), &one); err == nil {
			if one.Title != "" || one.Date != "" {
				return []aiTaskRaw{one}, nil
			}
		}
	}
	// Резерв: жадный поиск (как раньше), если скобки сбалансированы иначе
	if m := regexp.MustCompile(`(?s)\[[\s\S]*\]`).FindString(s); m != "" {
		if err := json.Unmarshal([]byte(m), &arr); err == nil && len(arr) > 0 {
			return arr, nil
		}
	}
	if m := regexp.MustCompile(`(?s)\{[\s\S]*\}`).FindString(s); m != "" {
		if err := json.Unmarshal([]byte(m), &one); err == nil {
			if one.Title != "" || one.Date != "" {
				return []aiTaskRaw{one}, nil
			}
		}
	}
	for _, chunk := range extractMarkdownJSONChunks(content) {
		t := normalizeAIJSONInput(chunk)
		if err := json.Unmarshal([]byte(t), &arr); err == nil && len(arr) > 0 {
			return arr, nil
		}
		if err := json.Unmarshal([]byte(t), &one); err == nil && (one.Title != "" || one.Date != "") {
			return []aiTaskRaw{one}, nil
		}
		if m, ok := extractJSONArray(t); ok {
			if err := json.Unmarshal([]byte(m), &arr); err == nil && len(arr) > 0 {
				return arr, nil
			}
		}
	}
	return nil, errors.New("не удалось распознать JSON")
}

// normalizeAIJSONInput убирает BOM, лишние кавычки вокруг всего ответа и обрамление ```…``` в начале/конце.
func normalizeAIJSONInput(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 3 && s[0] == 0xef && s[1] == 0xbb && s[2] == 0xbf {
		s = strings.TrimSpace(s[3:])
	}
	s = stripMarkdownFence(s)
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if u, err := strconv.Unquote(s); err == nil {
			s = strings.TrimSpace(u)
		}
	}
	return s
}

// extractMarkdownJSONChunks вытаскивает содержимое каждого блока ```…``` (часто модель пишет текст, затем код).
func extractMarkdownJSONChunks(s string) []string {
	re := regexp.MustCompile("(?s)```(?:json|JSON)?\\s*\n?(.*?)```")
	ms := re.FindAllStringSubmatch(s, -1)
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		if len(m) > 1 && strings.TrimSpace(m[1]) != "" {
			out = append(out, strings.TrimSpace(m[1]))
		}
	}
	return out
}

// extractJSONArray — сбалансированный фрагмент массива с учётом строк в кавычках.
func extractJSONArray(s string) (string, bool) {
	i := strings.Index(s, "[")
	if i < 0 {
		return "", false
	}
	depth := 0
	inString := false
	escape := false
	for j := i; j < len(s); j++ {
		c := s[j]
		if escape {
			escape = false
			continue
		}
		if inString {
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return s[i : j+1], true
			}
		}
	}
	return "", false
}

// extractJSONObject — один верхнеуровневый объект { … }.
func extractJSONObject(s string) (string, bool) {
	i := strings.Index(s, "{")
	if i < 0 {
		return "", false
	}
	depth := 0
	inString := false
	escape := false
	for j := i; j < len(s); j++ {
		c := s[j]
		if escape {
			escape = false
			continue
		}
		if inString {
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[i : j+1], true
			}
		}
	}
	return "", false
}

func stripMarkdownFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSpace(s)
		if len(s) >= 4 && strings.EqualFold(s[:4], "json") {
			if idx := strings.Index(s, "\n"); idx >= 0 {
				s = strings.TrimSpace(s[idx+1:])
			}
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = strings.TrimSpace(s[:idx])
		}
	}
	return strings.TrimSpace(s)
}

var priorityColor = map[string]string{
	"high":   "11",
	"medium": "5",
	"low":    "9",
}

// calendarColorID: важные (high) — тон 11; быт (home / эвристика по title) при не-high — тон 1 (голубой в UI, не путать с low=9).
// Если модель не выставила context=home, подстраховываем по типичным формулировкам в title (цвет только для календаря).
func calendarColorID(t *aiTaskRaw) string {
	if isBreakTask(t) {
		return "3"
	}
	p := strings.ToLower(strings.TrimSpace(t.Priority))
	if p == "high" {
		return priorityColor["high"]
	}
	ctx := strings.ToLower(strings.TrimSpace(t.Context))
	if ctx == "home" || titleSuggestsHomeChores(t.Title) {
		return "1"
	}
	if id, ok := priorityColor[p]; ok {
		return id
	}
	return ""
}

func titleSuggestsHomeChores(title string) bool {
	s := strings.ToLower(strings.TrimSpace(title))
	if s == "" {
		return false
	}
	patterns := []string{
		"продукт", "магазин", "покупк", "по списку", "гипермаркет", "супермаркет",
		"уборк", "стирк", "готовк", "готовить", "хозяйств", "бытов",
		"коммунал", "ремонт в кварт", "вынести мусор", "заказать воду",
	}
	for _, p := range patterns {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

func createGoogleCalendarEvent(ctx context.Context, httpClient *http.Client, t *aiTaskRaw, timeZone string) (string, error) {
	summary := strings.TrimSpace(t.Title)
	if summary == "" {
		summary = "Задача"
	}
	if isBreakTask(t) {
		summary = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(summary, "☕️"), "☕"))
		summary = strings.TrimSpace(summary)
		if summary == "" {
			summary = "Перерыв"
		}
		summary = "☕ " + summary
	}
	dateStr := t.Date
	timeStr := t.Time
	if timeStr == "" {
		timeStr = "09:00"
	}
	startDT := dateStr + "T" + timeStr + ":00"
	start := map[string]string{"dateTime": startDT, "timeZone": timeZone}

	var endDT string
	if et := strings.TrimSpace(t.EndTime); et != "" {
		endDT = dateStr + "T" + et + ":00"
	} else {
		dur := t.DurationMinutes
		if dur <= 0 {
			dur = 60
		}
		sh, sm := parseHM(timeStr)
		startMin := sh*60 + sm
		endMin := startMin + dur
		if endMin >= 24*60 {
			endMin = 24*60 - 1
		}
		endDT = dateStr + "T" + fmt.Sprintf("%02d:%02d:00", endMin/60, endMin%60)
	}
	end := map[string]string{"dateTime": endDT, "timeZone": timeZone}

	// В описание календаря — только человекочитаемое для пользователя (без методологий и служебных полей планировщика).
	var descParts []string
	if strings.TrimSpace(t.Reasoning) != "" {
		descParts = append(descParts, strings.TrimSpace(t.Reasoning))
	}
	if len(t.Subtasks) > 0 {
		parts := make([]string, 0, len(t.Subtasks))
		for i, st := range t.Subtasks {
			st = strings.TrimSpace(st)
			if st == "" {
				continue
			}
			parts = append(parts, fmt.Sprintf("%d) %s", i+1, st))
		}
		if len(parts) > 0 {
			descParts = append(descParts, "Подзадачи: "+strings.Join(parts, "; "))
		}
	}
	desc := strings.Join(descParts, "\n\n")

	ev := map[string]interface{}{
		"summary": summary,
		"start":   start,
		"end":     end,
	}
	if isBreakTask(t) {
		ev["extendedProperties"] = map[string]interface{}{
			"private": map[string]string{
				"pocketsecret_kind": "break",
			},
		}
	}
	if desc != "" {
		ev["description"] = desc
	}
	if id := calendarColorID(t); id != "" {
		ev["colorId"] = id
	}

	body, err := json.Marshal(ev)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://www.googleapis.com/calendar/v3/calendars/primary/events",
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := string(rb)
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return "", fmt.Errorf("google calendar %d: %s", resp.StatusCode, msg)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rb, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}
