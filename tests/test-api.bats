#!/usr/bin/env bats

setup() {
  SCRIPT_DIR="$(dirname $BATS_TEST_FILENAME)"
  cd $SCRIPT_DIR
  rm -f $CLI_LOGFILE
}

#################
# MONITOR COMMAND TESTS
#################

@test "monitor command shows help" {
  ../cronitor monitor --help | grep -qi "manage.*monitors"
}

@test "monitor command lists subcommands" {
  ../cronitor monitor --help | grep -q "list"
  ../cronitor monitor --help | grep -q "get"
  ../cronitor monitor --help | grep -q "create"
  ../cronitor monitor --help | grep -q "update"
  ../cronitor monitor --help | grep -q "delete"
  ../cronitor monitor --help | grep -q "pause"
  ../cronitor monitor --help | grep -q "unpause"
}

@test "monitor list shows help" {
  ../cronitor monitor list --help | grep -q "List all monitors"
}

@test "monitor list has pagination flag" {
  ../cronitor monitor list --help | grep -q "\-\-page"
}

@test "monitor list has env flag" {
  ../cronitor monitor list --help | grep -q "\-\-env"
}

@test "monitor get requires key" {
  run ../cronitor monitor get 2>&1
  [ "$status" -eq 1 ]
}

@test "monitor get has --with-events flag" {
  ../cronitor monitor get --help | grep -q "\-\-with-events"
}

@test "monitor create has --data flag" {
  ../cronitor monitor create --help | grep -q "\-\-data"
}

@test "monitor create has --file flag" {
  ../cronitor monitor create --help | grep -q "\-\-file"
}

@test "monitor update requires key" {
  run ../cronitor monitor update 2>&1
  [ "$status" -eq 1 ]
}

@test "monitor delete requires key" {
  run ../cronitor monitor delete 2>&1
  [ "$status" -eq 1 ]
}

@test "monitor pause requires key" {
  run ../cronitor monitor pause 2>&1
  [ "$status" -eq 1 ]
}

@test "monitor pause has --hours flag" {
  ../cronitor monitor pause --help | grep -q "\-\-hours"
}

@test "monitor unpause requires key" {
  run ../cronitor monitor unpause 2>&1
  [ "$status" -eq 1 ]
}

#################
# STATUSPAGE COMMAND TESTS
#################

@test "statuspage command shows help" {
  ../cronitor statuspage --help | grep -qi "manage.*status"
}

@test "statuspage command lists subcommands" {
  ../cronitor statuspage --help | grep -q "list"
  ../cronitor statuspage --help | grep -q "get"
  ../cronitor statuspage --help | grep -q "create"
  ../cronitor statuspage --help | grep -q "update"
  ../cronitor statuspage --help | grep -q "delete"
}

@test "statuspage list shows help" {
  ../cronitor statuspage list --help | grep -qi "list"
}

@test "statuspage get requires key" {
  run ../cronitor statuspage get 2>&1
  [ "$status" -eq 1 ]
}

@test "statuspage update requires key" {
  run ../cronitor statuspage update 2>&1
  [ "$status" -eq 1 ]
}

@test "statuspage delete requires key" {
  run ../cronitor statuspage delete 2>&1
  [ "$status" -eq 1 ]
}

#################
# ISSUE COMMAND TESTS
#################

@test "issue command shows help" {
  ../cronitor issue --help | grep -qi "manage.*issues"
}

@test "issue command lists subcommands" {
  ../cronitor issue --help | grep -q "list"
  ../cronitor issue --help | grep -q "get"
  ../cronitor issue --help | grep -q "create"
  ../cronitor issue --help | grep -q "update"
  ../cronitor issue --help | grep -q "resolve"
  ../cronitor issue --help | grep -q "delete"
}

@test "issue list has --state flag" {
  ../cronitor issue list --help | grep -q "\-\-state"
}

@test "issue list has --severity flag" {
  ../cronitor issue list --help | grep -q "\-\-severity"
}

@test "issue list has --monitor flag" {
  ../cronitor issue list --help | grep -q "\-\-monitor"
}

@test "issue get requires key" {
  run ../cronitor issue get 2>&1
  [ "$status" -eq 1 ]
}

@test "issue resolve requires key" {
  run ../cronitor issue resolve 2>&1
  [ "$status" -eq 1 ]
}

@test "issue delete requires key" {
  run ../cronitor issue delete 2>&1
  [ "$status" -eq 1 ]
}

#################
# NOTIFICATION COMMAND TESTS
#################

@test "notification command shows help" {
  ../cronitor notification --help | grep -qi "notification"
}

@test "notification command lists subcommands" {
  ../cronitor notification --help | grep -q "list"
  ../cronitor notification --help | grep -q "get"
  ../cronitor notification --help | grep -q "create"
  ../cronitor notification --help | grep -q "update"
  ../cronitor notification --help | grep -q "delete"
}

@test "notification has alias 'notifications'" {
  ../cronitor notifications --help | grep -qi "notification"
}

@test "notification get requires key" {
  run ../cronitor notification get 2>&1
  [ "$status" -eq 1 ]
}

@test "notification update requires key" {
  run ../cronitor notification update 2>&1
  [ "$status" -eq 1 ]
}

@test "notification delete requires key" {
  run ../cronitor notification delete 2>&1
  [ "$status" -eq 1 ]
}

#################
# ENVIRONMENT COMMAND TESTS
#################

@test "environment command shows help" {
  ../cronitor environment --help | grep -qi "environment"
}

@test "environment command lists subcommands" {
  ../cronitor environment --help | grep -q "list"
  ../cronitor environment --help | grep -q "get"
  ../cronitor environment --help | grep -q "create"
  ../cronitor environment --help | grep -q "update"
  ../cronitor environment --help | grep -q "delete"
}

@test "environment has alias 'env'" {
  ../cronitor env --help | grep -qi "environment"
}

@test "environment get requires key" {
  run ../cronitor environment get 2>&1
  [ "$status" -eq 1 ]
}

@test "environment update requires key" {
  run ../cronitor environment update 2>&1
  [ "$status" -eq 1 ]
}

@test "environment delete requires key" {
  run ../cronitor environment delete 2>&1
  [ "$status" -eq 1 ]
}

#################
# GLOBAL FLAGS TESTS
#################

@test "monitor has --format flag" {
  ../cronitor monitor --help | grep -q "\-\-format"
}

@test "monitor has --output flag" {
  ../cronitor monitor --help | grep -q "\-o, \-\-output"
}

@test "statuspage has --format flag" {
  ../cronitor statuspage --help | grep -q "\-\-format"
}

@test "issue has --format flag" {
  ../cronitor issue --help | grep -q "\-\-format"
}

#################
# INTEGRATION TESTS (SKIPPED BY DEFAULT)
#################

@test "monitor list integration test" {
  if [ -z "$CRONITOR_API_KEY" ]; then skip "Requires CRONITOR_API_KEY"; fi
  run ../cronitor monitor list --format json
  [ "$status" -eq 0 ]
}

@test "issue list integration test" {
  if [ -z "$CRONITOR_API_KEY" ]; then skip "Requires CRONITOR_API_KEY"; fi
  run ../cronitor issue list --format json
  [ "$status" -eq 0 ]
}

@test "statuspage list integration test" {
  if [ -z "$CRONITOR_API_KEY" ]; then skip "Requires CRONITOR_API_KEY"; fi
  run ../cronitor statuspage list --format json
  [ "$status" -eq 0 ]
}
