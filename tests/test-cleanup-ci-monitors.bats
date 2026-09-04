#!/usr/bin/env bats

# Tests for tests/cleanup-ci-monitors.sh using a stub cronitor binary.
# The stub records every invocation and never touches the network.

setup() {
  SCRIPT_DIR="$(dirname $BATS_TEST_FILENAME)"
  cd $SCRIPT_DIR

  STUB_DIR="$BATS_TEST_TMPDIR/stub"
  mkdir -p "$STUB_DIR"
  export STUB_CALLS="$STUB_DIR/calls.log"
  export STUB_LIST_RESPONSES="$STUB_DIR/list-responses"
  mkdir -p "$STUB_LIST_RESPONSES"

  # The stub returns list-responses/1.json on the first list call,
  # list-responses/2.json on the second, and so on. When no file
  # exists, it returns an empty monitor list.
  cat > "$STUB_DIR/cronitor" <<'STUB'
#!/usr/bin/env bash
echo "$*" >> "$STUB_CALLS"
if [ "$1 $2" = "monitor list" ]; then
  n=$(grep -c "^monitor list" "$STUB_CALLS")
  f="$STUB_LIST_RESPONSES/$n.json"
  if [ -f "$f" ]; then cat "$f"; else echo '{"monitors": [], "page_info": {"page": 1, "pageSize": 100, "totalMonitorCount": 0}}'; fi
fi
exit 0
STUB
  chmod +x "$STUB_DIR/cronitor"
  export CRONITOR_BIN="$STUB_DIR/cronitor"
}

@test "Cleanup deletes every monitor whose name carries the runner hostname" {
  cat > "$STUB_LIST_RESPONSES/1.json" <<'JSON'
{"monitors": [
  {"key": "aaa111", "name": "[runnervmtest1] echo six dig parse", "type": "job"},
  {"key": "bbb222", "name": "runnervmtest1/GoogleUpdaterTaskSystem", "type": "job"},
  {"key": "ccc333", "name": "unrelated monitor mentioning runnervmtest1", "type": "job"}
], "page_info": {"page": 1, "pageSize": 100, "totalMonitorCount": 3}}
JSON

  run ./cleanup-ci-monitors.sh runnervmtest1
  [ "$status" -eq 0 ]
  grep -q "^monitor list .*--search runnervmtest1" "$STUB_CALLS"
  grep -qE "^monitor delete( -k [^ ]+)? aaa111 bbb222$" "$STUB_CALLS"
  ! grep -q "ccc333" "$STUB_CALLS"
}

@test "Cleanup keeps listing until no matching monitors remain" {
  cat > "$STUB_LIST_RESPONSES/1.json" <<'JSON'
{"monitors": [{"key": "aaa111", "name": "[runnervmtest1] true", "type": "job"}], "page_info": {"page": 1, "pageSize": 100, "totalMonitorCount": 1}}
JSON
  cat > "$STUB_LIST_RESPONSES/2.json" <<'JSON'
{"monitors": [{"key": "ddd444", "name": "[runnervmtest1] false", "type": "job"}], "page_info": {"page": 1, "pageSize": 100, "totalMonitorCount": 1}}
JSON

  run ./cleanup-ci-monitors.sh runnervmtest1
  [ "$status" -eq 0 ]
  grep -qE "^monitor delete( -k [^ ]+)? aaa111$" "$STUB_CALLS"
  grep -qE "^monitor delete( -k [^ ]+)? ddd444$" "$STUB_CALLS"
  [ "$(grep -c '^monitor list' "$STUB_CALLS")" -eq 3 ]
}

@test "Cleanup does not call delete when nothing matches" {
  run ./cleanup-ci-monitors.sh runnervmtest1
  [ "$status" -eq 0 ]
  ! grep -q "^monitor delete" "$STUB_CALLS"
}

@test "Cleanup passes the API key from the environment" {
  CRONITOR_API_KEY=secret123 run ./cleanup-ci-monitors.sh runnervmtest1
  [ "$status" -eq 0 ]
  grep -q "^monitor list .*-k secret123" "$STUB_CALLS"
}

@test "Cleanup strips carriage returns that jq emits on Windows" {
  cat > "$STUB_LIST_RESPONSES/1.json" <<'JSON'
{"monitors": [
  {"key": "aaa111", "name": "[runnervmtest1] true", "type": "job"},
  {"key": "bbb222", "name": "[runnervmtest1] false", "type": "job"}
], "page_info": {"page": 1, "pageSize": 100, "totalMonitorCount": 2}}
JSON

  # Wrap the real jq so every output line ends in CRLF, as on MSYS2.
  REAL_JQ="$(command -v jq)"
  cat > "$STUB_DIR/jq" <<WRAP
#!/usr/bin/env bash
"$REAL_JQ" "\$@" | sed 's/\$/\r/'
WRAP
  chmod +x "$STUB_DIR/jq"

  PATH="$STUB_DIR:$PATH" run ./cleanup-ci-monitors.sh runnervmtest1
  [ "$status" -eq 0 ]
  grep -qE "^monitor delete( -k [^ ]+)? aaa111 bbb222$" "$STUB_CALLS"
  ! grep -q $'\r' "$STUB_CALLS"
}
