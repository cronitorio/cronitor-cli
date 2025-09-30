# CronitorCLI
**Command line tools for Cronitor.io**

CronitorCLI is the recommended companion application to the Cronitor monitoring service.  Use it on your workstation and deploy it to your server for powerful features, including:

* Import and sync all of your cron jobs with Cronitor
* Easily manage your cron jobs with the [web-based control panel](#crontab-guru-dashboard)
* Automatic integration with Cronitor
* Power tools for your cron jobs

## Installation
CronitorCLI is packaged as a single executable for Linux, MacOS and Windows. There is a simple installation script, but all you need to do is download and decompress the app into a location of your choice for easy system-wide use.

For the latest installation details, see https://cronitor.io/docs/using-cronitor-cli#installation

## Usage

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
```

## Crontab Guru Dashboard

The Cronitor CLI bundles the [Crontab Guru Dashboard](https://crontab.guru/dashboard.html), a self‑hosted web UI to manage your cron jobs, including a one‑click “run now” and "suspend", a local console for testing jobs, and a built in MCP server for configuring jobs and checking the health/status of existing ones.

Start locally

```
cronitor dash
# then visit http://localhost:9000
```

Secure access
The dashboard is intended for local or secured access. A simple, safe pattern for remote use is an SSH tunnel:
```
ssh -L 9000:localhost:9000 user@your-server
# now open http://localhost:9000
```

Access control & options
```
# Set login credentials for the dashboard
cronitor configure --dash-username USER --dash-password PASS

# Optionally, restrict which system users' crontabs are loaded
cronitor configure --users user1,user2
```
For systemd and Docker examples, and security best‑practices, see the full [Dashboard documentation](https://crontab.guru/dashboard.html).

## Uninstall CronitorCLI
First, you will need to update any crontab files that were edited to include Cronitor to remove the reference to `cronitor exec MONITOR_KEY` that were added when you created monitors.

Then, remove the cronitor executable from wherever it was installed. If you followed our default instructions it can be removed with `rm /usr/bin/cronitor`
