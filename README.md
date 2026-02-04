# CronitorCLI
[![Tests](https://github.com/cronitorio/cronitor-cli/actions/workflows/tests.yml/badge.svg)](https://github.com/cronitorio/cronitor-cli/actions/workflows/tests.yml)

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
cronitor [command]
```

### Cron Management
| Command | Description |
|---------|-------------|
| `cronitor sync` | Sync cron jobs to Cronitor |
| `cronitor exec <key> <cmd>` | Run a command with monitoring |
| `cronitor list` | List all cron jobs |
| `cronitor status` | View monitor status |
| `cronitor dash` | Start the web dashboard |

### API Resources

Manage Cronitor resources directly from the command line. Each resource supports `list`, `get`, `create`, `update`, and `delete` subcommands.

#### Monitors

```bash
cronitor monitor list                                        # List all monitors
cronitor monitor list --page 2 --env production              # Paginate, filter by env
cronitor monitor get <key>                                   # Get a monitor
cronitor monitor get <key> --with-events                     # Include latest events
cronitor monitor create --data '{"key":"my-job","type":"job"}'
cronitor monitor update <key> --data '{"name":"New Name"}'
cronitor monitor delete <key>
cronitor monitor pause <key>                                 # Pause indefinitely
cronitor monitor pause <key> --hours 24                      # Pause for 24 hours
cronitor monitor unpause <key>
```

#### Status Pages

```bash
cronitor statuspage list
cronitor statuspage get <key>
cronitor statuspage create --data '{"name":"My Status Page"}'
cronitor statuspage update <key> --data '{"name":"Updated"}'
cronitor statuspage delete <key>
```

#### Issues

```bash
cronitor issue list                                          # List all issues
cronitor issue list --state open --severity high             # Filter by state/severity
cronitor issue list --monitor my-job                         # Filter by monitor
cronitor issue get <key>
cronitor issue create --data '{"monitor":"my-job","summary":"Issue title"}'
cronitor issue update <key> --data '{"state":"resolved"}'
cronitor issue resolve <key>                                 # Shorthand for resolving
cronitor issue delete <key>
```

#### Notifications

```bash
cronitor notification list
cronitor notification get <key>
cronitor notification create --data '{"name":"DevOps","emails":["team@co.com"]}'
cronitor notification update <key> --data '{"name":"Updated"}'
cronitor notification delete <key>
```

#### Environments

```bash
cronitor environment list
cronitor environment get <key>
cronitor environment create --data '{"name":"Production","key":"production"}'
cronitor environment update <key> --data '{"name":"Updated"}'
cronitor environment delete <key>
```

**Aliases:** `cronitor env` → `environment`, `cronitor notifications` → `notification`

### Common Flags

| Flag | Description |
|------|-------------|
| `--format json\|table` | Output format (default: `table` for list, `json` for get) |
| `-o, --output <file>` | Write output to a file |
| `--page <n>` | Page number for paginated results |
| `-d, --data <json>` | JSON data for create/update |
| `-f, --file <path>` | Read JSON data from a file |
| `-k, --api-key <key>` | Cronitor API key |

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

## MCP Server (AI Integration)

The Cronitor CLI includes a built-in [Model Context Protocol (MCP)](https://modelcontextprotocol.io) server for managing cron jobs with natural language through AI-powered tools like Claude Code, Cursor, Cline, and Windsurf.

**Quick start:** Run `cronitor dash` on your server, then configure your MCP client to spawn `cronitor dash --mcp-instance default`.

For setup instructions, available tools, and configuration options, see the [MCP Integration Guide](docs/MCP_INTEGRATION.md).

## Uninstall CronitorCLI
First, you will need to update any crontab files that were edited to include Cronitor to remove the reference to `cronitor exec MONITOR_KEY` that were added when you created monitors.

Then, remove the cronitor executable from wherever it was installed. If you followed our default instructions it can be removed with `rm /usr/bin/cronitor`
