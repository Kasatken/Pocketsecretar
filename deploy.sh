#!/usr/bin/env bash
# Деплой на сервер: Go + React из корня проекта (/opt/pocketsecret и т.п.)
#   bash deploy.sh        — всё
#   bash deploy.sh go    — только API (build + install + systemctl)
#   bash deploy.sh web   — только фронт (npm build + /var/www/html)
#
# Нужно на сервере: Go ≥ 1.22, Node.js + npm (для фронта).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Не подтягивать автоматически другой toolchain Go (иначе возможны сюрпризы на VDS).
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"

need_cmds() {
	local missing=()
	for c in "$@"; do
		command -v "$c" >/dev/null 2>&1 || missing+=("$c")
	done
	if [[ ${#missing[@]} -gt 0 ]]; then
		echo "Не найдены команды: ${missing[*]}" >&2
		echo "Установите их на сервере (например: apt install golang-go | snap install go — версия ≥ 1.22; apt install npm)." >&2
		exit 1
	fi
}

go_preflight() {
	need_cmds go
	local ver
	ver="$(go version | awk '{print $3}' | sed 's/^go//')"
	# Сравнение MAJOR.MINOR (bash): отсечь патч
	local major="${ver%%.*}"
	local rest="${ver#*.}"
	local minor="${rest%%.*}"
	if [[ -z "$minor" ]]; then minor=0; fi
	if [[ "$major" -lt 1 ]] || { [[ "$major" -eq 1 ]] && [[ "$minor" -lt 22 ]]; }; then
		echo "Нужен Go ≥ 1.22, сейчас: $(go version)" >&2
		exit 1
	fi
}

deploy_go() {
	go_preflight
	echo "==> Go backend (go mod tidy + build)"
	mkdir -p "$ROOT/dist"
	(
		cd "$ROOT/backend"
		# Устаревшие файлы с сервера/старых веток: дублируют collab_spaces.go или ломают сборку.
		for f in \
			repository/spaces.go \
			handlers/spaces_collab.go \
			handlers/spaces_availability.go \
			handlers/spaces_ws.go \
		; do
			if [[ -f "$f" ]]; then
				echo "==> Удаляю устаревший backend/$f (актуальный код — collab_spaces.go + main.go)" >&2
				rm -f "$f"
			fi
		done
		go mod tidy
		go build -ldflags="-s -w" -o "$ROOT/dist/server" .
	)

	echo "==> Установка бинаря"
	sudo install -d -m755 /opt/pocketsecret
	sudo install -m755 "$ROOT/dist/server" /opt/pocketsecret/server

	echo "==> Установка systemd unit"
	sudo install -m644 "$ROOT/deploy/pocketsecret.service" /etc/systemd/system/pocketsecret.service
	sudo systemctl daemon-reload
	sudo systemctl enable pocketsecret

	echo "==> Env для systemd (/etc/pocketsecret/pocketsecret.env)"
	sudo install -d -m755 /etc/pocketsecret
	if [[ ! -f "$ROOT/deploy/pocketsecret.env" ]]; then
		echo "Нет $ROOT/deploy/pocketsecret.env — сохраните и загрузите на сервер deploy/pocketsecret.env." >&2
		exit 1
	fi
	sudo install -m600 "$ROOT/deploy/pocketsecret.env" /etc/pocketsecret/pocketsecret.env

	echo "==> Перезапуск сервиса"
	sudo systemctl restart pocketsecret
	sleep 1
	if curl -sf "http://127.0.0.1:8080/api/health" >/dev/null; then
		echo "Проверка: GET /api/health — OK"
	else
		echo "Предупреждение: /api/health не ответил (проверьте PORT в env и journalctl -u pocketsecret)." >&2
	fi
	sudo systemctl status pocketsecret --no-pager -l || true

	echo "==> Готово API: curl -s https://pocketsecretary.ru/api/health"
}

deploy_web() {
	need_cmds node npm
	echo "==> React (Vite)"
	cd "$ROOT/frontend"
	npm install --silent
	npm run build

	echo "==> Публикация в /var/www/html"
	sudo cp -a dist/. /var/www/html/

	echo "==> Готово сайт: https://pocketsecretary.ru"
}

case "${1:-all}" in
	all | "")
		deploy_go
		echo ""
		deploy_web
		echo ""
		echo "==> Всё задеплоено!"
		echo "    Сайт:  https://pocketsecretary.ru"
		echo "    API:   https://pocketsecretary.ru/api/health"
		;;
	go)
		deploy_go
		;;
	web)
		deploy_web
		;;
	*)
		echo "Использование: bash deploy.sh [all|go|web]" >&2
		exit 1
		;;
esac
