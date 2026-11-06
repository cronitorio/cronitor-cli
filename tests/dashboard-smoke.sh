#!/usr/bin/env bash
# Smoke test for the cronitor + crontab-dashboard pair. Builds both binaries
# and checks:
#   1. without crontab-dashboard, `cronitor dash` exits 1 with an install command
#   2. `cronitor --config X dash --port N` execs crontab-dashboard (same PID),
#      serves the UI, requires auth, and uses the config given
#   3. SIGTERM stops it cleanly
# Needs the frontend: run `npm run build` in web/ first.
#
#   tests/dashboard-smoke.sh
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d /tmp/crontab-dashboard-smoke.XXXXXX)
port=${PORT:-9311}
user=smoke
pass=smoke-password-not-real
pid=
cleanup() {
	[ -n "$pid" ] && kill "$pid" 2>/dev/null || true
	rm -rf "$work"
}
trap cleanup EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }

test -s "$repo/web/static/index.html" || fail "web/static/index.html is missing; run npm run build in web/"
mkdir -p "$work/bin" "$work/solo"
(cd "$repo" && go build -o "$work/bin/cronitor" . && go build -o "$work/bin/crontab-dashboard" ./cmd/crontab-dashboard)
cp "$work/bin/cronitor" "$work/solo/cronitor"

# 1. No dashboard installed.
if out=$(env -i HOME="$work" PATH=/usr/bin:/bin "$work/solo/cronitor" dash 2>&1); then
	fail "cronitor dash succeeded without crontab-dashboard"
fi
grep -q "releases/download/.*/crontab-dashboard_" <<<"$out" || fail "no install command in: $out"
grep -q -- "-C $work/solo" <<<"$out" || fail "install command does not target the cronitor folder: $out"
echo "ok 1 - missing dashboard prints an install command"

# 2. Launch through cronitor dash.
env -i HOME="$work" PATH=/usr/bin:/bin CRONITOR_DASH_USER=$user CRONITOR_DASH_PASS=$pass \
	"$work/bin/cronitor" --config "$work/cronitor.json" dash --port "$port" > "$work/dash.log" 2>&1 &
pid=$!
for _ in $(seq 1 40); do
	curl -s -o /dev/null "http://127.0.0.1:$port/" && break
	sleep 0.25
done
curl -s -o /dev/null "http://127.0.0.1:$port/" || { cat "$work/dash.log"; fail "dashboard did not start"; }

comm=$(ps -o comm= -p "$pid")
[[ $comm == crontab-dash* ]] || fail "PID $pid runs '$comm', want crontab-dashboard (exec)"
page=$(curl -s -u "$user:$pass" "http://127.0.0.1:$port/")
grep -q '<div id="root">' <<<"$page" || fail "UI not served: ${page:0:200}"
code=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$port/api/settings")
[ "$code" = 401 ] || fail "API without auth returned $code, want 401"
settings=$(curl -s -u "$user:$pass" "http://127.0.0.1:$port/api/settings")
grep -q "\"config_file_path\":\"$work/cronitor.json\"" <<<"$settings" || fail "--config not passed through: ${settings:0:300}"
echo "ok 2 - cronitor dash runs crontab-dashboard with its flags"

# 3. SIGTERM. The dashboard handles signals once it prints "Dashboard running".
for _ in $(seq 1 40); do
	grep -q "Dashboard running" "$work/dash.log" && break
	sleep 0.25
done
kill -TERM "$pid"
status=0
wait "$pid" || status=$?
pid=
[ "$status" = 0 ] || { cat "$work/dash.log"; fail "exit status $status after SIGTERM"; }
echo "ok 3 - SIGTERM stops the dashboard cleanly"
