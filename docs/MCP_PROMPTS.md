# MCP Prompts and Rules Configuration

This guide explains how to configure persistent rules and instructions for the Cronitor MCP server.

## Overview

You can add persistent rules and instructions that guide how your AI tool interacts with the Cronitor MCP server. These rules help ensure consistent behavior, enforce best practices, and add safety guardrails for different environments.

There are several ways to configure rules:

1. **Project Rules Files** (tool-specific rules files in your project)
2. **MCP Server Configuration** (per-server instructions in `mcp.json`)
3. **Built-in System Prompts** (automatically applied based on instance name)
4. **Custom Instance Prompts** (via Cronitor configuration)

## 1. Project Rules Files

Most AI tools support project-specific rules files. Create a rules file in your project root with MCP-specific guidance:

**Common rules file locations:**
- **Claude Code**: `.claude/rules.md` or `CLAUDE.md`
- **Cursor**: `.cursorrules`
- **Other tools**: Check your tool's documentation

**Example rules file content:**

```markdown
# Cronitor MCP Rules

When using Cronitor MCP tools:

## Default Behavior
- Always create jobs in the user's personal crontab unless explicitly specified otherwise
- Use descriptive job names that clearly indicate the purpose
- Monitoring is disabled by default - mention it after job creation and suggest enabling it
- Use full paths for executables (except "cronitor" which is installed system-wide)

## Job Creation Guidelines
- Ask the user if they want to test commands with "run_cronjob_now" after creation
- Prefer explicit paths over relying on PATH environment variable
- For web projects, use curl to invoke endpoints rather than running scripts directly

## Scheduling Best Practices
- Avoid scheduling multiple heavy jobs at the same time
- For daily jobs, prefer running between 2-4 AM local time
- Use random minutes (not :00 or :30) to avoid load spikes on shared infrastructure

## Instance Selection
- Use "default" instance for local development
- Always confirm which instance before making changes to production
- List existing jobs before creating new ones to avoid duplicates

## Safety
- Always use job KEY (not name) for update/delete operations
- Ask for confirmation before destructive operations
- Never modify jobs on production without explicit user confirmation
```

## 2. MCP Server Configuration

You can add per-server instructions in your MCP configuration file. This is useful for adding environment-specific warnings or guidelines.

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
  }
}
```

Some tools also support an `instructions` field for more detailed guidance:

```json
{
  "mcpServers": {
    "cronitor": {
      "command": "cronitor",
      "args": ["dash", "--mcp-instance", "default"]
    },
    "cronitor-production": {
      "command": "cronitor",
      "args": ["dash", "--mcp-instance", "production"]
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
        "PRODUCTION INSTANCE - Changes are immediate!",
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

The MCP server automatically applies instance-specific prompts based on the instance name. These provide sensible defaults for common environments.

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

To see the current system prompt for an instance, ask your AI tool:
```
"Show me the current Cronitor instance information"
```

## 4. Custom Instance Prompts

You can add custom prompts for specific instances in your `~/.cronitor/cronitor.json`:

```json
{
  "mcp_instances": {
    "production": {
      "url": "http://prod-server:9000",
      "username": "admin",
      "password": "password",
      "system_prompt": "Custom rules for this production instance:\n- Always notify #ops-channel before changes\n- No changes during business hours (9 AM - 5 PM EST)\n- All jobs must have error notifications enabled"
    }
  }
}
```

This is useful for organization-specific policies or server-specific requirements.

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
2. Lists existing jobs to avoid duplicates
3. Creates job with descriptive name: "mysql-database-backup"
4. Schedules at 3:17 AM (avoiding common times)
5. Uses full path for backup script
6. Leaves monitoring disabled but mentions it
7. Asks if user wants to test with run_cronjob_now
```

## Rule Priority

When multiple rule sources exist, they're generally applied in this order (highest priority first):

1. Direct user instructions in the current conversation
2. Project rules files (`.cursorrules`, `CLAUDE.md`, etc.)
3. MCP server configuration instructions
4. Custom instance prompts in cronitor.json
5. Built-in system prompts

## Troubleshooting

- Project rules files apply to all MCP interactions in that project
- MCP configuration instructions are per-server
- Use `get_cronitor_instance` to see active rules for the current instance
- If rules aren't being followed, check that your rules file is in the correct location for your AI tool
