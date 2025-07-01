# CronitorCLI
**Command line tools for Cronitor.io**

CronitorCLI is the recommended companion application to the Cronitor monitoring service.  Use it on your workstation and deploy it to your server for powerful features, including:

* Import and sync all of your cron jobs with Cronitor
* Easily manage your cron jobs with the web-based control panel
* Automatic integration with Cronitor
* Power tools for your cron jobs

### Installation
CronitorCLI is packaged as a single executable for Linux, MacOS and Windows. There is a simple installation script, but all you need to do is download and decompress the app into a location of your choice for easy system-wide use.

For the latest installation details, see https://cronitor.io/docs/using-cronitor-cli#installation

### Usage

```
CronitorCLI version 31.4

Command line tools for Cronitor.io. See https://cronitor.io/docs/using-cronitor-cli for details.

Usage:
  cronitor [command]

Available Commands:
  completion  generate the autocompletion script for the specified shell
  configure   Save configuration variables to the config file
  dash        Start the web dashboard
  exec        Execute a command with monitoring
  help        Help about any command
  list        Search for and list all cron jobs
  ping        Send a telemetry ping to Cronitor
  shell       Run commands from a cron-like shell
  signup      Sign up for a Cronitor account
  status      View monitor status
  sync        Add monitoring to new cron jobs and sync changes to existing jobs
  update      Update to the latest version

Flags:
  -k, --api-key string        Cronitor API Key
  -c, --config string         Config file
      --env string            Cronitor Environment
  -h, --help                  help for cronitor
  -n, --hostname string       A unique identifier for this host (default: system hostname)
  -l, --log string            Write debug logs to supplied file
  -p, --ping-api-key string   Ping API Key
  -u, --users string          Comma-separated list of users whose crontabs to include (default: current user only)
  -v, --verbose               Verbose output

Use "cronitor [command] --help" for more information about a command.

### Uninstall CronitorCLI
First, you will need to update any crontab files that were edited to include Cronitor to remove the reference to `cronitor exec MONITOR_KEY` that were added when you created monitors.

Then, remove the cronitor executable from wherever it was installed. If you followed our default instructions it can be removed with `rm /usr/bin/cronitor`
