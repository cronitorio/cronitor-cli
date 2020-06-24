#!/usr/bin/env bash
# This script is used to test exec functionality
echo "CronitorCLI PID $PPID" >> /tmp/clitest
trap "echo HUP" SIGHUP
trap "echo SIG1" SIGUSR1
sleep 20
