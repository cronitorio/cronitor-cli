#!/usr/bin/env bats

setup() {
  SCRIPT_DIR="$(dirname $BATS_TEST_FILENAME)"
  FIXTURES_DIR="$(dirname $SCRIPT_DIR)/fixtures"
  cd $SCRIPT_DIR

  load test_helper
  API_KEY="$CRONITOR_API_KEY"
  TMPFILE="$BATS_TMPDIR/crontab.txt"
}

teardown() {
  rm -f $TMPFILE
  rm -f $CLI_LOGFILE
  rm -f $CLI_CRONTAB_TEMP
}

#################
# DISCOVER TESTS
#################

@test "Discover reads file and sends PUT" {
  ../cronitor $CRONITOR_ARGS discover --auto $FIXTURES_DIR/crontab.txt -k "$API_KEY" --log $CLI_LOGFILE > /dev/null
  grep -q "Request" $CLI_LOGFILE
}

@test "Discover parses response and rewrites crontab" {
  run ../cronitor $CRONITOR_ARGS discover --auto $FIXTURES_DIR/crontab.txt -k "$API_KEY"
  echo "$output" >&3
  echo "$output" | grep "slave_status.sh" | grep -q "cronitor exec"
}

@test "Discover is silent when being run under exec" {
 [[ $(../cronitor $CRONITOR_ARGS exec d3x0c1 ../cronitor $CRONITOR_ARGS discover --auto $FIXTURES_DIR/crontab.txt -k "$API_KEY" | wc -c) -eq 0 ]]
}

@test "Discover correctly parses crontab with username" {
  echo "* * * * * $CLI_USERNAME echo 'username parse'" | cat - $FIXTURES_DIR/crontab.txt > $CLI_CRONTAB_TEMP
  ../cronitor $CRONITOR_ARGS discover --auto $CLI_CRONTAB_TEMP -k "$API_KEY" | grep "echo '" | grep -q "$CLI_USERNAME cronitor exec"
}

@test "Discover correctly parses crontab with 6 digits" {
  echo "* * * * * 0 echo 'six dig parse'" | cat - $FIXTURES_DIR/crontab.txt > $CLI_CRONTAB_TEMP
  ../cronitor $CRONITOR_ARGS discover --auto $CLI_CRONTAB_TEMP -k "$API_KEY"| grep "echo '" | grep -q "0 cronitor exec"
}

@test "Discover correctly parses crontab with 6th digit DoW string range" {
  echo "* * * * * Mon-Fri echo 'DoW string parse'" | cat - $FIXTURES_DIR/crontab.txt > $CLI_CRONTAB_TEMP
  ../cronitor $CRONITOR_ARGS discover --auto $CLI_CRONTAB_TEMP -k "$API_KEY" | grep "echo '" | grep -q "Mon-Fri cronitor exec"
}

@test "Discover correctly parses crontab with 6th digit DoW string list" {
  echo "* * * * * Mon,Wed,Fri echo 'DoW string list parse'" | cat - $FIXTURES_DIR/crontab.txt > $CLI_CRONTAB_TEMP
  ../cronitor $CRONITOR_ARGS discover --auto $CLI_CRONTAB_TEMP -k "$API_KEY" | grep "echo '" | grep -q "Mon,Wed,Fri cronitor exec"
}

@test "Discover correctly parses crontab with 6th digit DoW string name" {
  echo "* * * * * Mon echo 'DoW string name parse'" | cat - $FIXTURES_DIR/crontab.txt > $CLI_CRONTAB_TEMP
  ../cronitor $CRONITOR_ARGS discover --auto $CLI_CRONTAB_TEMP -k "$API_KEY" | grep "echo '" | grep -q "Mon cronitor exec"
}

@test "Discover rewrites crontab in place" {
  cp $FIXTURES_DIR/crontab.txt $TMPFILE
  ../cronitor $CRONITOR_ARGS discover --auto $TMPFILE -k "$API_KEY" > /dev/null
  grep "slave_status.sh" $TMPFILE | grep -q "cronitor exec"
}

@test "Discover ignores meta crontab entries" {
  cp $FIXTURES_DIR/metacrontab.txt $TMPFILE
  ../cronitor $CRONITOR_ARGS discover --auto $TMPFILE -k "$API_KEY" > /dev/null
  run -1 bash -c 'grep "cron.hourly" $TMPFILE | grep -q "cronitor exec"'
}

@test "Discover adds no-stdout flag when supplied" {
  run -1 bash -c '../cronitor $CRONITOR_ARGS discover --auto -v $FIXTURES_DIR/crontab.txt -k "$API_KEY" --no-stdout | grep "cronitor exec" | grep -q "no-stdout"'
}

@test "Discover omits 'notifications' if notification-list not specificed" {
  run -1 bash -c '../cronitor $CRONITOR_ARGS discover --auto -v $FIXTURES_DIR/crontab.txt -k "$API_KEY" | grep -q "notifications"'
}

@test "Discover includes custom notification-list" {
  ../cronitor $CRONITOR_ARGS discover --auto -v $FIXTURES_DIR/crontab.txt -k "$API_KEY" --notification-list test-list-name | grep -q "test-list-name"
}

@test "Discover reads all of the crontabs in a directory" {
  OUTPUT="$(../cronitor $CRONITOR_ARGS discover --auto $FIXTURES_DIR/cron.d -k "$API_KEY")"
  echo "$OUTPUT" | grep -q "every_minute" && echo "$OUTPUT" | grep -q "top_of_hour"
}