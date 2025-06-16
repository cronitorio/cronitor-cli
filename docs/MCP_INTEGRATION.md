# Cronitor MCP Integration

## Overview

Cronitor CLI now includes built-in support for the Model Context Protocol (MCP), enabling seamless integration with Cursor IDE and other LLM-powered development tools. This allows you to manage cron jobs using natural language directly from your code editor.

## Features

- **Natural Language Control**: Create, update, and manage cron jobs using plain English
- **Multi-Instance Support**: Connect to multiple Cronitor dashboard instances (dev, staging, prod)
- **Full CRUD Operations**: Create, read, update, and delete cron jobs
- **Job Execution**: Run jobs immediately from your editor
- **Resource Access**: Browse crontabs and jobs as MCP resources
- **Secure Authentication**: Uses existing Cronitor dashboard credentials

## Installation

The MCP server is included in the standard Cronitor CLI. Just ensure you have the latest version:

```bash
# Download the latest version
curl -s https://cronitor.io/install | sudo bash

# Or build from source
go build -o cronitor
```

## Configuration

### 1. Configure Cronitor Instances

Edit your Cronitor configuration file (`~/.cronitor/cronitor.json`):

```json
{
    "CRONITOR_DASH_USER": "admin",
    "CRONITOR_DASH_PASS": "your-password",
    "mcp_instances": {
        // Note: "default" instance automatically uses credentials from above
        // No need to configure it - just use: cronitor dash --mcp-instance default
        // It will connect to localhost:9000 with your dashboard credentials
        
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

### 2. Configure Cursor IDE

Configure MCP servers in Cursor using one of these methods:

#### Option 1: Global Configuration (All Projects)

Create `~/.cursor/mcp.json`:

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

#### Option 2: Project-Specific Configuration

Create `.cursor/mcp.json` in your project directory:

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

#### Option 3: Using Cursor's UI

You can also configure MCP servers through Cursor's UI:
1. Go to Settings → MCP
2. Click "Add New MCP Server"
3. Fill in the command and arguments

Note: The `--mcp-instance` flag automatically enables MCP mode, so you don't need a separate `--mcp` flag.

## Available MCP Tools

### create_cronjob
Create a new cron job.

**Parameters:**
- `name` (required): Name of the cron job
- `command` (required): Command to execute
- `schedule` (required): Cron expression or human-readable schedule
- `crontab_file`: Target crontab file (default: user:<current_user>)
- `monitored`: Enable Cronitor monitoring (default: false)
- `run_as_user`: User to run the job as (only needed for system crontabs)

**Example prompts:**
- "Create a cron job called 'backup-db' that runs '/scripts/backup.sh' every day at 2 AM"
- "Add a job to clean temp files every hour"

### list_cronjobs
List all cron jobs.

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
- `suspended`: Suspend/resume the job
- `monitored`: Enable/disable monitoring

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

## Usage Examples

### Creating Jobs

**User:** "Create a database backup job that runs every night at 2 AM"

**Cursor will:**
1. Use the `create_cronjob` tool
2. Set appropriate parameters:
   - name: "database-backup"
   - command: (you'll need to specify)
   - schedule: "0 2 * * *"

### Managing Multiple Instances

**With multiple MCP servers configured:**

**User:** "List all jobs on the production server"

**Cursor will:**
1. Use the production instance (`cronitor-prod`)
2. Call `list_cronjobs` to show all jobs

### Complex Schedules

The MCP server understands both cron expressions and human-readable schedules:

- "every 15 minutes" → `*/15 * * * *`
- "every day at noon" → `0 12 * * *`
- "every Monday at 10:30" → `30 10 * * 1`
- "hourly" → `0 * * * *`

## SSH and Remote Access

For remote Cronitor instances accessed via SSH:

### Option 1: SSH Tunnel
```bash
# Create SSH tunnel
ssh -L 9001:localhost:9000 user@remote-server

# Configure instance to use tunneled port
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

### Option 2: Direct SSH in Cursor Config
```json
{
  "mcpServers": {
    "cronitor-remote": {
      "command": "ssh",
      "args": [
        "user@remote-server",
        "cronitor dash --mcp"
      ]
    }
  }
}
```

## Environment Variables

The MCP server respects all standard Cronitor environment variables:

- `CRONITOR_CONFIG`: Path to config file
- `CRONITOR_MCP_ENABLED`: Enable MCP mode
- `CRONITOR_MCP_INSTANCE`: Default instance name
- `CRONITOR_DASH_USER`: Dashboard username
- `CRONITOR_DASH_PASS`: Dashboard password

## Persistent Rules and Instructions

You can configure persistent rules and instructions that will always be used when interacting with the MCP server. See [MCP_PROMPTS.md](MCP_PROMPTS.md) for detailed configuration options including:

- Cursor rules (`.cursorrules` file)
- Project-specific MCP configuration
- Built-in system prompts
- Custom instance prompts

## Troubleshooting

### Test MCP Server
```bash
# Test MCP server startup with default instance
cronitor dash --mcp-instance default

# Test with specific instance
cronitor dash --mcp-instance production

# Legacy format (still supported)
cronitor dash --mcp
```

### Check Configuration
```bash
# View current configuration
cronitor configure
```

### Common Issues

1. **"No Cronitor instance configured"**
   - Ensure your config file has the correct instance settings
   - Check that the instance name matches

2. **"Authentication failed (401 Error)"**
   - For "default" instance: Ensure CRONITOR_DASH_USER and CRONITOR_DASH_PASS are set in your config
   - For other instances: Verify username and password in the mcp_instances section
   - Ensure the dashboard is running and accessible

3. **"CSRF token validation failed (403 Error)"**
   - This is handled automatically by the MCP server
   - If it persists, try restarting the dashboard
   - The MCP server fetches a fresh CSRF token before each state-changing request

4. **"Connection refused"**
   - Check that the dashboard URL is correct
   - Verify the dashboard is actually running (`cronitor dash` in another terminal)
   - Verify network connectivity to the dashboard

## Security Considerations

- MCP servers run with the permissions of the user executing them
- Dashboard credentials are stored in the config file - protect it appropriately
- Use SSH tunnels or VPNs for remote instances
- Consider using separate credentials for different environments

## Support

For issues or questions:
- Check the [Cronitor documentation](https://cronitor.io/docs)
- Review the [MCP specification](https://modelcontextprotocol.io)
- Open an issue on the [Cronitor CLI GitHub repository](https://github.com/cronitorio/cronitor-cli) 