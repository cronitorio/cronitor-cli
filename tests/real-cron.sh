#!/usr/bin/env bash
# Manual end-to-end check of the crontab shell shim under real cron.
# Run as root on a disposable Linux box with cron installed (it replaces
# root's crontab and starts cron). Not run in CI.
#
#   sudo tests/real-cron.sh
#
# It builds cronitor with the API pointed at a local mock, syncs a fixture
# crontab with --format=short, installs it, and checks three cron ticks:
#   1. healthy binary: every job runs once and sends run + final pings
#   2. binary missing: marked jobs still run, no pings
#   3. binary truncated: marked jobs do not run (same as cronitor exec);
#      a line without a marker still runs
set -euo pipefail

[ "$(id -u)" = 0 ] || { echo "run as root" >&2; exit 1; }
command -v cron >/dev/null || { echo "cron is not installed" >&2; exit 1; }

repo=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d /tmp/cronitor-real-cron.XXXXXX)
out=$work/out
port=${PORT:-8765}
mkdir -p "$out" "$work/bin"
echo "work dir: $work"

# 1. Build. Only the API host is changed; pings use CRONITOR_PING_API_HOST.
mkdir -p "$repo/web/static"
[ -e "$repo/web/static/index.html" ] || echo placeholder > "$repo/web/static/index.html"
(cd "$repo" && go build -o "$work/bin/cronitor" \
	-ldflags "-X github.com/cronitorio/cronitor-cli/lib.BaseURLOverride=http://127.0.0.1:$port/api" .)

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
        out = []
        for i, m in enumerate(body):
            code = m.get("code") or "new%02d" % i
            out.append({"key": m["key"], "attributes": {"key": m["key"], "code": code}})
        self.reply(out)
    do_POST = do_GET
HTTPServer(("127.0.0.1", int(sys.argv[1])), H).serve_forever()
PY
python3 "$work/mock.py" "$port" "$work/requests.log" &
mock=$!
trap 'kill $mock 2>/dev/null; crontab -r 2>/dev/null || true; rm -f /etc/cronitor/cronitor-shell' EXIT
sleep 1

# 3. Fixture crontab, then sync it with the real binary.
cat > "$work/crontab" <<EOF
CRONITOR_PING_API_HOST=http://127.0.0.1:$port
PATH=$work/bin:/usr/bin:/bin
* * * * * echo plain >> $out/plain.log
* * * * * cat >> $out/stdin.log%line-one%line-two
* * * * * echo "a  b" | tr a A >> $out/pipe.log
* * * * * echo failing >> $out/fail.log; exit 3
* * * * * cronitor exec conv01 'cd $out && echo converted >> conv.log'
* * * * * cronitor exec --no-stdout nostd01 echo no-stdout-job
* * * * * cronitor --env staging exec env01 echo env-job
* * * * * cronitor --config $work/other.json exec keep01 echo kept-exec
EOF
echo '{"CRONITOR_API_KEY":"real-cron-test-key-0000"}' > "$work/cronitor.json"
echo '{}' > "$work/other.json"
"$work/bin/cronitor" --config "$work/cronitor.json" sync --auto --silent --format=short "$work/crontab"
# A line added after sync has no marker. It must survive a broken binary.
echo "* * * * * echo raw >> $out/raw.log" >> "$work/crontab"
echo "--- synced crontab"
cat "$work/crontab"
grep -q '^SHELL=.*cronitor-shell$' "$work/crontab"
grep -q 'CRONITOR=conv01 cd ' "$work/crontab"
grep -q 'CRONITOR=nostd01 CRONITOR_NO_STDOUT=1 echo no-stdout-job' "$work/crontab"
grep -q 'CRONITOR=env01 CRONITOR_ENV=staging echo env-job' "$work/crontab"
grep -q 'cronitor --config .* exec keep01' "$work/crontab"

crontab "$work/crontab"
pgrep -x cron >/dev/null || cron

count() { if [ -f "$1" ]; then grep -cF -- "${2:-}" "$1" || true; else echo 0; fi; }
wait_tick() {
	local now=$(date +%s)
	sleep $(( 60 - now % 60 + 10 ))
}
check() {
	if [ "$2" = "$3" ]; then echo "ok   $1 ($2)"; else echo "FAIL $1: got $2, want $3"; failed=1; fi
}
failed=0

# Tick 1: healthy.
: > "$work/requests.log"
wait_tick
echo "--- tick 1: healthy binary"
check "plain ran once" "$(count $out/plain.log)" 1
check "stdin from %" "$(cat $out/stdin.log 2>/dev/null | tr '\n' ' ')" "line-one line-two "
check "pipe kept spaces" "$(cat $out/pipe.log 2>/dev/null)" "A  b"
check "converted exec ran" "$(count $out/conv.log)" 1
check "raw line ran" "$(count $out/raw.log)" 1
for path in $(grep -o 'CRONITOR=[A-Za-z0-9]*' "$work/crontab" | cut -d= -f2); do
	check "run ping $path" "$(count $work/requests.log "/$path/run?")" 1
	final=$(( $(count $work/requests.log "/$path/complete?") + $(count $work/requests.log "/$path/fail?") ))
	check "final ping $path" "$final" 1
done
check "fail ping carries exit 3" "$(count $work/requests.log 'status_code=3')" 1
check "exec line pinged" "$(count $work/requests.log '/keep01/run?')" 1
check "env setting reached pings" "$(count $work/requests.log '/env01/run?' )-$(grep '/env01/' $work/requests.log | grep -c 'env=staging' || true)" "1-2"
check "no-stdout keeps output out of the ping" "$(grep '/nostd01/complete?' $work/requests.log | grep -c 'msg=no-stdout-job' || true)" 0

# Tick 2: binary missing. Marked jobs run without monitoring.
mv "$work/bin/cronitor" "$work/bin/cronitor.real"
: > "$work/requests.log"
wait_tick
echo "--- tick 2: binary missing"
check "plain ran" "$(count $out/plain.log)" 2
check "raw line ran" "$(count $out/raw.log)" 2
check "no pings" "$(count $work/requests.log)" 0

# Tick 3: binary truncated. Marked jobs fail like cronitor exec lines.
head -c 65536 "$work/bin/cronitor.real" > "$work/bin/cronitor"
chmod 755 "$work/bin/cronitor"
wait_tick
echo "--- tick 3: binary truncated"
check "plain did not run" "$(count $out/plain.log)" 2
check "raw line ran" "$(count $out/raw.log)" 3

echo
[ "$failed" = 0 ] && echo "ALL CHECKS PASSED" || { echo "SOME CHECKS FAILED (logs in $work)"; exit 1; }
