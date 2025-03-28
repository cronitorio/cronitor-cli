import React from 'react';
import useSWR from 'swr';
import { CheckCircleIcon, XCircleIcon } from '@heroicons/react/24/outline';
import guru from '../lib/guru';

const fetcher = url => fetch(url).then(res => res.json());

function StatusIndicator({ job }) {
  if (!job.is_monitored) {
    return (
      <div className="flex items-center">
        <XCircleIcon className="h-5 w-5 text-red-500 mr-1" />
        <span className="text-red-500">Not Monitored</span>
      </div>
    );
  }

  let statusColor = '';
  let statusText = '';

  if (job.disabled) {
    statusColor = 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300';
    statusText = 'Disabled';
  } else if (!job.initialized) {
    statusColor = 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300';
    statusText = 'Waiting';
  } else if (!job.passing) {
    statusColor = 'bg-red-100 text-red-600 dark:bg-red-900/30 dark:text-red-400';
    statusText = 'Failing';
  } else {
    statusColor = 'bg-green-100 text-green-600 dark:bg-green-900/30 dark:text-green-400';
    statusText = 'Healthy';
  }

  return (
    <div className="flex items-center space-x-2">
      <CheckCircleIcon className="h-5 w-5 text-green-500" />
      <span className="text-green-500">Monitored</span>
      <a
        href={`https://cronitor.io/app/monitors/${job.code}`}
        target="_blank"
        rel="noopener noreferrer"
        className={`px-2.5 py-0.5 rounded-full text-xs font-medium ${statusColor} hover:opacity-80 transition-opacity`}
      >
        {statusText}
      </a>
    </div>
  );
}

function JobCard({ job }) {
  return (
    <div className="bg-white dark:bg-gray-800 shadow rounded-lg p-4 space-y-2">
      {/* Line 1: Job Name */}
      <div className="text-lg font-medium text-gray-900 dark:text-white truncate">
        {job.name}
      </div>

      {/* Line 2: Command */}
      <div className="text-sm text-gray-600 dark:text-gray-300 font-mono truncate">
        <span className="font-medium text-gray-500 dark:text-gray-400 mr-2">Command:</span>
        {job.command}
      </div>

      {/* Line 3: Expression and Description */}
      <div className="text-sm flex items-center">
        <span className="font-medium text-gray-500 dark:text-gray-400 mr-2">Schedule:</span>
        <span className="font-mono text-gray-500 dark:text-gray-400 mr-4">
          {job.expression}
        </span>
        <span className="text-gray-600 dark:text-gray-300 italic">
          {guru(job.expression, job.timezone)}
        </span>
      </div>

      {/* Line 4: Attributes */}
      <div className="grid grid-cols-2 md:grid-cols-3 gap-4 text-sm text-gray-500 dark:text-gray-400">
        <div>
          <span className="font-medium">Run As:</span> {job.run_as_user || 'default'}
        </div>
        <div>
          <span className="font-medium">File:</span> {job.cron_file}
        </div>
        <div>
          <span className="font-medium">Line:</span> {job.line_number}
        </div>
      </div>

      {/* Line 5: Status Indicators */}
      <div className="flex items-center justify-between text-sm">
        <StatusIndicator job={job} />
        <div className="text-gray-500 dark:text-gray-400">
          <span className="font-medium">Timezone:</span> {job.timezone || 'system default'}
        </div>
      </div>
    </div>
  );
}

export default function Jobs() {
  const { data: jobs, error } = useSWR('/api/jobs', fetcher, {
    refreshInterval: 5000, // Refresh every 5 seconds
  });

  if (error) return <div>Failed to load jobs</div>;
  if (!jobs) return <div>Loading...</div>;

  return (
    <div>
      <h1 className="text-2xl font-semibold text-gray-900 dark:text-white mb-6">Jobs</h1>
      <div className="space-y-4">
        {jobs.map((job, index) => (
          <JobCard key={index} job={job} />
        ))}
      </div>
    </div>
  );
} 