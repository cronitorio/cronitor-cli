#!/usr/bin/env bats

setup() {
  SCRIPT_DIR="$(dirname $BATS_TEST_FILENAME)"
  cd $SCRIPT_DIR

  # load setup.bash
  rm -f $CLI_LOGFILE
}

#################
# STATUS TESTS
#################

@test "Status integration test without filter" {
  ../cronitor $CRONITOR_ARGS status --log $CLI_LOGFILE | grep -q "Ok"
}

@test "Status integration test with filter" {
  ../cronitor $CRONITOR_ARGS status OFY0dB --log $CLI_LOGFILE | grep -q "Ok"
}

@test "Status integration test with bad monitor code" {
  ../cronitor $CRONITOR_ARGS status asdfgh --log $CLI_LOGFILE 2>&1 | grep -q "could not be found"
}
