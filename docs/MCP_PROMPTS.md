# MCP Prompts and Rules Configuration

This guide explains how to configure persistent rules and instructions for the Cronitor MCP server.

## Overview

There are several ways to add persistent rules and instructions that will always be used when interacting with the Cronitor MCP server:

1. **Cursor Rules** (`.cursorrules` file)
2. **Project-specific MCP Configuration** (`.cursor/mcp.json`)
3. **Built-in System Prompts** (automatically applied based on instance)
4. **Custom Instance Prompts** (via configuration)

## 1. Cursor Rules (Recommended)

Create a `.cursorrules` file in your project root with MCP-specific rules:

```markdown
# Cronitor MCP Rules

When using Cronitor MCP tools:

## Default Behavior
- Always create jobs in the user's personal crontab unless explicitly specified otherwise
- Use descriptive job names that clearly indicate the purpose, avoid underscores and use other special characters sparingly 
- Disable Enable monitoring by default, but mention it after the job is created and suggest the user turn it on
- When referencing the cronitor executable, by default it is installed systemwide and should just be called "cronitor" in commands. 

## Job Creation Guidelines
- Ask the user if they want to test commands with "run_cronjob_now" after creation
- Prefer explicit paths over relying on PATH environment variable, except for "cronitor"

## Scheduling Best Practices
- Avoid scheduling multiple heavy jobs at the same time
- For daily jobs, prefer running between 2-4 AM local time
- Use random minutes (not :00 or :30) to avoid load spikes

## Instance Selection
- Users will often have multiple connected Cronitor MCP servers
- Use "default" instance for local development
- Always confirm which instance before making changes
- List existing jobs before creating new ones to avoid duplicates
```

## 2. Project-specific MCP Configuration

Add instructions to your `.cursor/mcp.json` file:

```json
{
  "mcpServers": {
    "cronitor": {
      "command": "cronitor",
      "args": ["dash", "--mcp-instance", "default"],
      "description": "Local Cronitor dashboard - Use for development cron jobs"
    },
    "cronitor-production": {
      "command": "cronitor",
      "args": ["dash", "--mcp-instance", "production"],
      "description": "Production Cronitor - CAUTION: Changes affect live system!"
    }
  },
  "instructions": {
    "cronitor": {
      "guidelines": [
        "This is the LOCAL development instance",
        "Test all commands here before deploying to production",
        "Use descriptive job names with project prefix",
        "Always enable monitoring for important jobs"
      ]
    },
    "cronitor-production": {
      "guidelines": [
        "⚠️ PRODUCTION INSTANCE - Changes are immediate!",
        "Always list existing jobs before creating new ones",
        "Backup critical crontabs before making changes",
        "Coordinate with team before modifying shared jobs",
        "Document all changes in the project changelog"
      ]
    }
  }
}
```

## 3. Built-in System Prompts

The MCP server automatically applies instance-specific prompts based on the instance name:

### Default Instance
- Local development environment rules
- Testing guidelines
- Documentation reminders

### Production Instance
- Production safety warnings
- Team coordination requirements
- Monitoring requirements
- Resource usage guidelines

### Staging Instance
- Testing and validation rules
- Production mirroring guidelines
- Alert configuration testing

To see the current system prompt for an instance, use:
```
"Show me the current Cronitor instance information"
```

## 4. Custom Instance Prompts

You can customize prompts for specific instances in your `~/.cronitor/cronitor.json`:

```json
{
  "mcp_instances": {
    "production": {
      "url": "http://localhost:9090",
      "username": "admin",
      "password": "password",
      "system_prompt": "Custom rules for this specific production instance:\n- Always notify #ops-channel before changes\n- No changes during business hours (9 AM - 5 PM EST)\n- All jobs must have error notifications enabled"
    }
  }
}
```

## Best Practices for Rules

1. **Be Specific**: Include concrete examples and patterns
2. **Include Safety Checks**: Add verification steps before destructive operations
3. **Document Conventions**: Specify naming conventions, scheduling patterns
4. **Environment-Specific**: Different rules for dev/staging/production
5. **Team Guidelines**: Include team-specific workflows and approval processes

## Example Workflow with Rules

When you interact with the MCP server, these rules are automatically considered:

```
User: "Create a backup job for the database"

AI Assistant (following rules):
1. Checks current instance (development)
2. Creates job with descriptive name: "mysql-database-backup"
3. Schedules at 3:17 AM (avoiding common times)
4. Includes error handling and logging
5. Enables monitoring by default
6. Tests with run_cronjob_now
```

## Troubleshooting

- Rules in `.cursorrules` apply to all MCP interactions in the project
- Project-specific `.cursor/mcp.json` overrides global settings
- Built-in system prompts are always included
- Use `get_cronitor_instance` to see active rules for current instance 