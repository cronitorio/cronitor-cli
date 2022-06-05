#!/usr/bin/env bats

setup() {
  SCRIPT_DIR="$(dirname $BATS_TEST_FILENAME)"
  cd $SCRIPT_DIR

  PROJECT_DIR="$(dirname $SCRIPT_DIR)"

  source $SCRIPT_DIR/setup.sh

  rm -f $CLI_LOGFILE
}

#################
# EXEC TESTS
#################

@test "Exec uses bash when available" {
  [[ "$(../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 $PROJECT_DIR/bin/test-bash.sh)"  == "i am an array" ]]
}


@test "Exec runs command check" {
  ../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 ./write-to-log-success.sh $CLI_LOGFILE "$TEST" > /dev/null
  grep -q "$TEST" $CLI_LOGFILE
}

@test "Exec runs command with complex args" {
  ../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 $PROJECT_DIR/bin/success.sh "arg with space" anotherArg > /dev/null
  grep -q "arg with space" $CLI_LOGFILE
}

@test "Exec runs command with really complex args" {
  ../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 "cd /tmp && pwd" > /dev/null
  grep -q "/tmp" $CLI_LOGFILE
}


@test "Exec sends complete ping on success" {
  ../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 true > /dev/null
  grep "Sending ping" $CLI_LOGFILE | grep -q "state=complete"
}

@test "Exec sends fail ping on failure" {
  ../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 false > /dev/null
  grep "Sending ping" $CLI_LOGFILE | grep -q "state=fail"
}

@test "Exec sends status code on complete ping" {
  ../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 $PROJECT_DIR/bin/fail.sh > /dev/null
  grep -q "&status_code=123" $CLI_LOGFILE
}

@test "Exec sends environment in pings" {
  ../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE --env test exec d3x0c1 true > /dev/null
  grep -q "&env=test" $CLI_LOGFILE
}


@test "Exec sends run timestamp as complete ping series" {
  ../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 true > /dev/null
  grep -q "&series=1" $CLI_LOGFILE
}

@test "Exec sends duration with complete ping" {
  ../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 sleep 1 > /dev/null
  grep -q "&duration=1." $CLI_LOGFILE
}

@test "Exec sends command with run ping" {
  ../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec --no-stdout d3x0c1 $PROJECT_DIR/bin/success.sh xyz > /dev/null
  grep "state=run" $CLI_LOGFILE | grep "&msg=" | grep -q "success.sh+xyz"
}

@test "Exec sends stdout with complete ping" {
  ../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 $PROJECT_DIR/bin/success.sh xyz > /dev/null
  grep "&msg=" $CLI_LOGFILE | grep -q "xyz"
}

@test "Exec does not send stdout when suppressed" {
  ../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec --no-stdout d3x0c1 $PROJECT_DIR/bin/success.sh xyz > /dev/null
  grep "/complete" $CLI_LOGFILE | grep "&msg=" | grep -q "xyz"
}

@test "Exec passes stdout through to caller" {
  ../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 $PROJECT_DIR/bin/success.sh xyz | grep -q xyz
}

@test "Exec passes stdout through to caller with newline chars intact" {
  output="$(../cronitor exec d3x0c1 $PROJECT_DIR/bin/success.sh xyz)"
  output_lines=`echo "${output}" | wc -l | cut -d'/' -f1 | awk '{$1=$1};1'`
  ! [ ${output_lines} -eq "1" ]
}

@test "Exec passes exitcode through to caller" {
  run -123 ../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 $PROJECT_DIR/bin/fail.sh > /dev/null
}
