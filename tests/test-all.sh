#!/usr/bin/env bash

SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR

# First, build fresh
cd ../
go build && echo "Build created.. OK"
cd - > /dev/null

./test-configure.sh
./test-discover.sh
./test-exec.sh
./test-list.sh
./test-ping.sh
./test-select.sh
./test-shell.sh
./test-status.sh
