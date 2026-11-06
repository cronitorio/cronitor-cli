#!/usr/bin/env bash
# End-to-end check of crontab-dashboard with real cron. Run as root on a
# disposable Linux box with cron installed: it replaces root's crontab and
# starts cron. CI runs it on the e2e label (.github/workflows/real-cron.yml).
#
#   sudo tests/real-cron-dashboard.sh
#
# Both binaries are built with the API pointed at a local mock. The dashboard
# is started through `cronitor dash`, then:
#   1. turning monitoring on through the dashboard API wraps the job, and the
#      next cron tick runs it with run + complete pings
#   2. turning monitoring off restores the line, and the next tick sends no pings
set -euo pipefail

[ "$(id -u)" = 0 ] || { echo "run as root" >&2; exit 1; }
command -v cron >/dev/null || { echo "cron is not installed" >&2; exit 1; }

repo=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d /tmp/crontab-dashboard-e2e.XXXXXX)
out=$work/out
port=${PORT:-8766}
dash_port=${DASH_PORT:-9312}
auth=e2e:e2e-password-not-real
mkdir -p "$out" "$work/bin"
echo "work dir: $work"

# 1. Build both binaries against the mock API. The frontend is not needed;
# the committed placeholder in web/static is enough for the API.
ldflags="-X github.com/cronitorio/cronitor-cli/lib.BaseURLOverride=http://127.0.0.1:$port/api"
(cd "$repo" && go build -o "$work/bin/cronitor" -ldflags "$ldflags" . \
	&& go build -o "$work/bin/crontab-dashboard" -ldflags "$ldflags" ./cmd/crontab-dashboard)

# 2. Mock API and ping receiver. Every request is logged to requests.log.
cat > "$work/mock.py" <<'PY'
import json, sys
from http.server import BaseHTTPRequestHandler, HTTPServer
log = open(sys.argv[2], "a", buffering=1)
class H(BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def reply(self, body):
        data = json.dumps(body).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)
    def do_GET(self):
        log.write("GET %s\n" % self.path)
        if self.path.startswith("/api/monitors"):
            return self.reply({"total_monitor_count": 0, "page_size": 50, "monitors": []})
        self.reply({})
    def do_PUT(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        log.write("PUT %s\n" % self.path)
        self.reply([{"key": m["key"], "attributes": {"key": m["key"], "code": "dash01"}} for m in body])
    do_POST = do_GET
HTTPServer(("127.0.0.1", int(sys.argv[1])), H).serve_forever()
PY
python3 "$work/mock.py" "$port" "$work/requests.log" &
mock=$!
dash=
trap 'kill $mock $dash 2>/dev/null; crontab -r 2>/dev/null || true' EXIT
sleep 1

# 3. Root crontab with one unmonitored job.
printf '%s\n' \
	"CRONITOR_PING_API_HOST=http://127.0.0.1:$port" \
	"PATH=$work/bin:/usr/bin:/bin" \
	"* * * * * echo tick >> $out/job.log" > "$work/crontab"
crontab "$work/crontab"
pgrep -x cron >/dev/null || cron
echo '{"CRONITOR_API_KEY":"real-cron-test-key-0000"}' > "$work/cronitor.json"

# 4. Start the dashboard the way existing services do.
CRONITOR_DASH_USER=${auth%%:*} CRONITOR_DASH_PASS=${auth#*:} \
	"$work/bin/cronitor" --config "$work/cronitor.json" dash --port "$dash_port" > "$work/dash.log" 2>&1 &
dash=$!
for _ in $(seq 1 40); do
	curl -s -o /dev/null "http://127.0.0.1:$dash_port/" && break
	sleep 0.25
done
curl -s -o /dev/null "http://127.0.0.1:$dash_port/" || { cat "$work/dash.log"; echo "dashboard did not start" >&2; exit 1; }

api() { curl -fsS -u "$auth" -H 'Content-Type: application/json' "$@"; }
# set_monitored true|false: PUT the job back with monitoring changed, as the UI
# does. Each GET returns a one-time CSRF token that the PUT must send.
set_monitored() {
	api -D "$work/headers" "http://127.0.0.1:$dash_port/api/jobs" > "$work/jobs.json"
	local token
	token=$(tr -d '\r' < "$work/headers" | sed -n 's/^[Xx]-[Cc][Ss][Rr][Ff]-[Tt]oken: //p')
	python3 -I -c '
import json, sys
jobs, marker, on = json.load(open(sys.argv[1])), sys.argv[2], sys.argv[3] == "true"
job = next(j for j in jobs if marker in j["command"])
job["monitored"] = on
print(json.dumps(job))
' "$work/jobs.json" "$out/job.log" "$1" > "$work/job.json"
	api -X PUT -H "X-CSRF-Token: $token" --data @"$work/job.json" "http://127.0.0.1:$dash_port/api/jobs" > /dev/null
}

count() { if [ -f "$1" ]; then grep -cF -- "${2:-}" "$1" || true; else echo 0; fi; }
wait_tick() {
	local now
	now=$(date +%s)
	sleep $(( 60 - now % 60 + 10 ))
}
check() {
	if [ "$2" = "$3" ]; then echo "ok   $1 ($2)"; else echo "FAIL $1: got $2, want $3"; failed=1; fi
}
failed=0

# Tick 1: monitoring on.
set_monitored true
echo "--- crontab after monitoring on"
crontab -l
check "job wrapped with cronitor exec" "$(crontab -l | grep -c 'cronitor exec dash01 echo tick' || true)" 1
: > "$work/requests.log"
wait_tick
echo "--- tick 1: monitored"
check "job ran" "$(count "$out/job.log" tick)" 1
check "run ping" "$(count "$work/requests.log" '/dash01/run?')" 1
check "complete ping" "$(count "$work/requests.log" '/dash01/complete?')" 1

# Tick 2: monitoring off.
set_monitored false
echo "--- crontab after monitoring off"
crontab -l
check "job unwrapped" "$(crontab -l | grep -cxF "* * * * * echo tick >> $out/job.log" || true)" 1
: > "$work/requests.log"
wait_tick
echo "--- tick 2: unmonitored"
check "job ran" "$(count "$out/job.log" tick)" 2
check "no pings" "$(count "$work/requests.log" '/dash01/')" 0

echo
if [ "$failed" = 0 ]; then
	echo "ALL CHECKS PASSED"
else
	echo "SOME CHECKS FAILED (logs in $work)"
	cat "$work/dash.log"
	exit 1
fi
