#!/usr/bin/env bash

SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR

source ./setup.sh

#################
# EXEC TESTS
#################

echo ""

rm -f $CLI_LOGFILE
TEST="Exec uses bash when available"
if [[ "$(../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 ../bin/test-bash.sh)"  == "i am an array" ]]
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Exec runs command check"
../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 ./write-to-log-success.sh $CLI_LOGFILE "$TEST" > /dev/null
if grep -q "$TEST" $CLI_LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Exec runs command with complex args"
../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 ../bin/success.sh "arg with space" anotherArg > /dev/null
if grep -q "arg with space" $CLI_LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Exec runs command with really complex args"
../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 "cd /tmp && pwd" > /dev/null
if grep -q "/tmp" $CLI_LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi


rm -f $CLI_LOGFILE
TEST="Exec sends complete ping on success"
../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 true > /dev/null
if grep "Sending ping" $CLI_LOGFILE | grep -q "state=complete"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Exec sends fail ping on failure"
../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 false > /dev/null
if grep "Sending ping" $CLI_LOGFILE | grep -q "state=fail"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Exec sends status code on complete ping"
../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 ../bin/fail.sh > /dev/null
if grep -q "&status_code=123" $CLI_LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Exec sends environment in pings"
../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE --env test exec d3x0c1 true > /dev/null
if grep -q "&env=test" $CLI_LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi


rm -f $CLI_LOGFILE
TEST="Exec sends run timestamp as complete ping series"
../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 true > /dev/null
if grep -q "&series=1" $CLI_LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Exec sends duration with complete ping"
../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 sleep 1 > /dev/null
if grep -q "&duration=1." $CLI_LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Exec sends command with run ping"
../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec --no-stdout d3x0c1 ../bin/success.sh xyz > /dev/null
if grep "state=run" $CLI_LOGFILE | grep "&msg=" | grep -q "success.sh+xyz"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Exec sends stdout with complete ping"
../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 ../bin/success.sh xyz > /dev/null
if grep "&msg=" $CLI_LOGFILE | grep -q "xyz"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Exec does not send stdout when suppressed"
../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec --no-stdout d3x0c1 ../bin/success.sh xyz > /dev/null
if grep "/complete" $CLI_LOGFILE | grep "&msg=" | grep -q "xyz"
    then echo "${TEST}.. FAIL"
    else echo "${TEST}.. OK"
fi

rm -f $CLI_LOGFILE
TEST="Exec passes stdout through to caller"
if ../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 ../bin/success.sh xyz | grep -q xyz
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Exec passes stdout through to caller with newline chars intact"
output="$(../cronitor exec d3x0c1 ../bin/success.sh xyz)"
output_lines=`echo "${output}" | wc -l | cut -d'/' -f1 | awk '{$1=$1};1'`
if [ ${output_lines} -eq "1" ]
    then echo "${TEST}.. FAIL"
    else echo "${TEST}.. OK"
fi

rm -f $CLI_LOGFILE
TEST="Exec passes exitcode through to caller"
../cronitor $CRONITOR_ARGS --log $CLI_LOGFILE exec d3x0c1 ../bin/fail.sh > /dev/null
if [ $? -eq 123 ]
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi
