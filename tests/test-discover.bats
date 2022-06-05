#!/usr/bin/env bats

setup() {
  SCRIPT_DIR="$(dirname $BATS_TEST_FILENAME)"
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

TEST="Discover reads file and sends PUT"
../cronitor $CRONITOR_ARGS discover --auto ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e --log $CLI_LOGFILE > /dev/null
if grep -q "Request" $CLI_LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

TEST="Discover parses reponse and rewrites crontab"
if ../cronitor $CRONITOR_ARGS discover --auto ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e| grep "slave_status.sh" | grep -q "cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi


TEST="Discover is silent when being run under exec"
if [[ $(../cronitor $CRONITOR_ARGS exec d3x0c1 cronitor $CRONITOR_ARGS discover --auto ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e | wc -c) -ne 0 ]];
    then echo "${TEST}.. FAIL"
    else echo "${TEST}.. OK"
fi


TEST="Discover correctly parses crontab with username"
echo "* * * * * $CLI_USERNAME echo 'username parse'" | cat - ../fixtures/crontab.txt > $CLI_CRONTAB_TEMP
if ../cronitor $CRONITOR_ARGS discover --auto $CLI_CRONTAB_TEMP -k 53b6c114717140cf896899060bcc9d7e| grep "echo '" | grep -q "$CLI_USERNAME cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

TEST="Discover correctly parses crontab with 6 digits"
echo "* * * * * 0 echo 'six dig parse'" | cat - ../fixtures/crontab.txt > $CLI_CRONTAB_TEMP
if ../cronitor $CRONITOR_ARGS discover --auto $CLI_CRONTAB_TEMP -k 53b6c114717140cf896899060bcc9d7e| grep "echo '" | grep -q "0 cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

TEST="Discover correctly parses crontab with 6th digit DoW string range"
echo "* * * * * Mon-Fri echo 'DoW string parse'" | cat - ../fixtures/crontab.txt > $CLI_CRONTAB_TEMP
if ../cronitor $CRONITOR_ARGS discover --auto $CLI_CRONTAB_TEMP -k 53b6c114717140cf896899060bcc9d7e| grep "echo '" | grep -q "Mon-Fri cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

TEST="Discover correctly parses crontab with 6th digit DoW string list"
echo "* * * * * Mon,Wed,Fri echo 'DoW string list parse'" | cat - ../fixtures/crontab.txt > $CLI_CRONTAB_TEMP
if ../cronitor $CRONITOR_ARGS discover --auto $CLI_CRONTAB_TEMP -k 53b6c114717140cf896899060bcc9d7e| grep "echo '" | grep -q "Mon,Wed,Fri cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

TEST="Discover correctly parses crontab with 6th digit DoW string name"
echo "* * * * * Mon echo 'DoW string name parse'" | cat - ../fixtures/crontab.txt > $CLI_CRONTAB_TEMP
if ../cronitor $CRONITOR_ARGS discover --auto $CLI_CRONTAB_TEMP -k 53b6c114717140cf896899060bcc9d7e| grep "echo '" | grep -q "Mon cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

TEST="Discover rewrites crontab in place"
TMPFILE="/tmp/crontab.txt"
cp ../fixtures/crontab.txt $TMPFILE
../cronitor $CRONITOR_ARGS discover --auto $TMPFILE -k 53b6c114717140cf896899060bcc9d7e > /dev/null
if grep "slave_status.sh" $TMPFILE | grep -q "cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

TEST="Discover ignores meta crontab entries"
TMPFILE="/tmp/crontab.txt"
cp ../fixtures/metacrontab.txt $TMPFILE
../cronitor $CRONITOR_ARGS discover --auto $TMPFILE -k 53b6c114717140cf896899060bcc9d7e > /dev/null
if grep "cron.hourly" $TMPFILE | grep -q "cronitor exec"
    then echo "${TEST}.. FAIL"
    else echo "${TEST}.. OK"
fi

TEST="Discover adds no-stdout flag when supplied"
if ../cronitor $CRONITOR_ARGS discover --auto -v ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e --no-stdout | grep "cronitor exec" | grep -q "no-stdout"
    then echo "${TEST}.. FAIL"  # Note reversed order here...
    else echo "${TEST}.. OK"
fi

TEST="Discover omits 'notifications' if notification-list not specificed"
if ../cronitor $CRONITOR_ARGS discover --auto -v ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e | grep -q "notifications"
    then echo "${TEST}.. FAIL"  # Note reversed order here...
    else echo "${TEST}.. OK"
fi

TEST="Discover includes custom notification-list"
if ../cronitor $CRONITOR_ARGS discover --auto -v ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e --notification-list test-list-name | grep -q "test-list-name"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

TEST="Discover reads all of the crontabs in a directory"
OUTPUT="$(../cronitor $CRONITOR_ARGS discover --auto ../fixtures/cron.d -k 53b6c114717140cf896899060bcc9d7e)"
if echo "$OUTPUT" | grep -q "every_minute" && echo "$OUTPUT" | grep -q "top_of_hour"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi