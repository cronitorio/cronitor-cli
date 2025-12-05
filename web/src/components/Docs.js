import React, { useState } from 'react';
import { 
  ChevronDownIcon, 
  ChevronRightIcon,
  ClockIcon,
  FolderOpenIcon,
  Cog6ToothIcon,
  ShieldCheckIcon,
  AcademicCapIcon,
  CommandLineIcon,
  CpuChipIcon
} from '@heroicons/react/24/outline';

function CollapsibleSection({ title, icon: Icon, children, defaultOpen = false }) {
  const [isOpen, setIsOpen] = useState(defaultOpen);

  return (
    <div className="border border-gray-200 dark:border-gray-700 rounded-lg mb-4">
      <button
        onClick={() => setIsOpen(!isOpen)}
        className="w-full px-6 py-4 text-left flex items-center justify-between hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors"
      >
        <div className="flex items-center">
          <Icon className="h-5 w-5 text-gray-500 dark:text-gray-400 mr-3" />
          <span className="text-lg font-medium text-gray-900 dark:text-white">{title}</span>
        </div>
        {isOpen ? (
          <ChevronDownIcon className="h-5 w-5 text-gray-500 dark:text-gray-400" />
        ) : (
          <ChevronRightIcon className="h-5 w-5 text-gray-500 dark:text-gray-400" />
        )}
      </button>
      {isOpen && (
        <div className="px-6 pt-6 pb-6 border-t border-gray-200 dark:border-gray-700">
          {children}
        </div>
      )}
    </div>
  );
}

function CodeBlock({ children, language = "bash" }) {
  return (
    <div className="bg-gray-900 dark:bg-gray-800 rounded-lg p-4 my-4 overflow-x-auto">
      <code className="text-sm text-gray-100 dark:text-gray-200 font-mono whitespace-pre-wrap">
        {children}
      </code>
    </div>
  );
}

function Screenshot({ src, alt, caption }) {
  return (
    <div className="my-6">
      <div className="border border-gray-200 dark:border-gray-700 rounded-lg overflow-hidden">
        <img 
          src={src} 
          alt={alt}
          className="w-full h-auto"
        />
      </div>
      {caption && (
        <p className="text-sm text-gray-600 dark:text-gray-400 mt-2 text-center italic">
          {caption}
        </p>
      )}
    </div>
  );
}

export default function Docs() {
  return (
    <div className="max-w-4xl mx-auto">
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-gray-900 dark:text-white mb-4">
          Crontab Guru Dashboard Documentation
        </h1>
        <p className="text-lg text-gray-600 dark:text-gray-400">
          A comprehensive guide to using the Crontab Guru dashboard for managing your cron jobs and crontabs.
        </p>
      </div>

      {/* Getting Started */}
      <CollapsibleSection title="Getting Started" icon={AcademicCapIcon} defaultOpen={true}>
        <div className="space-y-4">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Crontab Guru Dashboard</h3>
          <p className="text-gray-700 dark:text-gray-300">
            Crontab Guru is a self hosted web-based dashboard for managing cron jobs and crontab files. Your jobs are still run by cron, but you are free from the savagery of editing crontab files in vim. From the team that brought you <a href="https://cronitor.io" className="text-blue-600 dark:text-blue-400 hover:underline" target="_blank" rel="noopener noreferrer">Cronitor</a> and <a href="https://crontab.guru" className="text-blue-600 dark:text-blue-400 hover:underline" target="_blank" rel="noopener noreferrer">Crontab.guru</a>.
          </p>
          
          <Screenshot 
            src="/static/screenshots/jobs-dark.png" 
            alt="Crontab Guru Dashboard - Jobs Page"
            caption="The main Jobs page showing active cron jobs and their status"
          />
        </div>
      </CollapsibleSection>

      {/* Jobs */}
      <CollapsibleSection title="Jobs" icon={ClockIcon}>
        <div className="space-y-4">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Managing Cron Jobs</h3>
          
          <Screenshot 
            src="/static/screenshots/jobs-light.png" 
            alt="Jobs Page - Light Mode"
            caption="The Jobs page in light mode showing the clean, readable interface"
          />
          
          <h4 className="text-md font-medium text-gray-900 dark:text-white">Overview of the Jobs Page</h4>
          <p className="text-gray-700 dark:text-gray-300">
            The Jobs page provides a comprehensive view of all your cron jobs, including both active and suspended jobs. 
            Each job displays its current status, schedule, and active execution history.
          </p>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Job Status Indicators</h4>
          <ul className="list-disc list-inside space-y-1 text-gray-700 dark:text-gray-300">
            <li><span className="text-green-600 dark:text-green-400 font-medium">Scheduled:</span> Job will run according to schedule</li>
            <li><span className="text-yellow-600 dark:text-yellow-400 font-medium">Suspended:</span> This job is commented out in your crontab</li>
            <li><span className="text-gray-600 dark:text-gray-400 font-medium">Idle:</span> The job is not currently running</li>
            <li><span className="text-blue-600 dark:text-blue-400 font-medium">Running:</span> One or many instances of this job are currently running.</li>
          </ul>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Integrated Monitoring</h4>
          <p className="text-gray-700 dark:text-gray-300">
            Easy integration with Cronitor to monitor your job execution performance and metrics. When Cronitor is enabled, the dashboard will show:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-700 dark:text-gray-300">
            <li>If the job is Healthy or Failing</li>
            <li>If alerts are disabled</li>
            <li>Link directly to the Cronitor dashboard page for the job</li>
          </ul>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Job Console</h4>
          <p className="text-gray-700 dark:text-gray-300">
            Troubleshoot and test commands easily with the web console. The job console provides:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-700 dark:text-gray-300">
            <li>Live stream of job output during execution</li>
            <li>Easily kill running commands</li>
            <li>Optionally save changes back to your crontab</li>
          </ul>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Job Actions and Controls</h4>
          <p className="text-gray-700 dark:text-gray-300">
            Manage your jobs with these available actions:
          </p>
          <div className="overflow-x-auto mt-4">
            <table className="min-w-full border border-gray-300 dark:border-gray-600">
              <thead className="bg-gray-50 dark:bg-gray-800">
                <tr>
                  <th className="px-4 py-3 text-left text-sm font-medium text-gray-900 dark:text-white border-b border-gray-300 dark:border-gray-600">Action</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-gray-900 dark:text-white border-b border-gray-300 dark:border-gray-600">Description</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-gray-900 dark:text-white border-b border-gray-300 dark:border-gray-600">How to Access</th>
                </tr>
              </thead>
              <tbody className="bg-white dark:bg-gray-900 divide-y divide-gray-200 dark:divide-gray-700">
                <tr className="hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">Run Now</td>
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">Execute a job immediately</td>
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">Click the "Idle"/"Running" indicator to show the Run Now button</td>
                </tr>
                <tr className="hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">Suspend</td>
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">Temporarily pause job execution</td>
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">Click the "Scheduled" indicator to show the Suspend option</td>
                </tr>
                <tr className="hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">Resume</td>
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">Reactivate a suspended job</td>
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">Click the "Suspended" indicator to show the Resume option</td>
                </tr>
                <tr className="hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">Kill</td>
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">Stop a currently running job</td>
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">Click the "Running" indicator to show the Kill option</td>
                </tr>
                <tr className="hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">Edit</td>
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">Modify job parameters and schedule</td>
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">Job fields can be edited inline</td>
                </tr>
                <tr className="hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">Delete</td>
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">Remove a job permanently</td>
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">Click the "Scheduled"/"Suspended" indicator to show the Delete option</td>
                </tr>
              </tbody>
            </table>
          </div>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Schedule Interpretation with Crontab.guru</h4>
          <p className="text-gray-700 dark:text-gray-300">
            The dashboard integrates the same cron expression parser that powers <a href="https://crontab.guru" className="text-blue-600 dark:text-blue-400 hover:underline" target="_blank" rel="noopener noreferrer">crontab.guru</a>. 
            This powers the human-readable schedule descriptions and "Next At" times.
          </p>
          <CodeBlock>
{`# Examples of schedule descriptions:
0 2 * * *           → "At 02:00"
30 14 * * 1-5       → "At 14:30 on every day-of-week from Monday through Friday"
0 */6 * * *         → "At minute 0 past every 6th hour"
15 10 * * SUN       → "At 10:15 on Sunday"`}
          </CodeBlock>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Timezone Handling</h4>
          <p className="text-gray-700 dark:text-gray-300">
          Cron is timezone-aware and schedule descriptions and "Next At" times are always shown in the job's configured timezone. 
          Click "Show More" on any job to see the schedule translated to your local browser timezone.          </p>
          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Monitoring and Troubleshooting Failed Jobs</h4>
          <p className="text-gray-700 dark:text-gray-300">
            By default, the dashboard does not track job failures or collect logs. To use these features, enable monitoring for your jobs. With monitoring enabled, the Cronitor dashboard provides tools to diagnose and resolve issues:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-700 dark:text-gray-300">
            <li>Detailed error messages and stack traces</li>
            <li>Exit codes and logs</li>
            <li>Job performance metrics</li>
          </ul>
        </div>
      </CollapsibleSection>

      {/* Crontabs */}
      <CollapsibleSection title="Crontabs" icon={FolderOpenIcon}>
        <div className="space-y-4">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Crontab File Management</h3>
          
          <Screenshot 
            src="/static/screenshots/crontabs-dark.png" 
            alt="Crontabs Page"
            caption="The Crontabs page for managing and editing crontab files"
          />
          
          <h4 className="text-md font-medium text-gray-900 dark:text-white">Overview of the Crontabs Page</h4>
          <p className="text-gray-700 dark:text-gray-300">
            The Crontabs page allows you to manage crontab files directly through the web interface. 
            You can view, edit, and manage multiple crontab files for different users and systems.
          </p>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Creating New Crontabs</h4>
          <div className="bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg p-4">
            <p className="text-yellow-800 dark:text-yellow-200">
              <strong>Note:</strong> Creating new crontabs is not possible on macOS due to system restrictions. 
            </p>
          </div>
          <p className="text-gray-700 dark:text-gray-300 mt-4">
            On supported systems, and with sufficient permissions, you can create new crontab files by:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-700 dark:text-gray-300">
            <li>Specifying the target user</li>
            <li>Setting timezone information</li>
            <li>Adding initial comments and documentation</li>
            <li>Defining initial cron job entries</li>
          </ul>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Editing Existing Crontabs</h4>
          <p className="text-gray-700 dark:text-gray-300">
            The web-based editor provides syntax highlighting and validation for crontab files:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-700 dark:text-gray-300">
            <li>Real-time syntax checking</li>
            <li>Cron expression validation</li>
            <li>Visual schedule preview</li>
            <li>Automatic backup before changes</li>
          </ul>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Crontab Syntax and Validation</h4>
          <p className="text-gray-700 dark:text-gray-300">
            Standard crontab syntax is supported with additional validation:
          </p>
          <CodeBlock>
{`# Minute Hour Day Month Weekday Command
0 2 * * * /path/to/backup-script.sh
30 14 * * 1-5 /usr/bin/work-reminder`}
          </CodeBlock>
          <p className="text-gray-700 dark:text-gray-300">
            The dashboard validates expressions and provides helpful error messages for invalid syntax.
          </p>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">File Management Operations</h4>
          <ul className="list-disc list-inside space-y-1 text-gray-700 dark:text-gray-300">
            <li>View multiple crontab files simultaneously</li>
            <li>Compare changes before saving</li>
            <li>Export crontabs to backup files</li>
            <li>Import crontabs from backup files</li>
            <li>Bulk operations on multiple files</li>
          </ul>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Backup and Restore Functionality</h4>
          <p className="text-gray-700 dark:text-gray-300">
            Automatic and manual backup features protect your crontab configurations:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-700 dark:text-gray-300">
            <li>Automatic backups before every edit</li>
            <li>Manual backup creation on demand</li>
            <li>Point-in-time restoration</li>
            <li>Backup verification and integrity checks</li>
          </ul>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Crontab Permissions and Ownership</h4>
          <p className="text-gray-700 dark:text-gray-300">
            Manage file permissions and ownership settings:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-700 dark:text-gray-300">
            <li>View current file permissions</li>
            <li>Change file ownership (with appropriate privileges)</li>
            <li>Set access permissions for different users</li>
            <li>Audit permission changes</li>
          </ul>
        </div>
      </CollapsibleSection>

      {/* Settings */}
      <CollapsibleSection title="Settings" icon={Cog6ToothIcon}>
        <div className="space-y-4">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white">System Configuration</h3>
          
          <Screenshot 
            src="/static/screenshots/settings-dark.png" 
            alt="Settings Page"
            caption="The Settings page for configuring system and dashboard options"
          />
          
          <h4 className="text-md font-medium text-gray-900 dark:text-white">Overview of the Settings Page</h4>
          <p className="text-gray-700 dark:text-gray-300">
            The Settings page provides access to all configuration options for the Crontab Guru dashboard and CronitorCLI.
          </p>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">System Configuration Options</h4>
          <ul className="list-disc list-inside space-y-1 text-gray-700 dark:text-gray-300">
            <li><strong>API Keys:</strong> Configure Cronitor API and Ping API keys</li>
            <li><strong>Hostname:</strong> Set custom hostname for job identification</li>
            <li><strong>Environment:</strong> Specify environment (staging, production, etc.)</li>
            <li><strong>Timezone:</strong> Configure system timezone settings</li>
            <li><strong>Logging:</strong> Set debug log file paths and levels</li>
          </ul>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Dashboard Authentication</h4>
          <p className="text-gray-700 dark:text-gray-300">
            Configure dashboard access credentials:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-700 dark:text-gray-300">
            <li>Dashboard username and password</li>
            <li>Session timeout settings</li>
            <li>Password strength requirements</li>
          </ul>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Update Management</h4>
          <p className="text-gray-700 dark:text-gray-300">
            The dashboard can automatically check for and install updates:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-700 dark:text-gray-300">
            <li>Automatic update checking</li>
            <li>One-click update installation</li>
            <li>Update progress monitoring</li>
            <li>Rollback capabilities</li>
          </ul>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Safe Mode</h4>
          <p className="text-gray-700 dark:text-gray-300">
            Running the dashboard in safe mode restricts certain operations for enhanced security:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-700 dark:text-gray-300">
            <li>Prevents cron command modifications</li>
            <li>Disables web-based job console</li>
            <li>Disables adding new jobs from the dashboard</li>
            <li>Crontabs are shown in read-only mode</li>
          </ul>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Data Retention Settings</h4>
          <ul className="list-disc list-inside space-y-1 text-gray-700 dark:text-gray-300">
            <li>Job execution history retention period</li>
            <li>Log file rotation and cleanup</li>
            <li>Backup file retention policies</li>
            <li>Performance metrics storage duration</li>
          </ul>
        </div>
      </CollapsibleSection>

      {/* Cronitor Integration */}
      <CollapsibleSection title="Cronitor Integration" icon={CommandLineIcon}>
        <div className="space-y-4">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Cronitor Integration</h3>
          
          <h4 className="text-md font-medium text-gray-900 dark:text-white">Understanding the CLI Connection</h4>
          <p className="text-gray-700 dark:text-gray-300">
            When the dashboard is running and monitoring is enabled, job names and schedules are synced automatically with Cronitor. There is no need to run "cronitor sync" manually if you also use the dashboard. For job names, this is a 2-way sync: Names changed on Cronitor will show up on this dashboard, and names changed here will be synced with Cronitor. Schedules are only synced one-way: Changes made on Cronitor do not impact your actual job schedule. 
          </p>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Version Management</h4>
          <p className="text-gray-700 dark:text-gray-300">
            Keep your CronitorCLI installation up to date. An "update" button will be shown on the dashboard when a new version is available. To schedule updates you can use the command line:
          </p>
          <CodeBlock>
{`# Update to latest version (via dashboard or CLI)
cronitor update

# Restart your dashboard to apply the update`}
          </CodeBlock>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Troubleshooting Connection Issues</h4>
          <p className="text-gray-700 dark:text-gray-300">
            Common connection problems and solutions:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-700 dark:text-gray-300">
            <li><strong>Port conflicts:</strong> Change default port with <code>--port</code> flag</li>
            <li><strong>Authentication errors:</strong> Reconfigure credentials with <code>`cronitor configure`</code></li>
            <li><strong>Permission issues:</strong> Check file system permissions for config directory</li>
            <li><strong>Network problems:</strong> Verify firewall settings and local network access</li>
          </ul>
        </div>
      </CollapsibleSection>

      {/* MCP Integration */}
      <CollapsibleSection title="MCP Integration (AI/LLM)" icon={CpuChipIcon}>
        <div className="space-y-4">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Model Context Protocol Integration</h3>
          
          <div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg p-4">
            <p className="text-blue-800 dark:text-blue-200">
              <strong>MCP Integration:</strong> Use natural language to manage cron jobs directly from your AI coding assistant (Cursor, Claude Desktop, etc.)
            </p>
          </div>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">What is MCP?</h4>
          <p className="text-gray-700 dark:text-gray-300">
            The Model Context Protocol (MCP) enables AI assistants to interact with external tools and services. 
            With Crontab Guru's MCP integration, you can manage cron jobs using natural language commands directly from your IDE.
          </p>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Starting the MCP Server</h4>
          <p className="text-gray-700 dark:text-gray-300">
            The dashboard exposes the API that's used, but you must run a MCP server locally for a specific dashboard instance. In Cursor, this will be run automatically when you use the tool, you just need to ensure the dashboard instances themselves are running. 
          </p>
          <CodeBlock>
{`# Connect to default instance
cronitor dash --mcp-instance default

# Connect to production instance
cronitor dash --mcp-instance production

# Connect to staging instance
cronitor dash --mcp-instance staging`}
          </CodeBlock>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Configuring MCP Instances</h4>
          <p className="text-gray-700 dark:text-gray-300">
            You can configure multiple dashboard instances in your <code>/etc/cronitor/cronitor.json</code> file:
          </p>
          <CodeBlock language="json">
{`{
  "mcp_instances": {
    // Note: "default" instance automatically uses your existing dashboard credentials
    // No need to specify username/password - they're read from CRONITOR_DASH_USER/PASS
    // If no configuration exists, default connects to localhost:9000
    
    "production": {
      "url": "http://localhost:9090",
      "username": "prod-admin",
      "password": "prod-password"
    },
    "staging": {
      "url": "http://localhost:9089",
      "username": "staging-admin",
      "password": "staging-password"
    }
  }
}`}
          </CodeBlock>

          <div className="bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg p-4 mt-4">
            <p className="text-yellow-800 dark:text-yellow-200">
              <strong>Important:</strong> When connecting to remote servers, ensure your VPN or SSH tunnel is active first. 
              The MCP server runs locally and connects to remote dashboards through these secure connections. See more in the Security section below.
            </p>
          </div>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Configuring Cursor IDE</h4>
          <div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg p-4 mt-4">
            <p className="text-blue-800 dark:text-blue-200">
              <strong>Note:</strong> You can also configure MCP servers through Cursor's UI by going to Settings → MCP and clicking "Add New MCP Server".
            </p>
          </div>
          <p className="text-gray-700 dark:text-gray-300">
            Cursor MCP configuration is loaded from <code>~/.cursor/mcp.json</code>:
          </p>          
          <CodeBlock language="json">
{`{
  "mcpServers": {
    "cronitor": {
      "command": "cronitor",
      "args": ["dash", "--mcp-instance", "default"]
    },
    "cronitor-production": {
      "command": "cronitor",
      "args": ["dash", "--mcp-instance", "production"]
    },
    "cronitor-staging": {
      "command": "cronitor",
      "args": ["dash", "--mcp-instance", "staging"]
    }
  }
}`}
          </CodeBlock>
          

            

          

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">MCP Capabilities</h4>
          <p className="text-gray-700 dark:text-gray-300">
            The Cronitor MCP server provides the following capabilities:
          </p>
          
          <div className="overflow-x-auto mt-4">
            <table className="min-w-full border border-gray-300 dark:border-gray-600">
              <thead className="bg-gray-50 dark:bg-gray-800">
                <tr>
                  <th className="px-4 py-3 text-left text-sm font-medium text-gray-900 dark:text-white border-b border-gray-300 dark:border-gray-600">Capability</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-gray-900 dark:text-white border-b border-gray-300 dark:border-gray-600">Description</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-gray-900 dark:text-white border-b border-gray-300 dark:border-gray-600">Example Prompt</th>
                </tr>
              </thead>
              <tbody className="bg-white dark:bg-gray-900 divide-y divide-gray-200 dark:divide-gray-700">
                <tr className="hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">List Jobs</td>
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">View all cron jobs or filter by criteria</td>
                  <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-400 font-mono">"Show me all cron jobs"<br/>"List jobs that run daily"</td>
                </tr>
                <tr className="hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">Create Job</td>
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">Add new cron jobs with natural language scheduling</td>
                  <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-400 font-mono">"Create a job to backup database every night at 2 AM"<br/>"Add a job that runs cleanup.sh every 6 hours"</td>
                </tr>
                <tr className="hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">Edit Schedule</td>
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">Update job schedules using natural language</td>
                  <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-400 font-mono">"Change the backup job to run every weekday at 3 AM"<br/>"Update cleanup job to run every 15 minutes"</td>
                </tr>
                <tr className="hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">Edit Command</td>
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">Modify job commands and scripts</td>
                  <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-400 font-mono">"Change the backup job to use rsync instead"<br/>"Update cleanup job to delete files older than 7 days"</td>
                </tr>
                <tr className="hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">Enable Monitoring</td>
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">Enable Cronitor monitoring for jobs</td>
                  <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-400 font-mono">"Enable monitoring for the backup job"<br/>"Monitor all production jobs"</td>
                </tr>
                <tr className="hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">Run Now</td>
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">Execute jobs immediately for testing</td>
                  <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-400 font-mono">"Run the backup job now"<br/>"Execute all cleanup jobs immediately"</td>
                </tr>
                <tr className="hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">Delete Job</td>
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">Remove cron jobs</td>
                  <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-400 font-mono">"Delete the old backup job"<br/>"Remove all disabled jobs"</td>
                </tr>
                <tr className="hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">View Crontabs</td>
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">Access crontab file contents</td>
                  <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-400 font-mono">"Show me the root crontab"<br/>"Display all crontab files"</td>
                </tr>
              </tbody>
            </table>
          </div>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Natural Language Scheduling</h4>
          <p className="text-gray-700 dark:text-gray-300">
            The MCP server understands natural language scheduling expressions and converts them to cron syntax:
          </p>
          <div className="overflow-x-auto mt-4">
            <table className="min-w-full border border-gray-300 dark:border-gray-600">
              <thead className="bg-gray-50 dark:bg-gray-800">
                <tr>
                  <th className="px-4 py-3 text-left text-sm font-medium text-gray-900 dark:text-white border-b border-gray-300 dark:border-gray-600">Natural Language</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-gray-900 dark:text-white border-b border-gray-300 dark:border-gray-600">Cron Expression</th>
                </tr>
              </thead>
              <tbody className="bg-white dark:bg-gray-900 divide-y divide-gray-200 dark:divide-gray-700">
                <tr className="hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">"every 15 minutes"</td>
                  <td className="px-4 py-3 text-sm font-mono text-gray-600 dark:text-gray-400">*/15 * * * *</td>
                </tr>
                <tr className="hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">"daily at 2 AM"</td>
                  <td className="px-4 py-3 text-sm font-mono text-gray-600 dark:text-gray-400">0 2 * * *</td>
                </tr>
                <tr className="hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">"every Monday at 9 AM"</td>
                  <td className="px-4 py-3 text-sm font-mono text-gray-600 dark:text-gray-400">0 9 * * 1</td>
                </tr>
                <tr className="hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">"twice a day"</td>
                  <td className="px-4 py-3 text-sm font-mono text-gray-600 dark:text-gray-400">0 0,12 * * *</td>
                </tr>
                <tr className="hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">"weekdays at 6 PM"</td>
                  <td className="px-4 py-3 text-sm font-mono text-gray-600 dark:text-gray-400">0 18 * * 1-5</td>
                </tr>
                <tr className="hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-3 text-sm text-gray-700 dark:text-gray-300">"first day of every month"</td>
                  <td className="px-4 py-3 text-sm font-mono text-gray-600 dark:text-gray-400">0 0 1 * *</td>
                </tr>
              </tbody>
            </table>
          </div>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Example Workflow</h4>
          <p className="text-gray-700 dark:text-gray-300">
            Here's a typical workflow using MCP in Cursor:
          </p>
          <ol className="list-decimal list-inside space-y-2 text-gray-700 dark:text-gray-300">
            <li><strong>List existing jobs:</strong> "Show me all cron jobs on the production server"</li>
            <li><strong>Create a new job:</strong> "Create a job that backs up /var/www to S3 every night at 2 AM"</li>
            <li><strong>Enable monitoring:</strong> "Enable monitoring for the backup job"</li>
            <li><strong>Test the job:</strong> "Run the backup job now to test it"</li>
            <li><strong>Adjust schedule:</strong> "Change the backup job to run at 3 AM instead"</li>
            <li><strong>Monitor status:</strong> "Show me the status of all monitored jobs"</li>
          </ol>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Troubleshooting MCP</h4>
          <p className="text-gray-700 dark:text-gray-300">
            Common issues and solutions:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-700 dark:text-gray-300">
            <li><strong>Connection failed:</strong> Ensure the dashboard is running and accessible at the configured URL</li>
            <li><strong>Authentication error (401):</strong> For "default" instance, ensure CRONITOR_DASH_USER/PASS are set in your config. For other instances, verify credentials in mcp_instances</li>
            <li><strong>CSRF token validation failed (403):</strong> This is handled automatically by the MCP server. If it persists, try restarting the dashboard</li>
            <li><strong>Remote server unreachable:</strong> Verify your VPN connection or SSH tunnel is active before starting MCP</li>
            <li><strong>Commands not working:</strong> Check that the MCP server is running with <code>ps aux | grep "cronitor.*mcp"</code></li>
            <li><strong>Multiple instances:</strong> Use descriptive names (production, staging) to avoid confusion</li>
            <li><strong>Tunnel disconnected:</strong> If SSH tunnel drops, restart it and then restart the MCP server in Cursor</li>
          </ul>

          <div className="bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg p-4 mt-6">
            <p className="text-green-800 dark:text-green-200">
              <strong>Pro Tip:</strong> You can manage multiple servers from a single Cursor instance by configuring multiple MCP servers with different instance names.
            </p>
          </div>
        </div>
      </CollapsibleSection>

      {/* Security */}
      <CollapsibleSection title="Security" icon={ShieldCheckIcon}>
        <div className="space-y-4">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Security Considerations</h3>
          
          <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4">
            <p className="text-red-800 dark:text-red-200">
              <strong>Important:</strong> Public internet access is not recommended. Always use secure tunneling for remote access.
            </p>
          </div>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Use with SSH Tunnel or VPN</h4>
          <p className="text-gray-700 dark:text-gray-300">
            For secure remote access, use SSH tunneling or VPN connections:
          </p>
          <CodeBlock>
{`# SSH tunnel example
ssh -L 9000:localhost:9000 user@remote-cron-server

# Then access via http://localhost:9000`}
          </CodeBlock>

          <p className="text-gray-700 dark:text-gray-300">
            Do this automaticallyby updating your SSH config to create a tunnel every time you connect:
            </p>
          <CodeBlock>
{`# Add this to your ~/.ssh/config
Host remote-cron-server
  LocalForward 9000 localhost:9000`}
          </CodeBlock>

          <p className="text-gray-700 dark:text-gray-300">
            You can also do this with stunnel:
            </p>
          <CodeBlock>
{`# Add this to your ~/.stunnel/stunnel.conf
client = yes
accept = 9000
connect = remote-cron-server:9000`}
            </CodeBlock>   
            <p className="text-gray-700 dark:text-gray-300">
              Then run <code>stunnel ~/.stunnel/stunnel.conf</code> to start the tunnel.
            </p>       
          
          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Safe Mode</h4>
          <p className="text-gray-700 dark:text-gray-300">
            Safe mode provides additional security by restricting some sensitive operations:
          </p>
          <CodeBlock>
{`# Start dashboard in safe mode
cronitor dash --safe-mode`}
          </CodeBlock>
          <ul className="list-disc list-inside space-y-1 text-gray-700 dark:text-gray-300">
            <li>Prevents cron command modifications</li>
            <li>Disables web-based job console</li>
            <li>Disables adding new jobs from the dashboard</li>
            <li>Crontabs are shown in read-only mode</li>
          </ul>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">IP Whitelisting</h4>
          <p className="text-gray-700 dark:text-gray-300">
            For additional security, you can configure IP whitelisting to restrict dashboard access to specific IP addresses or ranges:
          </p>
          <CodeBlock>
{`# Allow access only from specific IPs
cronitor configure --allow-ips 192.168.1.0/24,10.0.0.1

# Allow access from local network only
cronitor configure--allow-ips 127.0.0.1,::1,192.168.0.0/16`}
          </CodeBlock>
          <ul className="list-disc list-inside space-y-1 text-gray-700 dark:text-gray-300">
            <li>Supports both IPv4 and IPv6 addresses</li>
            <li>CIDR notation for network ranges</li>
            <li>Multiple IPs can be specified with comma separation</li>
            <li>Blocked requests are logged for security auditing</li>
          </ul>

          <h4 className="text-md font-medium text-gray-900 dark:text-white mt-6">Security Best Practices</h4>
          <ul className="list-disc list-inside space-y-1 text-gray-700 dark:text-gray-300">
            <li>Use a secure tunnel or VPN for remote access</li>
            <li>Use strong authentication credentials</li>
            <li>Regularly update passwords</li>
            <li>Monitor access logs for suspicious activity</li>
            <li>Keep CronitorCLI updated to latest version</li>
            <li>Use IP Whitelisting to limit access to trusted locations</li>
          </ul>
        </div>
      </CollapsibleSection>
    </div>
  );
} 