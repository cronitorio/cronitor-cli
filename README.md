# CronitorCLI
**Command line tools for Cronitor.io**

CronitorCLI is the recommended companion application to the Cronitor monitoring service.  Use it on your workstation and deploy it to your server for powerful features, including:

* Import and sync all of your cron jobs
* Rich integration with Cronitor
* Power tools for your cron jobs

### Installation
CronitorCLI is packaged as a single executable for Linux, MacOS and Windows. There is no installation program, all you need to do is download and decompress the app into a location of your choice for easy system-wide use.

For the latest installation details, see https://cronitor.io/docs/using-cronitor-cli#installation

### Usage

```
CronitorCLI version 25.2

Command line tools for Cronitor.io. See https://cronitor.io/docs/using-cronitor-cli for details.

Usage:
  cronitor [command]

Available Commands:
  activity    View monitor activity
  configure   Save configuration variables to the config file
  discover    Attach monitoring to new cron jobs and watch for schedule updates
  exec        Execute a command with monitoring
  help        Help about any command
  list        Search for and list all cron jobs
  ping        Send a single ping to the selected monitoring endpoint
  select      Select a cron job to run interactively
  shell       Run commands from a cron-like shell
  status      View monitor status
  update      Update to the latest version

Flags:
  -k, --api-key string        Cronitor API Key
  -c, --config string         Config file
  -h, --help                  help for cronitor
  -n, --hostname string       A unique identifier for this host (default: system hostname)
  -l, --log string            Write debug logs to supplied file
  -p, --ping-api-key string   Ping API Key
  -v, --verbose               Verbose output

Use "cronitor [command] --help" for more information about a command.
```
