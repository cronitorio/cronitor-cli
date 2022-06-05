#!/usr/bin/env bats

setup() {
  SCRIPT_DIR="$(dirname $BATS_TEST_FILENAME)"
  FIXTURES_DIR="$SCRIPT_DIR/../fixtures"
  cd $SCRIPT_DIR

  source $SCRIPT_DIR/setup.sh
  rm -f $CLI_LOGFILE

  API_KEY="53b6c114717140cf896899060bcc9d7e"
}

teardown() {
  rm -f $TMPFILE
}

#################
# DISCOVER TESTS
#################

@test "Discover reads file and sends PUT" {
  ../cronitor $CRONITOR_ARGS discover --auto $FIXTURES_DIR/crontab.txt -k 53b6c114717140cf896899060bcc9d7e --log $CLI_LOGFILE > /dev/null
  grep -q "Request" $CLI_LOGFILE
}

@test "Discover parses response and rewrites crontab" {
  ../cronitor $CRONITOR_ARGS discover --auto $FIXTURES_DIR/crontab.txt -k 53b6c114717140cf896899060bcc9d7e| grep "slave_status.sh" | grep -q "cronitor exec"
}


@test "Discover is silent when being run under exec" {
 [[ $(../cronitor $CRONITOR_ARGS exec d3x0c1 cronitor $CRONITOR_ARGS discover --auto $FIXTURES_DIR/crontab.txt -k 53b6c114717140cf896899060bcc9d7e | wc -c) -eq 0 ]]
}


@test "Discover correctly parses crontab with username" {
  echo "* * * * * $CLI_USERNAME echo 'username parse'" | cat - $FIXTURES_DIR/crontab.txt > $CLI_CRONTAB_TEMP
  ../cronitor $CRONITOR_ARGS discover --auto $CLI_CRONTAB_TEMP -k 53b6c114717140cf896899060bcc9d7e| grep "echo '" | grep -q "$CLI_USERNAME cronitor exec"
}

@test "Discover correctly parses crontab with 6 digits" {
  echo "* * * * * 0 echo 'six dig parse'" | cat - $FIXTURES_DIR/crontab.txt > $CLI_CRONTAB_TEMP
  ../cronitor $CRONITOR_ARGS discover --auto $CLI_CRONTAB_TEMP -k 53b6c114717140cf896899060bcc9d7e| grep "echo '" | grep -q "0 cronitor exec"
}

@test "Discover correctly parses crontab with 6th digit DoW string range" {
  echo "* * * * * Mon-Fri echo 'DoW string parse'" | cat - $FIXTURES_DIR/crontab.txt > $CLI_CRONTAB_TEMP
  ../cronitor $CRONITOR_ARGS discover --auto $CLI_CRONTAB_TEMP -k 53b6c114717140cf896899060bcc9d7e| grep "echo '" | grep -q "Mon-Fri cronitor exec"
}

@test "Discover correctly parses crontab with 6th digit DoW string list" {
  echo "* * * * * Mon,Wed,Fri echo 'DoW string list parse'" | cat - $FIXTURES_DIR/crontab.txt > $CLI_CRONTAB_TEMP
  ../cronitor $CRONITOR_ARGS discover --auto $CLI_CRONTAB_TEMP -k 53b6c114717140cf896899060bcc9d7e| grep "echo '" | grep -q "Mon,Wed,Fri cronitor exec"
}

@test "Discover correctly parses crontab with 6th digit DoW string name" {
  echo "* * * * * Mon echo 'DoW string name parse'" | cat - $FIXTURES_DIR/crontab.txt > $CLI_CRONTAB_TEMP
  ../cronitor $CRONITOR_ARGS discover --auto $CLI_CRONTAB_TEMP -k 53b6c114717140cf896899060bcc9d7e| grep "echo '" | grep -q "Mon cronitor exec"
}

@test "Discover rewrites crontab in place" {
  TMPFILE="/tmp/crontab.txt"
  cp $FIXTURES_DIR/crontab.txt $TMPFILE
  ../cronitor $CRONITOR_ARGS discover --auto $TMPFILE -k 53b6c114717140cf896899060bcc9d7e > /dev/null
  grep "slave_status.sh" $TMPFILE | grep -q "cronitor exec"
}

@test "Discover ignores meta crontab entries" {
  TMPFILE="/tmp/crontab.txt"
  cp $FIXTURES_DIR/metacrontab.txt $TMPFILE
  ../cronitor $CRONITOR_ARGS discover --auto $TMPFILE -k 53b6c114717140cf896899060bcc9d7e > /dev/null
  run grep "cron.hourly" $TMPFILE | grep -q "cronitor exec"
  [ "$status" -eq 1 ]
}

@test "Discover adds no-stdout flag when supplied" {
  run -1 ../cronitor $CRONITOR_ARGS discover --auto -v $FIXTURES_DIR/crontab.txt -k 53b6c114717140cf896899060bcc9d7e --no-stdout | grep "cronitor exec" | grep -q "no-stdout"
}

@test "Discover omits 'notifications' if notification-list not specificed" {
  run -1 ../cronitor $CRONITOR_ARGS discover --auto -v $FIXTURES_DIR/crontab.txt -k 53b6c114717140cf896899060bcc9d7e | grep -q "notifications"
}

@test "Discover includes custom notification-list" {
  ../cronitor $CRONITOR_ARGS discover --auto -v $FIXTURES_DIR/crontab.txt -k 53b6c114717140cf896899060bcc9d7e --notification-list test-list-name | grep -q "test-list-name"
}

@test "Discover reads all of the crontabs in a directory" {
  OUTPUT="$(../cronitor $CRONITOR_ARGS discover --auto $FIXTURES_DIR/cron.d -k 53b6c114717140cf896899060bcc9d7e)"
  echo "$OUTPUT" | grep -q "every_minute" && echo "$OUTPUT" | grep -q "top_of_hour"
}