#!/usr/bin/env bats

setup() {
  SCRIPT_DIR="$(dirname $BATS_TEST_FILENAME)"
  cd $SCRIPT_DIR

  source $SCRIPT_DIR/setup.sh
  rm -f $CLI_LOGFILE
  rm -f $CLI_CONFIGFILE
  rm -f $CLI_LOGFILE_ALTERNATE
  MSG=`date`
}

#################
# CONFIGURE TESTS
#################

@test "Configure uses log file from env var" {
  CRONITOR_LOG=$CLI_LOGFILE ../cronitor $CRONITOR_ARGS ping d3x0c1 --run
  grep -q "Sending ping ${HOSTNAME}/d3x0c1/run" $CLI_LOGFILE
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
  CRONITOR_PING_API_KEY=123 ../cronitor $CRONITOR_ARGS ping d3x0c1 --run --log $CLI_LOGFILE
  grep -q "&auth_key=123" $CLI_LOGFILE
}

@test "Configure writes hostname correctly to config file" {
  ../cronitor $CRONITOR_ARGS configure --hostname "$MSG"
  grep "CRONITOR_HOSTNAME" $CLI_CONFIGFILE | grep -q "$MSG"
}

@test "Configure writes API Key correctly to config file" {
  ../cronitor $CRONITOR_ARGS configure --api-key "$MSG"
  grep "CRONITOR_API_KEY" $CLI_CONFIGFILE | grep -q "$MSG"
}

@test "Configure writes API Key correctly to new config file" {
  ../cronitor $CRONITOR_ARGS configure --api-key "$CLI_ACTUAL_API_KEY"  # Using actual API key here so it will be avail for later integration tests..
  grep "CRONITOR_API_KEY" $CLI_CONFIGFILE | grep -q "$CLI_ACTUAL_API_KEY"
}

@test "Configure writes API Key correctly to custom config file set by param" {
  ../cronitor $CRONITOR_ARGS configure --config $CLI_CONFIGFILE_ALTERNATE --api-key "$MSG"
  grep "CRONITOR_API_KEY" $CLI_CONFIGFILE_ALTERNATE | grep -q "$MSG"
}

@test "Configure writes API Key correctly to custom config file set by env var" {
  CRONITOR_CONFIG=$CLI_CONFIGFILE_ALTERNATE ../cronitor $CRONITOR_ARGS configure --api-key "$MSG"
  grep "CRONITOR_API_KEY" $CLI_CONFIGFILE_ALTERNATE | grep -q "$MSG"
}

@test "Configure writes Ping API Key correctly to config file" {
  ../cronitor $CRONITOR_ARGS configure --ping-api-key "$MSG"
  grep "CRONITOR_PING_API_KEY" $CLI_CONFIGFILE | grep -q "$MSG"
}

TEST="Configure writes log path correctly to config file"
../cronitor $CRONITOR_ARGS configure --log $CLI_LOGFILE_ALTERNATE
if grep "CRONITOR_LOG" $CLI_CONFIGFILE | grep -q $CLI_LOGFILE_ALTERNATE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

@test "Configure writes exclude text correctly to config file" {
  ../cronitor $CRONITOR_ARGS configure --exclude-from-name "$MSG"
  grep -q "CRONITOR_EXCLUDE_TEXT" $CLI_CONFIGFILE && grep -q "$MSG" $CLI_CONFIGFILE
}

@test "Configure writes multiple exclude text entries correctly to config file" {
  ../cronitor $CRONITOR_ARGS configure --exclude-from-name "${MSG}A" --exclude-from-name "${MSG}B"
  grep -q "CRONITOR_EXCLUDE_TEXT" $CLI_CONFIGFILE && grep -q "${MSG}A" $CLI_CONFIGFILE && grep -q "${MSG}B" $CLI_CONFIGFILE
}
