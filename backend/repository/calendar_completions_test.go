package repository

import (
	"testing"
	"time"

	"pocketsecret/config"
)

// dayAgo возвращает дату offset дней назад (по календарю приложения) как UTC-полночь —
// в том виде, в каком строки приходят из запроса DATE(... AT TIME ZONE ...).
func dayAgo(offset int) time.Time {
	d := time.Now().In(config.AppLocation()).AddDate(0, 0, -offset)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
}

func TestCalcCompletionStreak(t *testing.T) {
	cases := []struct {
		name string
		days []time.Time // по убыванию, как из ORDER BY 1 DESC
		want int
	}{
		{"пусто", nil, 0},
		{"только сегодня", []time.Time{dayAgo(0)}, 1},
		{"сегодня и вчера", []time.Time{dayAgo(0), dayAgo(1)}, 2},
		{"три дня подряд", []time.Time{dayAgo(0), dayAgo(1), dayAgo(2)}, 3},
		{"вчера без сегодня — стрик жив", []time.Time{dayAgo(1), dayAgo(2)}, 2},
		{"разрыв: сегодня и три дня назад", []time.Time{dayAgo(0), dayAgo(3)}, 1},
		{"последнее выполнение позавчера — стрик сгорел", []time.Time{dayAgo(2), dayAgo(3)}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := calcCompletionStreak(c.days, config.AppLocation()); got != c.want {
				t.Errorf("ожидалось %d, получено %d", c.want, got)
			}
		})
	}
}
