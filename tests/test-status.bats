#!/usr/bin/env bats

setup() {
  SCRIPT_DIR="$(dirname $BATS_TEST_FILENAME)"
  cd $SCRIPT_DIR

  load test_helper
  CLI_LOGFILE=$BATS_TMPDIR/test-build.log
}

teardown() {
  rm -f $CLI_LOGFILE
}

#################
# STATUS TESTS
#################

@test "Status integration test without filter" {
  skip "Integration test requires monitors to exist on Cronitor servers"
}

@test "Status integration test with filter" {
  skip "Integration test requires monitor d3x0c1 to exist on Cronitor servers"
}

@test "Status integration test with bad monitor code" {
  skip "Integration test - error message format varies by platform"
}
