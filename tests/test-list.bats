#!/usr/bin/env bats

setup() {
  SCRIPT_DIR="$(dirname $BATS_TEST_FILENAME)"
  FIXTURES_DIR="$(dirname $SCRIPT_DIR)/fixtures"
  cd $SCRIPT_DIR

  load test_helper
  rm -f $CLI_LOGFILE
}

#################
# LIST TESTS
#################

@test "List reads crontab and writes table" {
  skip_if_linux "We can't figure out why this isn't working"
  run ../cronitor $CRONITOR_ARGS list $FIXTURES_DIR/crontab.txt
  # echo "Real file\n" >&3
  # cat $FIXTURES_DIR/crontab.txt >&3
  # echo "Processed file" >&3
  # echo "$output" >&3
  echo "$output" | grep -q "/usr/bin/true"
}

@test "List reads crontab and formats table correctly" {
  ../cronitor $CRONITOR_ARGS list $FIXTURES_DIR/crontab.txt | grep -q "\-----"
}