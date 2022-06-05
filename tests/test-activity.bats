#!/usr/bin/env bats

setup() {
  SCRIPT_DIR="$BATS_TEST_FILENAME"
  cd $SCRIPT_DIR

  source $SCRIPT_DIR/setup.sh
  rm -f $CLI_LOGFILE
}

#################
# ACTIVITY TESTS
#################

echo ""

@test "Activity integration test without filter" {
  ../cronitor $CRONITOR_ARGS activity 44oI2n --log $CLI_LOGFILE | grep -q "monitor_name"
}

@test "Activity integration test with only pings filter" {
  ../cronitor $CRONITOR_ARGS activity 44oI2n --only pings --log $CLI_LOGFILE | grep -q "monitor_name"
}

@test "Activity integration test with only alerts filter" {
  ../cronitor $CRONITOR_ARGS activity 44oI2n --only alerts --log $CLI_LOGFILE | grep -q -v "\"description\": \"ping\""
}
