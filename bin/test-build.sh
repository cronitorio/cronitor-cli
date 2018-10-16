#!/usr/bin/env bash

SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR

if [ "$1" = "--use-dev" ]
    then
        CRONITOR_ARGS="--use-dev"
        HOSTNAME="http://dev.cronitor.io"
    else
        CRONITOR_ARGS=""
        HOSTNAME="https://cronitor.link"
fi

LOGFILE="/tmp/test-build.log"
LOGFILE_ALTERNATE="/tmp/test-build-alternate.log"
CONFIGFILE="/etc/cronitor/cronitor.json"
CONFIGFILE_ALTERNATE="/tmp/test-build-config.json"
ACTUAL_API_KEY="cb54ac4fd16142469f2d84fc1bbebd84"
CRONTAB_TEMP="/tmp/crontab"

# First, build fresh
cd ../
go build && echo "Build created.. OK"
cd - > /dev/null

#################
# CONFIGURE TESTS
#################

echo ""
rm -f $LOGFILE
TEST="Configure uses log file from env var"
CRONITOR_LOG=$LOGFILE ../cronitor $CRONITOR_ARGS ping d3x0c1 --run
if grep -q "Sending ping ${HOSTNAME}/d3x0c1/run" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Configure uses hostname from env var"
CRONITOR_HOSTNAME=myHost ../cronitor $CRONITOR_ARGS ping d3x0c1 --run --log $LOGFILE
if grep -q "&host=myHost" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Configure uses hostname from arg not env var"
CRONITOR_HOSTNAME=myHost ../cronitor $CRONITOR_ARGS ping d3x0c1 --run --log $LOGFILE --hostname otherHost
if grep -q "&host=otherHost" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Configure uses ping api key from env var"
CRONITOR_PING_API_KEY=123 ../cronitor $CRONITOR_ARGS ping d3x0c1 --run --log $LOGFILE
if grep -q "&auth_key=123" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
MSG=`date`
TEST="Configure writes hostname correctly to config file"
../cronitor $CRONITOR_ARGS configure --hostname "$MSG"
if grep "CRONITOR_HOSTNAME" $CONFIGFILE | grep -q "$MSG"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
MSG=`date`
TEST="Configure writes API Key correctly to config file"
../cronitor $CRONITOR_ARGS configure --api-key "$MSG"
if grep "CRONITOR_API_KEY" $CONFIGFILE | grep -q "$MSG"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
rm -f $CONFIGFILE
MSG=`date`
TEST="Configure writes API Key correctly to new config file"
../cronitor $CRONITOR_ARGS configure --api-key "$ACTUAL_API_KEY"  # Using actual API key here so it will be avail for later integration tests..
if grep "CRONITOR_API_KEY" $CONFIGFILE | grep -q "$ACTUAL_API_KEY"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
rm -f $CONFIGFILE_ALTERNATE
MSG=`date`
TEST="Configure writes API Key correctly to custom config file set by param"
../cronitor $CRONITOR_ARGS configure --config $CONFIGFILE_ALTERNATE --api-key "$MSG"
if grep "CRONITOR_API_KEY" $CONFIGFILE_ALTERNATE | grep -q "$MSG"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
rm -f $CONFIGFILE_ALTERNATE
MSG=`date`
TEST="Configure writes API Key correctly to custom config file set by env var"
CRONITOR_CONFIG=$CONFIGFILE_ALTERNATE ../cronitor $CRONITOR_ARGS configure --api-key "$MSG"
if grep "CRONITOR_API_KEY" $CONFIGFILE_ALTERNATE | grep -q "$MSG"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
MSG=`date`
TEST="Configure writes Ping API Key correctly to config file"
../cronitor $CRONITOR_ARGS configure --ping-api-key "$MSG"
if grep "CRONITOR_PING_API_KEY" $CONFIGFILE | grep -q "$MSG"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
rm -f $LOGFILE_ALTERNATE
TEST="Configure writes log path correctly to config file"
../cronitor $CRONITOR_ARGS configure --log $LOGFILE_ALTERNATE
if grep "CRONITOR_LOG" $CONFIGFILE | grep -q $LOGFILE_ALTERNATE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi
rm -f $LOGFILE_ALTERNATE # Remove the log file we just created...


rm -f $LOGFILE
MSG=`date`
TEST="Configure writes exclude text correctly to config file"
../cronitor $CRONITOR_ARGS configure --exclude-from-name "$MSG"
if grep -q "CRONITOR_EXCLUDE_TEXT" $CONFIGFILE && grep -q "$MSG" $CONFIGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
MSG=`date`
TEST="Configure writes multiple exclude text entries correctly to config file"
../cronitor $CRONITOR_ARGS configure --exclude-from-name "${MSG}A" --exclude-from-name "${MSG}B"
if grep -q "CRONITOR_EXCLUDE_TEXT" $CONFIGFILE && grep -q "${MSG}A" $CONFIGFILE && grep -q "${MSG}B" $CONFIGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi


#################
# PING TESTS
#################
echo ""

rm -f $LOGFILE
TEST="Ping without args"
../cronitor $CRONITOR_ARGS ping d3x0c1 --run --log $LOGFILE
if grep -q "Sending ping ${HOSTNAME}/d3x0c1/run" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Ping with custom hostname"
../cronitor $CRONITOR_ARGS ping d3x0c1 --run --hostname customHostnameForTest --log $LOGFILE
if grep -q "Sending ping ${HOSTNAME}/d3x0c1/run" $LOGFILE && grep -q "host=customHostnameForTest" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Ping with message"
MSG="messagewithoutspaces"
../cronitor $CRONITOR_ARGS ping d3x0c1 --run --msg "$MSG" --log $LOGFILE
if grep -q "Sending ping ${HOSTNAME}/d3x0c1/run" $LOGFILE && grep -q "$MSG" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Ping with ping api key"
KEY="XXXXXXXXXX"
../cronitor $CRONITOR_ARGS ping d3x0c1 --run --ping-api-key $KEY --log $LOGFILE
if grep -q "Sending ping ${HOSTNAME}/d3x0c1/run" $LOGFILE && grep -q "auth_key=${KEY}" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Ping integration test"
MSG=`date`
../cronitor $CRONITOR_ARGS ping 44oI2n --run --msg "$MSG" --log $LOGFILE && sleep 1
if ../cronitor $CRONITOR_ARGS activity 44oI2n | grep -q "$MSG"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi


#################
# EXEC TESTS
#################
echo ""

rm -f $LOGFILE
TEST="Exec runs command check"
../cronitor $CRONITOR_ARGS --log $LOGFILE exec d3x0c1 ./write-to-log-success.sh $LOGFILE "$TEST" > /dev/null
if grep -q "$TEST" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec runs command with complex args"
../cronitor $CRONITOR_ARGS --log $LOGFILE exec d3x0c1 ./success.sh "arg with space" anotherArg > /dev/null
if grep -q "arg with space" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi


rm -f $LOGFILE
TEST="Exec sends complete ping on success"
../cronitor $CRONITOR_ARGS --log $LOGFILE exec d3x0c1 true > /dev/null
if grep -q "Sending ping ${HOSTNAME}/d3x0c1/complete" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec sends fail ping on failure"
../cronitor $CRONITOR_ARGS --log $LOGFILE exec d3x0c1 false > /dev/null
if grep -q "Sending ping ${HOSTNAME}/d3x0c1/fail" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec sends status code on complete ping"
../cronitor $CRONITOR_ARGS --log $LOGFILE exec d3x0c1 ./fail.sh > /dev/null
if grep -q "&status_code=123" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec sends run timestamp as complete ping series"
../cronitor $CRONITOR_ARGS --log $LOGFILE exec d3x0c1 true > /dev/null
if grep -q "&series=1" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec sends duration with complete ping"
../cronitor $CRONITOR_ARGS --log $LOGFILE exec d3x0c1 sleep 1 > /dev/null
if grep -q "&duration=1." $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec sends command with run ping"
../cronitor $CRONITOR_ARGS --log $LOGFILE exec --no-stdout d3x0c1 ./success.sh xyz > /dev/null
if grep "/run" $LOGFILE | grep "&msg=" | grep -q "success.sh+xyz"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec sends stdout with complete ping"
../cronitor $CRONITOR_ARGS --log $LOGFILE exec d3x0c1 ./success.sh xyz > /dev/null
if grep "&msg=" $LOGFILE | grep -q "xyz"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec does not send stdout when suppressed"
../cronitor $CRONITOR_ARGS --log $LOGFILE exec --no-stdout d3x0c1 ./success.sh xyz > /dev/null
if grep "/complete" $LOGFILE | grep "&msg=" | grep -q "xyz"
    then echo "${TEST}.. FAIL"
    else echo "${TEST}.. OK"
fi

rm -f $LOGFILE
TEST="Exec passes stdout through to caller"
if ../cronitor $CRONITOR_ARGS --log $LOGFILE exec d3x0c1 ./success.sh xyz | grep -q xyz
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec passes stdout through to caller with newline chars intact"
output="$(../cronitor exec d3x0c1 ./success.sh xyz)"
output_lines=`echo "${output}" | wc -l | cut -d'/' -f1 | awk '{$1=$1};1'`
if [ ${output_lines} -eq "1" ]
    then echo "${TEST}.. FAIL"
    else echo "${TEST}.. OK"
fi

rm -f $LOGFILE
TEST="Exec passes exitcode through to caller"
../cronitor $CRONITOR_ARGS --log $LOGFILE exec d3x0c1 ./fail.sh > /dev/null
if [ $? -eq 123 ]
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi


# Production integration test
rm -f $LOGFILE
TEST="Exec integration test"
MSG=`date`
../cronitor $CRONITOR_ARGS --log $LOGFILE exec 44oI2n echo "$MSG" > /dev/null && sleep 1
if ../cronitor $CRONITOR_ARGS activity 44oI2n | grep -q "$MSG"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi


#################
# STATUS TESTS
#################
echo ""

rm -f $LOGFILE
TEST="Status integration test without filter"
if ../cronitor $CRONITOR_ARGS status --log $LOGFILE | grep -q "Ok"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Status integration test with filter"
if ../cronitor $CRONITOR_ARGS status 44oI2n --log $LOGFILE | grep -q "Ok"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Status integration test with bad monitor code"
if ../cronitor $CRONITOR_ARGS status asdfgh --log $LOGFILE 2>&1 | grep -q "404"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi


#################
# ACTIVITY TESTS
#################
echo ""

rm -f $LOGFILE
TEST="Activity integration test without filter"
if ../cronitor $CRONITOR_ARGS activity 44oI2n --log $LOGFILE | grep -q "monitor_name"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Activity integration test with only pings filter"
if ../cronitor $CRONITOR_ARGS activity 44oI2n --only pings --log $LOGFILE | grep -q "monitor_name"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Activity integration test with only alerts filter"
if ../cronitor $CRONITOR_ARGS activity 44oI2n --only alerts --log $LOGFILE | grep -q -v "\"description\": \"ping\""
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi


#################
# DISCOVER TESTS
#################
echo ""
API_KEY="53b6c114717140cf896899060bcc9d7e"

rm -f $LOGFILE
TEST="Discover reads file and sends PUT"
../cronitor $CRONITOR_ARGS discover --auto ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e --log $LOGFILE > /dev/null
if grep -q "Request" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Discover parses reponse and rewrites crontab"
if ../cronitor $CRONITOR_ARGS discover --auto ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e| grep "slave_status.sh" | grep -q "cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Discover is silent when being run under exec"
if [[ $(../cronitor $CRONITOR_ARGS exec d3x0c1 cronitor $CRONITOR_ARGS discover --auto ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e | wc -c) -ne 0 ]];
    then echo "${TEST}.. FAIL"
    else echo "${TEST}.. OK"
fi


rm -f $LOGFILE
TEST="Discover correctly parses crontab with username"
echo "* * * * * sharter echo 'username parse'" | cat - ../fixtures/crontab.txt > $CRONTAB_TEMP
if ../cronitor $CRONITOR_ARGS discover --auto $CRONTAB_TEMP -k 53b6c114717140cf896899060bcc9d7e| grep "echo '" | grep -q "sharter cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Discover correctly parses crontab with 6 digits"
echo "* * * * * 0 echo 'six dig parse'" | cat - ../fixtures/crontab.txt > $CRONTAB_TEMP
if ../cronitor $CRONITOR_ARGS discover --auto $CRONTAB_TEMP -k 53b6c114717140cf896899060bcc9d7e| grep "echo '" | grep -q "0 cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Discover correctly parses crontab with 6th digit DoW string range"
echo "* * * * * Mon-Fri echo 'DoW string parse'" | cat - ../fixtures/crontab.txt > $CRONTAB_TEMP
if ../cronitor $CRONITOR_ARGS discover --auto $CRONTAB_TEMP -k 53b6c114717140cf896899060bcc9d7e| grep "echo '" | grep -q "Mon-Fri cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Discover correctly parses crontab with 6th digit DoW string list"
echo "* * * * * Mon,Wed,Fri echo 'DoW string list parse'" | cat - ../fixtures/crontab.txt > $CRONTAB_TEMP
if ../cronitor $CRONITOR_ARGS discover --auto $CRONTAB_TEMP -k 53b6c114717140cf896899060bcc9d7e| grep "echo '" | grep -q "Mon,Wed,Fri cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Discover correctly parses crontab with 6th digit DoW string name"
echo "* * * * * Mon echo 'DoW string name parse'" | cat - ../fixtures/crontab.txt > $CRONTAB_TEMP
if ../cronitor $CRONITOR_ARGS discover --auto $CRONTAB_TEMP -k 53b6c114717140cf896899060bcc9d7e| grep "echo '" | grep -q "Mon cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Discover rewrites crontab in place"
TMPFILE="/tmp/crontab.txt"
cp ../fixtures/crontab.txt $TMPFILE
../cronitor $CRONITOR_ARGS discover --auto $TMPFILE -k 53b6c114717140cf896899060bcc9d7e --save > /dev/null
if grep "slave_status.sh" $TMPFILE | grep -q "cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
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
if ../cronitor $CRONITOR_ARGS discover --auto ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e --no-stdout | grep "cronitor exec" | grep -q "no-stdout"
    then echo "${TEST}.. FAIL"  # Note reversed order here...
    else echo "${TEST}.. OK"
fi

TEST="Discover omits 'notifications' if notification-list not specificed"
if ../cronitor $CRONITOR_ARGS discover --auto ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e | grep -q "notifications"
    then echo "${TEST}.. FAIL"  # Note reversed order here...
    else echo "${TEST}.. OK"
fi

TEST="Discover includes custom notification-list"
if ../cronitor $CRONITOR_ARGS discover --auto ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e --notification-list test-list-name | grep -q "test-list-name"
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
