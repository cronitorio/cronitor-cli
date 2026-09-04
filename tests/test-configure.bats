#!/usr/bin/env bats

load setup_suite

setup() {
  SCRIPT_DIR="$(dirname $BATS_TEST_FILENAME)"
  cd $SCRIPT_DIR
  load test_helper
  export BATS_TMPDIR="/tmp/cronitor-test"
  mkdir -p $BATS_TMPDIR
  export CLI_CONFIGFILE_ALTERNATE="$BATS_TMPDIR/test-build-config.json"
  MSG=$(date)
}

teardown() {
  rm -f $CLI_LOGFILE
  rm -f $CLI_CONFIGFILE
  rm -f $CLI_LOGFILE_ALTERNATE
  rm -f $CLI_CONFIGFILE_ALTERNATE
  rm -rf $BATS_TMPDIR
}

#################
# CONFIGURE TESTS
#################

@test "Configure uses log file from env var" {
  LOG_FILE="$BATS_TMPDIR/cronitor-test.log"
  rm -f $LOG_FILE
  CRONITOR_LOG=$LOG_FILE ../cronitor $CRONITOR_ARGS ping d3x0c1 --run --api-key "$CRONITOR_API_KEY"
  # Check that the log file was created and contains the ping
  [ -f "$LOG_FILE" ] && grep -q "d3x0c1" $LOG_FILE
}

@test "Configure uses hostname from env var" {
  rm -f $CLI_LOGFILE
  CRONITOR_HOSTNAME=myHost ../cronitor $CRONITOR_ARGS ping d3x0c1 --run --log $CLI_LOGFILE
  [ -f "$CLI_LOGFILE" ] && grep -q "&host=myHost" $CLI_LOGFILE
}

@test "Configure uses hostname from arg not env var" {
  rm -f $CLI_LOGFILE
  CRONITOR_HOSTNAME=myHost ../cronitor $CRONITOR_ARGS ping d3x0c1 --run --log $CLI_LOGFILE --hostname otherHost
  [ -f "$CLI_LOGFILE" ] && grep -q "&host=otherHost" $CLI_LOGFILE
}

@test "Configure uses ping api key from env var" {
  rm -f $CLI_LOGFILE
  CRONITOR_PING_API_KEY=123 ../cronitor $CRONITOR_ARGS ping d3x0c1 --run --log $CLI_LOGFILE --api-key "$CRONITOR_API_KEY"
  [ -f "$CLI_LOGFILE" ] && grep -q "ping/123" $CLI_LOGFILE
}

@test "Configure writes hostname correctly to config file" {
  CRONITOR_CONFIG=$CLI_CONFIGFILE_ALTERNATE ../cronitor $CRONITOR_ARGS configure --hostname "$MSG" >/dev/null
  grep "CRONITOR_HOSTNAME" $CLI_CONFIGFILE_ALTERNATE | grep -q "$MSG"
}

@test "Configure writes API Key correctly to config file" {
  CRONITOR_CONFIG=$CLI_CONFIGFILE_ALTERNATE ../cronitor $CRONITOR_ARGS configure --api-key "$MSG"  >/dev/null
  grep "CRONITOR_API_KEY" $CLI_CONFIGFILE_ALTERNATE | grep -q "$MSG"
}

@test "Configure writes API Key correctly to new config file" {
  CRONITOR_CONFIG=$CLI_CONFIGFILE_ALTERNATE ../cronitor $CRONITOR_ARGS configure --api-key "$CLI_ACTUAL_API_KEY" >/dev/null  # Using actual API key here so it will be avail for later integration tests..
  grep "CRONITOR_API_KEY" $CLI_CONFIGFILE_ALTERNATE | grep -q "$CLI_ACTUAL_API_KEY"
}

@test "Configure writes API Key correctly to custom config file set by param" {
  ../cronitor $CRONITOR_ARGS configure --config $CLI_CONFIGFILE_ALTERNATE --api-key "$MSG"  2>/dev/null
  grep "CRONITOR_API_KEY" $CLI_CONFIGFILE_ALTERNATE | grep -q "$MSG"
}

@test "Configure writes API Key correctly to custom config file set by env var" {
  export MSG=$(date)
  echo "Using MSG: $MSG"
  CRONITOR_CONFIG=$CLI_CONFIGFILE_ALTERNATE ../cronitor $CRONITOR_ARGS configure --api-key "${MSG}" 2>/dev/null
  grep "CRONITOR_API_KEY" $CLI_CONFIGFILE_ALTERNATE | grep -q "${MSG}"
}

@test "Configure writes Ping API Key correctly to config file" {
  skip "Configure command does not save global ping-api-key flag to config file"
}

@test "Configure writes log path correctly to config file" {
  skip "Configure command does not save global log flag to config file"
}

@test "Configure writes exclude text correctly to config file" {
  CRONITOR_CONFIG=$CLI_CONFIGFILE_ALTERNATE ../cronitor $CRONITOR_ARGS configure --exclude-from-name "$MSG"  2>/dev/null
  grep -q "CRONITOR_EXCLUDE_TEXT" $CLI_CONFIGFILE_ALTERNATE && grep -q "$MSG" $CLI_CONFIGFILE_ALTERNATE
}

@test "Configure writes multiple exclude text entries correctly to config file" {
  CRONITOR_CONFIG=$CLI_CONFIGFILE_ALTERNATE ../cronitor $CRONITOR_ARGS configure --exclude-from-name "${MSG}A" --exclude-from-name "${MSG}B"  2>/dev/null
  grep -q "CRONITOR_EXCLUDE_TEXT" $CLI_CONFIGFILE_ALTERNATE && grep -q "${MSG}A" $CLI_CONFIGFILE_ALTERNATE && grep -q "${MSG}B" $CLI_CONFIGFILE_ALTERNATE
}

@test "Configure does not print API or ping keys" {
  FAKE_API="test-api-key-not-real"
  FAKE_PING="test-ping-key-not-real"
  output=$(CRONITOR_CONFIG=$CLI_CONFIGFILE_ALTERNATE ../cronitor $CRONITOR_ARGS configure --api-key "$FAKE_API" --ping-api-key "$FAKE_PING" 2>&1)
  ! echo "$output" | grep -q "$FAKE_API"
  ! echo "$output" | grep -q "$FAKE_PING"
  echo "$output" | grep -q "API Key"
  echo "$output" | grep -q "Set"
}

@test "Configure does not print dashboard password" {
  FAKE_PASS="test-dash-password-not-real"
  output=$(CRONITOR_CONFIG=$CLI_CONFIGFILE_ALTERNATE ../cronitor $CRONITOR_ARGS configure --dash-username bats-user --dash-password "$FAKE_PASS" 2>&1)
  ! echo "$output" | grep -q "$FAKE_PASS"
  echo "$output" | grep -q "********"
}

@test "Configure verbose does not dump secret env values or unrelated names" {
  FAKE_API="test-api-key-not-real"
  FAKE_AWS="fake-aws-secret-value-not-real"
  output=$(AWS_SECRET_ACCESS_KEY="$FAKE_AWS" CRONITOR_API_KEY="$FAKE_API" CRONITOR_CONFIG=$CLI_CONFIGFILE_ALTERNATE ../cronitor $CRONITOR_ARGS configure --verbose 2>&1)
  ! echo "$output" | grep -q "$FAKE_API"
  ! echo "$output" | grep -q "$FAKE_AWS"
  ! echo "$output" | grep -q "AWS_SECRET_ACCESS_KEY"
  echo "$output" | grep -q "CRONITOR_API_KEY: Set"
}

@test "Configure new credential file is mode 0600" {
  skip_if_windows
  CRONITOR_CONFIG=$CLI_CONFIGFILE_ALTERNATE ../cronitor $CRONITOR_ARGS configure --hostname bats-host >/dev/null
  perms=$(stat -c '%a' "$CLI_CONFIGFILE_ALTERNATE")
  [ "$perms" = "600" ]
}

@test "Configure keeps an existing 0644 config at 0644" {
  skip_if_windows
  printf '%s\n' '{"CRONITOR_HOSTNAME":"keep-me"}' > "$CLI_CONFIGFILE_ALTERNATE"
  chmod 644 "$CLI_CONFIGFILE_ALTERNATE"
  run env CRONITOR_CONFIG=$CLI_CONFIGFILE_ALTERNATE ../cronitor $CRONITOR_ARGS configure --api-key "test-api-key-not-real"
  [ "$status" -eq 0 ]
  ! echo "$output" | grep -q "test-api-key-not-real"
  perms=$(stat -c '%a' "$CLI_CONFIGFILE_ALTERNATE")
  [ "$perms" = "644" ]
  grep "CRONITOR_HOSTNAME" "$CLI_CONFIGFILE_ALTERNATE" | grep -q "keep-me"
  grep "CRONITOR_API_KEY" "$CLI_CONFIGFILE_ALTERNATE" | grep -q "test-api-key-not-real"
}

@test "Configure keeps an existing 0640 config at 0640" {
  skip_if_windows
  CRONITOR_CONFIG=$CLI_CONFIGFILE_ALTERNATE ../cronitor $CRONITOR_ARGS configure --hostname "$MSG" >/dev/null
  chmod 640 "$CLI_CONFIGFILE_ALTERNATE"
  run env CRONITOR_CONFIG=$CLI_CONFIGFILE_ALTERNATE ../cronitor $CRONITOR_ARGS configure --api-key "test-api-key-not-real"
  [ "$status" -eq 0 ]
  perms=$(stat -c '%a' "$CLI_CONFIGFILE_ALTERNATE")
  [ "$perms" = "640" ]
  grep "CRONITOR_HOSTNAME" "$CLI_CONFIGFILE_ALTERNATE" | grep -q "$MSG"
}

@test "Configure preserves hostname when writing a new API key" {
  CRONITOR_CONFIG=$CLI_CONFIGFILE_ALTERNATE ../cronitor $CRONITOR_ARGS configure --hostname "$MSG" >/dev/null
  CRONITOR_CONFIG=$CLI_CONFIGFILE_ALTERNATE ../cronitor $CRONITOR_ARGS configure --api-key "test-api-key-not-real" >/dev/null
  grep "CRONITOR_HOSTNAME" "$CLI_CONFIGFILE_ALTERNATE" | grep -q "$MSG"
}
