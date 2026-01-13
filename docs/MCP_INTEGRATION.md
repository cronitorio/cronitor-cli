# Cronitor MCP Integration

## Overview

Cronitor CLI includes built-in support for the [Model Context Protocol (MCP)](https://modelcontextprotocol.io), enabling integration with AI-powered development tools like Claude Code, Cursor, Cline, Windsurf, and others. Manage cron jobs using natural language directly from your editor or terminal.

## Features

- **Natural Language Control**: Create, update, and manage cron jobs using plain English
- **Multi-Instance Support**: Connect to multiple Cronitor dashboard instances (dev, staging, prod)
- **Full CRUD Operations**: Create, read, update, and delete cron jobs
- **Job Execution**: Run jobs immediately from your editor
- **Resource Access**: Browse crontabs and jobs as MCP resources
- **Secure Authentication**: Uses existing Cronitor dashboard credentials

## How It Works

The MCP integration has two components:

1. **Dashboard** (`cronitor dash`) - The web UI that manages cron jobs. This **must be running on the server where your cron jobs live**.

2. **MCP Server** (`cronitor dash --mcp-instance`) - A bridge process that your AI tool spawns locally. It communicates with your AI tool via stdio and connects to the dashboard over HTTP.

```
┌─────────────────────┐      stdio      ┌─────────────────────┐      HTTP       ┌─────────────────────┐
│   AI Tool           │ ◄─────────────► │   MCP Server        │ ◄─────────────► │   Dashboard         │
│   (Claude Code,     │                 │   (cronitor dash    │                 │   (cronitor dash)   │
│    Cursor, etc.)    │                 │    --mcp-instance)  │                 │   on your server    │
└─────────────────────┘                 └─────────────────────┘                 └─────────────────────┘
      Your machine                           Your machine                           Your server
```

## Quick Start

### Step 1: Start the Dashboard on Your Server

On the machine where your cron jobs run, start the Cronitor dashboard:

```bash
# Set credentials for the dashboard
cronitor configure --dash-username USER --dash-password PASS

# Start the dashboard (runs on port 9000 by default)
cronitor dash
```

**Important:** The dashboard must be running and accessible for the MCP server to work. For production use, consider running it as a systemd service or in Docker.

### Step 2: Configure Your MCP Client

Configure your AI tool to spawn the Cronitor MCP server. Most MCP-compatible tools use a JSON configuration file.

**Generic MCP configuration:**
```json
{
  "mcpServers": {
    "cronitor": {
      "command": "cronitor",
      "args": ["dash", "--mcp-instance", "default"]
    }
  }
}
```

The `default` instance connects to `localhost:9000` using credentials from your Cronitor config file (`~/.cronitor/cronitor.json`).

**Common configuration file locations:**
- **Claude Code**: `~/.claude/mcp.json` or project `.claude/mcp.json`
- **Cursor**: `~/.cursor/mcp.json` or project `.cursor/mcp.json`
- **Other tools**: Check your tool's MCP documentation

### Step 3: Start Using Natural Language

Once configured, you can manage cron jobs with prompts like:
- "Create a database backup job that runs every night at 2 AM"
- "List all cron jobs containing 'backup'"
- "Suspend the cleanup job"
- "Run the backup job now"

## Configuration

### Cronitor Configuration File

Edit `~/.cronitor/cronitor.json` to configure dashboard credentials and additional instances:

```json
{
    "CRONITOR_DASH_USER": "admin",
    "CRONITOR_DASH_PASS": "your-password",
    "mcp_instances": {
        "production": {
            "url": "http://prod-server.example.com:9000",
            "username": "prod-admin",
            "password": "prod-password"
        },
        "staging": {
            "url": "http://staging-server.example.com:9000",
            "username": "staging-admin",
            "password": "staging-password"
        }
    }
}
```

**Note:** The `default` instance automatically uses `CRONITOR_DASH_USER` and `CRONITOR_DASH_PASS` from your config and connects to `localhost:9000`. You don't need to configure it explicitly.

### Multi-Instance Setup

To connect to multiple dashboards, configure each as a separate MCP server:

```json
{
  "mcpServers": {
    "cronitor": {
      "command": "cronitor",
      "args": ["dash", "--mcp-instance", "default"]
    },
    "cronitor-prod": {
      "command": "cronitor",
      "args": ["dash", "--mcp-instance", "production"]
    },
    "cronitor-staging": {
      "command": "cronitor",
      "args": ["dash", "--mcp-instance", "staging"]
    }
  }
}
```

### Project-Specific Configuration

You can override the config file path per-project using an environment variable:

```json
{
  "mcpServers": {
    "cronitor-project": {
      "command": "cronitor",
      "args": ["dash", "--mcp-instance", "default"],
      "env": {
        "CRONITOR_CONFIG": "/path/to/project/cronitor.json"
      }
    }
  }
}
```

## Available MCP Tools

### create_cronjob
Create a new cron job.

**Parameters:**
- `name` (required): Name of the cron job
- `command` (required): Command to execute
- `schedule` (required): Cron expression or human-readable schedule
- `crontab_file`: Target crontab file (default: user's personal crontab)
- `monitored`: Enable Cronitor monitoring (default: false)
- `run_as_user`: User to run the job as (only for system crontabs)

**Example prompts:**
- "Create a cron job called 'backup-db' that runs '/scripts/backup.sh' every day at 2 AM"
- "Add a job to clean temp files every hour"

### list_cronjobs
List all cron jobs with optional filtering.

**Parameters:**
- `filter`: Filter by name or command

**Example prompts:**
- "Show me all cron jobs"
- "List jobs that contain 'backup' in their name"

### update_cronjob
Update an existing cron job.

**Parameters:**
- `key` (required): Job key or identifier
- `name`: New name
- `command`: New command
- `schedule`: New schedule
- `suspended`: Suspend/resume the job (boolean)
- `monitored`: Enable/disable monitoring (boolean)

**Example prompts:**
- "Change the backup job to run every 6 hours"
- "Suspend the cleanup job"

### delete_cronjob
Delete a cron job.

**Parameters:**
- `key` (required): Job key or identifier

**Example prompts:**
- "Delete the old-backup job"

### run_cronjob_now
Execute a cron job immediately.

**Parameters:**
- `key` (required): Job key or identifier

**Example prompts:**
- "Run the backup job now"
- "Execute the cleanup script immediately"

### get_cronitor_instance
Get information about the current Cronitor instance.

**Example prompts:**
- "Which Cronitor instance am I connected to?"

## Available MCP Resources

### cronitor://crontabs
Access all crontab files in JSON format.

### cronitor://jobs
Access all cron jobs in JSON format.

## Human-Readable Schedules

The MCP server understands both cron expressions and natural language schedules:

| Natural Language | Cron Expression |
|-----------------|-----------------|
| `every minute` | `* * * * *` |
| `every hour` | `0 * * * *` |
| `every day` | `0 0 * * *` |
| `every 15 minutes` | `*/15 * * * *` |
| `every day at noon` | `0 12 * * *` |
| `every Monday at 10:30` | `30 10 * * 1` |
| `hourly` | `0 * * * *` |
| `daily` | `0 0 * * *` |
| `weekly` | `0 0 * * 0` |
| `monthly` | `0 0 1 * *` |

## Remote Access

Since the dashboard runs on your server and the MCP server runs locally, you'll need network access to the dashboard. Here are common patterns:

### SSH Tunnel (Recommended)

Create an SSH tunnel to securely access a remote dashboard:

```bash
# Create tunnel from local port 9001 to remote port 9000
ssh -L 9001:localhost:9000 user@your-server
```

Then configure the instance to use the tunneled port:

```json
{
  "mcp_instances": {
    "remote": {
      "url": "http://localhost:9001",
      "username": "admin",
      "password": "password"
    }
  }
}
```

### Direct SSH Transport

Some MCP clients support running commands over SSH directly:

```json
{
  "mcpServers": {
    "cronitor-remote": {
      "command": "ssh",
      "args": [
        "user@your-server",
        "cronitor dash --mcp-instance default"
      ]
    }
  }
}
```

This runs the MCP server on the remote machine and pipes stdio over SSH.

## Environment Variables

The MCP server respects these environment variables:

| Variable | Description |
|----------|-------------|
| `CRONITOR_CONFIG` | Path to config file |
| `CRONITOR_MCP_INSTANCE` | Default instance name |
| `CRONITOR_DASH_USER` | Dashboard username |
| `CRONITOR_DASH_PASS` | Dashboard password |

## Troubleshooting

### Test the MCP Server

```bash
# Test MCP server startup with default instance
cronitor dash --mcp-instance default

# Test with specific instance
cronitor dash --mcp-instance production
```

### Check Configuration

```bash
# View current configuration
cronitor configure
```

### Common Issues

**"No Cronitor instance configured"**
- Ensure your config file has the correct instance settings
- Check that the instance name matches what you configured

**"Authentication failed (401 Error)"**
- For `default` instance: Ensure `CRONITOR_DASH_USER` and `CRONITOR_DASH_PASS` are set in your config
- For other instances: Verify username and password in the `mcp_instances` section
- Ensure the dashboard is running and accessible

**"Connection refused"**
- Check that the dashboard URL is correct
- Verify the dashboard is running: `cronitor dash` in another terminal
- Check network connectivity and any firewalls
- For remote dashboards, ensure your SSH tunnel is active

**"CSRF token validation failed (403 Error)"**
- This is usually handled automatically by the MCP server
- If it persists, try restarting the dashboard

## Security Considerations

- MCP servers run with the permissions of the user executing them
- Dashboard credentials are stored in the config file - protect it with appropriate file permissions
- Use SSH tunnels or VPNs for remote access rather than exposing the dashboard directly
- Consider using separate credentials for different environments

## Persistent Rules and Instructions

You can configure persistent rules that guide how your AI tool interacts with the MCP server. See [MCP_PROMPTS.md](MCP_PROMPTS.md) for details on:

- Project-specific rules files
- Custom system prompts per instance
- Best practices for different environments

## Support

For issues or questions:
- Check the [Cronitor documentation](https://cronitor.io/docs)
- Review the [MCP specification](https://modelcontextprotocol.io)
- Open an issue on the [Cronitor CLI GitHub repository](https://github.com/cronitorio/cronitor-cli)
