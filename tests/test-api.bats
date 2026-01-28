#!/usr/bin/env bats

setup() {
  SCRIPT_DIR="$(dirname $BATS_TEST_FILENAME)"
  cd $SCRIPT_DIR

  rm -f $CLI_LOGFILE
}

#################
# API COMMAND TESTS
#################

@test "API command shows help" {
  ../cronitor api --help | grep -q "Interact with the Cronitor API"
}

@test "API command lists available subcommands" {
  ../cronitor api --help | grep -q "monitors"
  ../cronitor api --help | grep -q "issues"
  ../cronitor api --help | grep -q "statuspages"
  ../cronitor api --help | grep -q "components"
  ../cronitor api --help | grep -q "incidents"
  ../cronitor api --help | grep -q "metrics"
  ../cronitor api --help | grep -q "notifications"
  ../cronitor api --help | grep -q "environments"
}

@test "API command requires API key" {
  # When no API key is configured, the command should fail
  run ../cronitor api monitors 2>&1
  [ "$status" -eq 1 ]
}

#################
# MONITORS SUBCOMMAND TESTS
#################

@test "API monitors shows help" {
  ../cronitor api monitors --help | grep -q "Manage Cronitor monitors"
}

@test "API monitors help shows all actions" {
  ../cronitor api monitors --help | grep -q "list"
  ../cronitor api monitors --help | grep -q "get"
  ../cronitor api monitors --help | grep -q "create"
  ../cronitor api monitors --help | grep -q "update"
  ../cronitor api monitors --help | grep -q "delete"
  ../cronitor api monitors --help | grep -q "pause"
  ../cronitor api monitors --help | grep -q "unpause"
}

@test "API monitors has --hours flag for pause" {
  ../cronitor api monitors --help | grep -q "\-\-hours"
}

@test "API monitors has --with-events flag" {
  ../cronitor api monitors --help | grep -q "\-\-with-events"
}

@test "API monitors get requires key" {
  run ../cronitor api monitors get -k test-api-key 2>&1
  [ "$status" -eq 1 ]
  [[ "$output" == *"key is required"* ]]
}

@test "API monitors update requires key" {
  run ../cronitor api monitors update -k test-api-key 2>&1
  [ "$status" -eq 1 ]
  [[ "$output" == *"key is required"* ]]
}

@test "API monitors pause requires key" {
  run ../cronitor api monitors pause -k test-api-key 2>&1
  [ "$status" -eq 1 ]
  [[ "$output" == *"key is required"* ]]
}

@test "API monitors create requires body" {
  run ../cronitor api monitors create -k test-api-key 2>&1
  [ "$status" -eq 1 ]
  [[ "$output" == *"request body is required"* ]]
}

#################
# ISSUES SUBCOMMAND TESTS
#################

@test "API issues shows help" {
  ../cronitor api issues --help | grep -q "Manage Cronitor issues"
}

@test "API issues help shows all actions" {
  ../cronitor api issues --help | grep -q "list"
  ../cronitor api issues --help | grep -q "get"
  ../cronitor api issues --help | grep -q "create"
  ../cronitor api issues --help | grep -q "update"
  ../cronitor api issues --help | grep -q "delete"
  ../cronitor api issues --help | grep -q "bulk"
}

@test "API issues has --state flag" {
  ../cronitor api issues --help | grep -q "\-\-state"
}

@test "API issues has --severity flag" {
  ../cronitor api issues --help | grep -q "\-\-severity"
}

@test "API issues get requires key" {
  run ../cronitor api issues get -k test-api-key 2>&1
  [ "$status" -eq 1 ]
  [[ "$output" == *"key is required"* ]]
}

@test "API issues create requires body" {
  run ../cronitor api issues create -k test-api-key 2>&1
  [ "$status" -eq 1 ]
  [[ "$output" == *"request body is required"* ]]
}

#################
# STATUSPAGES SUBCOMMAND TESTS
#################

@test "API statuspages shows help" {
  ../cronitor api statuspages --help | grep -q "Manage Cronitor status pages"
}

@test "API statuspages help shows all actions" {
  ../cronitor api statuspages --help | grep -q "list"
  ../cronitor api statuspages --help | grep -q "get"
  ../cronitor api statuspages --help | grep -q "create"
  ../cronitor api statuspages --help | grep -q "update"
  ../cronitor api statuspages --help | grep -q "delete"
}

@test "API statuspages get requires key" {
  run ../cronitor api statuspages get -k test-api-key 2>&1
  [ "$status" -eq 1 ]
  [[ "$output" == *"key is required"* ]]
}

@test "API statuspages create requires body" {
  run ../cronitor api statuspages create -k test-api-key 2>&1
  [ "$status" -eq 1 ]
  [[ "$output" == *"request body is required"* ]]
}

#################
# COMPONENTS SUBCOMMAND TESTS
#################

@test "API components shows help" {
  ../cronitor api components --help | grep -qi "status page components"
}

@test "API components help shows all actions" {
  ../cronitor api components --help | grep -q "list"
  ../cronitor api components --help | grep -q "get"
  ../cronitor api components --help | grep -q "create"
  ../cronitor api components --help | grep -q "update"
  ../cronitor api components --help | grep -q "delete"
}

@test "API components has --statuspage flag" {
  ../cronitor api components --help | grep -q "\-\-statuspage"
}

@test "API components get requires key" {
  run ../cronitor api components get -k test-api-key 2>&1
  [ "$status" -eq 1 ]
  [[ "$output" == *"key is required"* ]]
}

#################
# INCIDENTS SUBCOMMAND TESTS
#################

@test "API incidents shows help" {
  ../cronitor api incidents --help | grep -qi "status page incidents"
}

@test "API incidents help shows all actions" {
  ../cronitor api incidents --help | grep -q "list"
  ../cronitor api incidents --help | grep -q "get"
  ../cronitor api incidents --help | grep -q "create"
  ../cronitor api incidents --help | grep -q "update"
  ../cronitor api incidents --help | grep -q "resolve"
}

@test "API incidents has --statuspage flag" {
  ../cronitor api incidents --help | grep -q "\-\-statuspage"
}

@test "API incidents get requires key" {
  run ../cronitor api incidents get -k test-api-key 2>&1
  [ "$status" -eq 1 ]
  [[ "$output" == *"ID is required"* ]]
}

@test "API incidents resolve requires key" {
  run ../cronitor api incidents resolve -k test-api-key 2>&1
  [ "$status" -eq 1 ]
  [[ "$output" == *"ID is required"* ]]
}

#################
# METRICS SUBCOMMAND TESTS
#################

@test "API metrics shows help" {
  ../cronitor api metrics --help | grep -q "View monitor metrics"
}

@test "API metrics has time range flags" {
  ../cronitor api metrics --help | grep -q "\-\-start"
  ../cronitor api metrics --help | grep -q "\-\-end"
}

@test "API metrics has --aggregates flag" {
  ../cronitor api metrics --help | grep -q "\-\-aggregates"
}

@test "API metrics has --group flag" {
  ../cronitor api metrics --help | grep -q "\-\-group"
}

#################
# NOTIFICATIONS SUBCOMMAND TESTS
#################

@test "API notifications shows help" {
  ../cronitor api notifications --help | grep -qi "notification lists"
}

@test "API notifications help shows all actions" {
  ../cronitor api notifications --help | grep -q "list"
  ../cronitor api notifications --help | grep -q "get"
  ../cronitor api notifications --help | grep -q "create"
  ../cronitor api notifications --help | grep -q "update"
  ../cronitor api notifications --help | grep -q "delete"
}

@test "API notifications get requires key" {
  run ../cronitor api notifications get -k test-api-key 2>&1
  [ "$status" -eq 1 ]
  [[ "$output" == *"key is required"* ]]
}

@test "API notifications create requires body" {
  run ../cronitor api notifications create -k test-api-key 2>&1
  [ "$status" -eq 1 ]
  [[ "$output" == *"request body is required"* ]]
}

#################
# ENVIRONMENTS SUBCOMMAND TESTS
#################

@test "API environments shows help" {
  ../cronitor api environments --help | grep -qi "environments"
}

@test "API environments help shows all actions" {
  ../cronitor api environments --help | grep -q "list"
  ../cronitor api environments --help | grep -q "get"
  ../cronitor api environments --help | grep -q "create"
  ../cronitor api environments --help | grep -q "update"
  ../cronitor api environments --help | grep -q "delete"
}

@test "API environments get requires key" {
  run ../cronitor api environments get -k test-api-key 2>&1
  [ "$status" -eq 1 ]
  [[ "$output" == *"key is required"* ]]
}

@test "API environments create requires body" {
  run ../cronitor api environments create -k test-api-key 2>&1
  [ "$status" -eq 1 ]
  [[ "$output" == *"request body is required"* ]]
}

#################
# GLOBAL FLAGS TESTS
#################

@test "API command has --data flag" {
  ../cronitor api --help | grep -q "\-d, \-\-data"
}

@test "API command has --file flag" {
  ../cronitor api --help | grep -q "\-f, \-\-file"
}

@test "API command has --format flag" {
  ../cronitor api --help | grep -q "\-\-format"
}

@test "API command has --page flag" {
  ../cronitor api --help | grep -q "\-\-page"
}

@test "API command has --output flag" {
  ../cronitor api --help | grep -q "\-o, \-\-output"
}

@test "API command has --raw flag" {
  ../cronitor api --help | grep -q "\-\-raw"
}

@test "API command has --env global flag" {
  ../cronitor api --help | grep -q "\-\-env"
}

#################
# JSON VALIDATION TESTS
#################

@test "API monitors create rejects invalid JSON" {
  run ../cronitor api monitors create -k test-api-key --data 'not valid json' 2>&1
  [ "$status" -eq 1 ]
  [[ "$output" == *"invalid JSON"* ]]
}

@test "API issues create rejects invalid JSON" {
  run ../cronitor api issues create -k test-api-key --data '{broken' 2>&1
  [ "$status" -eq 1 ]
  [[ "$output" == *"invalid JSON"* ]]
}

#################
# INTEGRATION TESTS (SKIPPED BY DEFAULT)
#################

@test "API monitors list integration test" {
  skip "Integration test requires valid API key"
  # ../cronitor api monitors -k $CRONITOR_API_KEY
}

@test "API monitors get integration test" {
  skip "Integration test requires valid API key and existing monitor"
  # ../cronitor api monitors get test-monitor -k $CRONITOR_API_KEY
}

@test "API monitors create integration test" {
  skip "Integration test requires valid API key"
  # ../cronitor api monitors create -k $CRONITOR_API_KEY --data '{"key":"test-cli-monitor","type":"job"}'
}

@test "API issues list integration test" {
  skip "Integration test requires valid API key"
  # ../cronitor api issues -k $CRONITOR_API_KEY
}

@test "API statuspages list integration test" {
  skip "Integration test requires valid API key"
  # ../cronitor api statuspages -k $CRONITOR_API_KEY
}

@test "API metrics integration test" {
  skip "Integration test requires valid API key"
  # ../cronitor api metrics -k $CRONITOR_API_KEY
}
