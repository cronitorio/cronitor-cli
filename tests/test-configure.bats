#!/usr/bin/env bats

load setup_suite

setup() {
  SCRIPT_DIR="$(dirname $BATS_TEST_FILENAME)"
  cd $SCRIPT_DIR
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
  LOG_FILE="/tmp/cronitor-test.log"
  CRONITOR_LOG=$LOG_FILE ../cronitor $CRONITOR_ARGS ping d3x0c1 --run --api-key "$CRONITOR_API_KEY"
  grep "d3x0c1" $LOG_FILE | grep -q '/run'
}

@test "Configure uses hostname from env var" {
  CRONITOR_HOSTNAME=myHost ../cronitor $CRONITOR_ARGS ping d3x0c1 --run --log $CLI_LOGFILE
  grep -q "&host=myHost" $CLI_LOGFILE
}

@test "Configure uses hostname from arg not env var" {
  CRONITOR_HOSTNAME=myHost ../cronitor $CRONITOR_ARGS ping d3x0c1 --run --log $CLI_LOGFILE --hostname otherHost
  grep -q "&host=otherHost" $CLI_LOGFILE
}

@test "Configure uses ping api key from env var" {
  CRONITOR_PING_API_KEY=123 ../cronitor $CRONITOR_ARGS ping d3x0c1 --run --log $CLI_LOGFILE --api-key "$CRONITOR_API_KEY"
  grep -q "ping/123" $CLI_LOGFILE
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
