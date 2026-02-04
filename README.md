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

Manage Cronitor resources directly from the command line.

#### Monitors

```bash
cronitor monitor list                                    # List all monitors
cronitor monitor list --type job --state failing         # Filter by type and state
cronitor monitor list --tag critical --env production    # Filter by tag and environment
cronitor monitor list --all                              # Fetch all pages
cronitor monitor search "backup"                         # Search monitors
cronitor monitor get <key>                               # Get monitor details
cronitor monitor get <key> --with-events                 # Include latest events
cronitor monitor create -d '{"key":"my-job","type":"job"}'
cronitor monitor create --file monitors.yaml             # Create from YAML
cronitor monitor update <key> -d '{"name":"New Name"}'
cronitor monitor delete <key>                            # Delete one
cronitor monitor delete key1 key2 key3                   # Delete many
cronitor monitor clone <key> --name "Copy"               # Clone a monitor
cronitor monitor pause <key>                             # Pause indefinitely
cronitor monitor pause <key> --hours 24                  # Pause for 24 hours
cronitor monitor unpause <key>
```

#### Status Pages

```bash
cronitor statuspage list
cronitor statuspage list --with-status                   # Include current status
cronitor statuspage get <key> --with-components          # Include components
cronitor statuspage create -d '{"name":"My Status Page","subdomain":"my-status"}'
cronitor statuspage update <key> -d '{"name":"Updated"}'
cronitor statuspage delete <key>

# Components (nested under statuspage)
cronitor statuspage component list --statuspage my-page
cronitor statuspage component create -d '{"statuspage":"my-page","monitor":"api-health"}'
cronitor statuspage component update <key> -d '{"name":"New Name"}'
cronitor statuspage component delete <key>
```

#### Issues

```bash
cronitor issue list                                      # List all issues
cronitor issue list --state unresolved --severity outage # Filter
cronitor issue list --monitor my-job --time 24h          # By monitor, time range
cronitor issue list --search "database"                  # Search issues
cronitor issue get <key>
cronitor issue create -d '{"name":"DB issues","severity":"outage"}'
cronitor issue update <key> -d '{"state":"investigating"}'
cronitor issue resolve <key>                             # Shorthand for resolving
cronitor issue delete <key>
cronitor issue bulk --action delete --issues KEY1,KEY2   # Bulk actions
```

#### Notifications

```bash
cronitor notification list
cronitor notification get <key>
cronitor notification create -d '{"name":"DevOps","notifications":{"emails":["team@co.com"]}}'
cronitor notification update <key> -d '{"name":"Updated"}'
cronitor notification delete <key>
```

#### Environments

```bash
cronitor environment list
cronitor environment get <key>
cronitor environment create -d '{"key":"staging","name":"Staging"}'
cronitor environment update <key> -d '{"name":"Updated"}'
cronitor environment delete <key>
```

**Aliases:** `cronitor env` → `environment`, `cronitor notifications` → `notification`

### Common Flags

| Flag | Description |
|------|-------------|
| `--format json\|table\|yaml` | Output format (default: `table` for list, `json` for get) |
| `-o, --output <file>` | Write output to a file |
| `--page <n>` | Page number for paginated results |
| `--all` | Fetch all pages of results |
| `-d, --data <json>` | JSON data for create/update |
| `-f, --file <path>` | Read JSON or YAML from a file |
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
