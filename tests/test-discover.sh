#!/usr/bin/env bash

SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR

source ./setup.sh

#################
# DISCOVER TESTS
#################

echo ""
API_KEY="53b6c114717140cf896899060bcc9d7e"

rm -f $CLI_LOGFILE
TEST="Discover reads file and sends PUT"
../cronitor $CRONITOR_ARGS discover --auto ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e --log $CLI_LOGFILE > /dev/null
if grep -q "Request" $CLI_LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Discover parses reponse and rewrites crontab"
if ../cronitor $CRONITOR_ARGS discover --auto ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e| grep "slave_status.sh" | grep -q "cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Discover is silent when being run under exec"
if [[ $(../cronitor $CRONITOR_ARGS exec d3x0c1 cronitor $CRONITOR_ARGS discover --auto ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e | wc -c) -ne 0 ]];
    then echo "${TEST}.. FAIL"
    else echo "${TEST}.. OK"
fi


rm -f $CLI_LOGFILE
TEST="Discover correctly parses crontab with username"
echo "* * * * * sharter echo 'username parse'" | cat - ../fixtures/crontab.txt > $CLI_CRONTAB_TEMP
if ../cronitor $CRONITOR_ARGS discover --auto $CLI_CRONTAB_TEMP -k 53b6c114717140cf896899060bcc9d7e| grep "echo '" | grep -q "sharter cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Discover correctly parses crontab with 6 digits"
echo "* * * * * 0 echo 'six dig parse'" | cat - ../fixtures/crontab.txt > $CLI_CRONTAB_TEMP
if ../cronitor $CRONITOR_ARGS discover --auto $CLI_CRONTAB_TEMP -k 53b6c114717140cf896899060bcc9d7e| grep "echo '" | grep -q "0 cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Discover correctly parses crontab with 6th digit DoW string range"
echo "* * * * * Mon-Fri echo 'DoW string parse'" | cat - ../fixtures/crontab.txt > $CLI_CRONTAB_TEMP
if ../cronitor $CRONITOR_ARGS discover --auto $CLI_CRONTAB_TEMP -k 53b6c114717140cf896899060bcc9d7e| grep "echo '" | grep -q "Mon-Fri cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Discover correctly parses crontab with 6th digit DoW string list"
echo "* * * * * Mon,Wed,Fri echo 'DoW string list parse'" | cat - ../fixtures/crontab.txt > $CLI_CRONTAB_TEMP
if ../cronitor $CRONITOR_ARGS discover --auto $CLI_CRONTAB_TEMP -k 53b6c114717140cf896899060bcc9d7e| grep "echo '" | grep -q "Mon,Wed,Fri cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Discover correctly parses crontab with 6th digit DoW string name"
echo "* * * * * Mon echo 'DoW string name parse'" | cat - ../fixtures/crontab.txt > $CLI_CRONTAB_TEMP
if ../cronitor $CRONITOR_ARGS discover --auto $CLI_CRONTAB_TEMP -k 53b6c114717140cf896899060bcc9d7e| grep "echo '" | grep -q "Mon cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Discover rewrites crontab in place"
TMPFILE="/tmp/crontab.txt"
cp ../fixtures/crontab.txt $TMPFILE
../cronitor $CRONITOR_ARGS discover --auto $TMPFILE -k 53b6c114717140cf896899060bcc9d7e > /dev/null
if grep "slave_status.sh" $TMPFILE | grep -q "cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi
rm -f $TMPFILE

rm -f $CLI_LOGFILE

TEST="Discover ignores meta crontab entries"
TMPFILE="/tmp/crontab.txt"
cp ../fixtures/metacrontab.txt $TMPFILE
../cronitor $CRONITOR_ARGS discover --auto $TMPFILE -k 53b6c114717140cf896899060bcc9d7e > /dev/null
if grep "cron.hourly" $TMPFILE | grep -q "cronitor exec"
    then echo "${TEST}.. FAIL"
    else echo "${TEST}.. OK"
fi
rm -f $TMPFILE

TEST="Discover adds auto-discover"
if ../cronitor $CRONITOR_ARGS discover --auto ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e | grep "cronitor" | grep -q "discover"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

TEST="Discover does not add auto-discover when suppressed"
if ../cronitor $CRONITOR_ARGS discover --auto ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e --no-auto-discover | grep -q "cronitor discover"
    then echo "${TEST}.. FAIL"  # Note reversed order here...
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

echo ""
