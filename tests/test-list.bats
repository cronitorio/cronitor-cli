#!/usr/bin/env bats

setup() {
  SCRIPT_DIR="$(dirname $BATS_TEST_FILENAME)"
  cd $SCRIPT_DIR

  source $SCRIPT_DIR/setup.sh
  rm -f $CLI_LOGFILE
}

#################
# LIST TESTS
#################

@test "List reads crontab and writes table" {
  ../cronitor $CRONITOR_ARGS list ../fixtures/crontab.txt | grep -q "/usr/bin/true"
}

@test "List reads crontab and formats table correctly" {
  ../cronitor $CRONITOR_ARGS list ../fixtures/crontab.txt | grep -q "\-----"
}