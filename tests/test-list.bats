#!/usr/bin/env bats

setup() {
  SCRIPT_DIR="$(dirname $BATS_TEST_FILENAME)"
  FIXTURES_DIR="$(dirname $SCRIPT_DIR)/fixtures"
  cd $SCRIPT_DIR

  # load setup.bash
  rm -f $CLI_LOGFILE
}

#################
# LIST TESTS
#################

@test "List reads crontab and writes table" {
  run ../cronitor $CRONITOR_ARGS list $FIXTURES_DIR/crontab.txt
  echo "Real file\n"
  cat $FIXTURES_DIR/crontab.txt >&3
  echo "Processed file"
  echo "$output" >&3
  echo "$output" | grep -q "/usr/bin/true"
}

@test "List reads crontab and formats table correctly" {
  ../cronitor $CRONITOR_ARGS list $FIXTURES_DIR/crontab.txt | grep -q "\-----"
}