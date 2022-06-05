#!/usr/bin/env bats

setup() {
  SCRIPT_DIR="$(dirname $BATS_TEST_FILENAME)"
  cd $SCRIPT_DIR

  source $SCRIPT_DIR/setup.sh
  rm -f $CLI_LOGFILE
}

#################
# PING TESTS
#################

@test "Ping without args" {
  ../cronitor $CRONITOR_ARGS ping d3x0c1 --run --log $CLI_LOGFILE
  grep -q "Sending ping ${HOSTNAME}/d3x0c1/run" $CLI_LOGFILE
}

@test "Ping with custom hostname" {
  ../cronitor $CRONITOR_ARGS ping d3x0c1 --run --hostname customHostnameForTest --log $CLI_LOGFILE
  grep -q "Sending ping ${HOSTNAME}/d3x0c1/run" $CLI_LOGFILE && grep -q "host=customHostnameForTest" $CLI_LOGFILE
}

@test "Ping with message" {
  MSG="messagewithoutspaces"
  ../cronitor $CRONITOR_ARGS ping d3x0c1 --run --msg "$MSG" --log $CLI_LOGFILE
  grep -q "Sending ping ${HOSTNAME}/d3x0c1/run" $CLI_LOGFILE && grep -q "$MSG" $CLI_LOGFILE
}

@test "Ping with ping api key" {
  KEY="XXXXXXXXXX"
  ../cronitor $CRONITOR_ARGS ping d3x0c1 --run --ping-api-key $KEY --log $CLI_LOGFILE
  grep -q "Sending ping" $CLI_LOGFILE && grep -q "${KEY}" $CLI_LOGFILE
}

@test "Ping integration test" {
  MSG=`date`
  ../cronitor $CRONITOR_ARGS ping 44oI2n --run --msg "$MSG" --log $CLI_LOGFILE && sleep 1
  ../cronitor $CRONITOR_ARGS activity 44oI2n | grep -q "$MSG"
}
