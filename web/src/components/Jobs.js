import React from 'react';
import useSWR from 'swr';
import { CheckCircleIcon, XCircleIcon, PencilIcon, ArrowPathIcon } from '@heroicons/react/24/outline';
import guru from '../lib/guru';

const fetcher = url => fetch(url).then(res => res.json());

function Toast({ message, onClose, type = 'error' }) {
  const bgColor = type === 'error' ? 'bg-red-500' : 'bg-green-500';
  return (
    <div className={`fixed bottom-4 left-4 ${bgColor} text-white px-4 py-2 rounded shadow-lg flex items-center space-x-2 z-50`}>
      {type === 'error' ? <XCircleIcon className="h-5 w-5" /> : <CheckCircleIcon className="h-5 w-5" />}
      <span>{message}</span>
      <button onClick={onClose} className="ml-2">
        ×
      </button>
    </div>
  );
}

function StatusIndicator({ job }) {
  const [isMonitored, setIsMonitored] = React.useState(job.is_monitored);
  const [isLoading, setIsLoading] = React.useState(false);

  const handleToggle = async () => {
    setIsLoading(true);
    try {
      const response = await fetch('/api/jobs', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          key: job.key,
          code: job.code,
          name: job.name,
          run_as_user: job.run_as_user,
          expression: job.expression,
          timezone: job.timezone,
          is_monitored: !isMonitored,
        }),
      });

      if (!response.ok) {
        throw new Error('Failed to update job status');
      }

      setIsMonitored(!isMonitored);
    } catch (error) {
      console.error('Error updating job status:', error);
    } finally {
      setIsLoading(false);
    }
  };

  let statusColor = '';
  let statusText = '';

  if (job.disabled) {
    statusColor = 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300';
    statusText = 'Upgrade to activate';
  } else if (!job.initialized) {
    statusColor = 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300';
    statusText = 'Waiting for first run';
  } else if (!job.passing) {
    statusColor = 'bg-red-100 text-red-600 dark:bg-red-900/30 dark:text-red-400';
    statusText = 'Failing';
  } else {
    statusColor = 'bg-green-100 text-green-600 dark:bg-green-900/30 dark:text-green-400';
    statusText = 'Healthy';
  }

  return (
    <div className="flex items-center justify-between space-x-4">
      <div className="flex items-center space-x-2">
        <span className={`text-sm font-medium ${isMonitored ? 'text-green-500' : 'text-red-500'}`}>
          Monitoring: {isMonitored ? 'Enabled' : 'Disabled'}
        </span>
        <button
          onClick={handleToggle}
          disabled={isLoading}
          className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 ${
            isMonitored ? 'bg-green-500' : 'bg-red-500'
          }`}
        >
          <span
            className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
              isMonitored ? 'translate-x-6' : 'translate-x-1'
            }`}
          />
        </button>
      </div>
      {isMonitored && (
        <a
          href={`https://cronitor.io/app/monitors/${job.code}`}
          target="_blank"
          rel="noopener noreferrer"
          className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${statusColor}`}
        >
          {statusText === 'Healthy' && <CheckCircleIcon className="h-4 w-4 mr-1" />}
          {statusText === 'Failing' && <XCircleIcon className="h-4 w-4 mr-1" />}
          <span>{statusText}</span>
        </a>
      )}
    </div>
  );
}

function JobCard({ job }) {
  const [isEditing, setIsEditing] = React.useState(false);
  const [editedName, setEditedName] = React.useState(job.name || job.default_name);
  const [isEditingCommand, setIsEditingCommand] = React.useState(false);
  const [editedCommand, setEditedCommand] = React.useState(job.command);
  const [isEditingSchedule, setIsEditingSchedule] = React.useState(false);
  const [editedSchedule, setEditedSchedule] = React.useState(job.expression || '');
  const [showInstances, setShowInstances] = React.useState(false);
  const [killingPids, setKillingPids] = React.useState(new Set());
  const [isKillingAll, setIsKillingAll] = React.useState(false);
  const [error, setError] = React.useState(null);
  const [isToastVisible, setIsToastVisible] = React.useState(false);
  const [toastMessage, setToastMessage] = React.useState('');
  const [toastType, setToastType] = React.useState('error');
  const inputRef = React.useRef(null);
  const commandInputRef = React.useRef(null);
  const scheduleInputRef = React.useRef(null);
  const { mutate, data: jobs } = useSWR('/api/jobs', fetcher, {
    refreshInterval: 5000, // Refresh every 5 seconds
    revalidateOnFocus: true, // Refresh when tab regains focus
  });
  const [isScheduleValid, setIsScheduleValid] = React.useState(true);

  const showToast = (message, type = 'error') => {
    setToastMessage(message);
    setToastType(type);
    setIsToastVisible(true);
    // Auto-hide toast after 3 seconds
    setTimeout(() => setIsToastVisible(false), 3000);
  };

  // Ensure instances is always an array
  const instances = job.instances || [];

  const handleSave = async () => {
    const originalName = job.name || job.default_name;
    const newName = editedName;
    
    // Show saving toast
    showToast('Saving...', 'success');
    
    // Optimistic update using SWR's mutate
    const optimisticData = jobs.map(j => {
      if (j.key === job.key) {
        return { ...j, name: newName };
      }
      return j;
    });
    
    // Optimistically update the UI
    mutate(optimisticData, false);

    try {
      const response = await fetch('/api/jobs', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          ...job,
          name: newName,
        }),
      });

      if (!response.ok) {
        throw new Error('Failed to update job name');
      }

      setIsEditing(false);
      showToast('Changes saved', 'success');
      // Revalidate to ensure we have the latest data
      mutate();
    } catch (error) {
      // Revert optimistic update on error
      const revertedData = jobs.map(j => {
        if (j.key === job.key) {
          return { ...j, name: originalName };
        }
        return j;
      });
      mutate(revertedData, false);
      showToast('Failed to update job name: ' + error.message);
    }
  };

  const handleCommandSave = async () => {
    const originalCommand = job.command;
    const newCommand = editedCommand;
    
    // Show saving toast
    showToast('Saving...', 'success');
    
    // Optimistic update using SWR's mutate
    const optimisticData = jobs.map(j => {
      if (j.key === job.key) {
        return { ...j, command: newCommand };
      }
      return j;
    });
    
    // Optimistically update the UI
    mutate(optimisticData, false);

    try {
      const response = await fetch('/api/jobs', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          ...job,
          command: newCommand,
        }),
      });

      if (!response.ok) {
        throw new Error('Failed to update job command');
      }

      setIsEditingCommand(false);
      showToast('Changes saved', 'success');
      // Revalidate to ensure we have the latest data
      mutate();
    } catch (error) {
      // Revert optimistic update on error
      const revertedData = jobs.map(j => {
        if (j.key === job.key) {
          return { ...j, command: originalCommand };
        }
        return j;
      });
      mutate(revertedData, false);
      showToast('Failed to update job command: ' + error.message);
    }
  };

  const validateSchedule = (schedule) => {
    // Basic cron expression validation
    // Format: * * * * * or @daily, @hourly, etc.
    const cronRegex = /^(@(annually|yearly|monthly|weekly|daily|hourly|reboot))|(@every (\d+(ns|us|µs|ms|s|m|h))+)|((((\d+,)+\d+|(\d+(\/|-)\d+)|\d+|\*) ?){5,7})$/;
    return cronRegex.test(schedule);
  };

  const handleScheduleSave = async () => {
    if (!validateSchedule(editedSchedule)) {
      setIsScheduleValid(false);
      return;
    }

    const originalSchedule = job.expression;
    const newSchedule = editedSchedule;
    
    // Show saving toast
    showToast('Saving...', 'success');
    
    // Optimistic update using SWR's mutate
    const optimisticData = jobs.map(j => {
      if (j.key === job.key) {
        return { ...j, expression: newSchedule };
      }
      return j;
    });
    
    // Optimistically update the UI
    mutate(optimisticData, false);

    try {
      const response = await fetch('/api/jobs', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          ...job,
          expression: newSchedule,
        }),
      });

      if (!response.ok) {
        throw new Error('Failed to update job schedule');
      }

      setIsEditingSchedule(false);
      setIsScheduleValid(true);
      showToast('Changes saved', 'success');
      // Revalidate to ensure we have the latest data
      mutate();
    } catch (error) {
      // Revert optimistic update on error
      const revertedData = jobs.map(j => {
        if (j.key === job.key) {
          return { ...j, expression: originalSchedule };
        }
        return j;
      });
      mutate(revertedData, false);
      showToast('Failed to update job schedule: ' + error.message);
    }
  };

  const handleKeyDown = (e) => {
    if (e.key === 'Enter') {
      handleSave();
    } else if (e.key === 'Escape') {
      setIsEditing(false);
      setEditedName(job.name || job.default_name);
    }
  };

  const handleCommandKeyDown = (e) => {
    if (e.key === 'Enter') {
      handleCommandSave();
    } else if (e.key === 'Escape') {
      setIsEditingCommand(false);
      setEditedCommand(job.command);
    }
  };

  const handleScheduleKeyDown = (e) => {
    if (e.key === 'Enter') {
      if (validateSchedule(editedSchedule)) {
        handleScheduleSave();
      }
    } else if (e.key === 'Escape') {
      setIsEditingSchedule(false);
      setEditedSchedule(job.expression || '');
      setIsScheduleValid(true);
    }
  };

  const handleScheduleBlur = () => {
    // Only revert if we haven't just saved
    if (!editedSchedule || editedSchedule === job.expression) {
      setIsEditingSchedule(false);
      setEditedSchedule(job.expression || '');
      setIsScheduleValid(true);
    }
  };

  React.useEffect(() => {
    if (isEditing && inputRef.current) {
      inputRef.current.focus();
    }
  }, [isEditing]);

  React.useEffect(() => {
    if (isEditingCommand && commandInputRef.current) {
      commandInputRef.current.focus();
    }
  }, [isEditingCommand]);

  React.useEffect(() => {
    if (isEditingSchedule && scheduleInputRef.current) {
      scheduleInputRef.current.focus();
    }
  }, [isEditingSchedule]);

  // Reset error state when editing starts
  React.useEffect(() => {
    if (isEditing || isEditingCommand || isEditingSchedule) {
      setError(null);
    }
  }, [isEditing, isEditingCommand, isEditingSchedule]);

  React.useEffect(() => {
    if (isEditingSchedule) {
      setIsScheduleValid(validateSchedule(editedSchedule));
    }
  }, [editedSchedule, isEditingSchedule]);

  const handleKill = async (pids, isAll = false) => {
    console.log('Starting kill operation for PIDs:', pids);
    setKillingPids(prev => new Set([...prev, ...pids]));
    if (isAll) {
      setIsKillingAll(true);
    }
    setError(null);
    try {
      const response = await fetch('/api/jobs/kill', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ pids }),
      });

      if (!response.ok) {
        const data = await response.json();
        if (data.errors && data.errors.length > 0) {
          const errorMessages = data.errors.map(e => `PID ${e.pid}: ${e.error}`).join(', ');
          setError(`Failed to kill processes: ${errorMessages}`);
        } else {
          setError('Failed to kill processes');
        }
      } else {
        // Invalidate the cache to trigger a refresh
        mutate();
      }
    } catch (error) {
      setError('Failed to kill processes: ' + error.message);
    } finally {
      // Add a small delay before resetting the state to ensure the loading UI is visible
      setTimeout(() => {
        console.log('Clearing killing state for PIDs:', pids);
        setKillingPids(prev => {
          const newSet = new Set(prev);
          pids.forEach(pid => newSet.delete(pid));
          return newSet;
        });
        if (isAll) {
          setIsKillingAll(false);
        }
      }, 2000);
    }
  };

  const handleRunNow = async (job) => {
    try {
      const response = await fetch(`/api/jobs/${job.key}/run`, {
        method: 'POST',
      });

      if (!response.ok) {
        throw new Error('Failed to run job');
      }

      // Optionally, refresh the job list or update the job state
    } catch (error) {
      console.error('Error running job:', error);
    }
  };

  const getScheduleDescription = (schedule, timezone) => {
    if (!schedule || typeof schedule !== 'string' || !schedule.trim()) {
      return 'Enter a valid cron schedule';
    }
    try {
      return guru(schedule, timezone);
    } catch (error) {
      console.error('Error parsing schedule:', error);
      return 'Invalid schedule format';
    }
  };

  return (
    <div className="bg-white dark:bg-gray-800 shadow rounded-lg p-4 space-y-2 relative">
      {isToastVisible && <Toast message={toastMessage} onClose={() => setIsToastVisible(false)} type={toastType} />}
      {/* Status Tag */}
      <button
        onClick={() => setShowInstances(!showInstances)}
        className={`absolute mt-[14px] right-2 inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium z-10 ${
          instances.length > 0
            ? 'bg-[#4DBEFF] text-gray-900 hover:bg-[#4DBEFF]/90'
            : 'bg-gray-50 text-gray-400 hover:bg-gray-100'
        }`}
      >
        RUNNING: {instances.length > 0 ? instances.length : 'None'}
      </button>

      {/* Line 1: Job Name */}
      <div className="group relative">
        {isEditing ? (
          <input
            ref={inputRef}
            type="text"
            value={editedName}
            onChange={(e) => setEditedName(e.target.value)}
            onKeyDown={handleKeyDown}
            onBlur={() => {
              setIsEditing(false);
              setEditedName(job.name || job.default_name);
            }}
            className="w-full text-lg font-medium text-gray-900 dark:text-white bg-transparent border-b border-gray-300 dark:border-gray-600 focus:outline-none focus:border-blue-500"
          />
        ) : (
          <div className="flex items-center">
            <div className="text-lg font-medium text-gray-900 dark:text-white truncate">
              {job.name || job.default_name}
            </div>
            <button
              onClick={() => setIsEditing(true)}
              className="ml-2 opacity-0 group-hover:opacity-100 transition-opacity"
            >
              <PencilIcon className="h-4 w-4 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200" />
            </button>
          </div>
        )}
      </div>

      {/* Line 2: Command */}
      <div className="group relative">
        {isEditingCommand ? (
          <div className="flex items-center">
            <span className="font-medium text-gray-500 dark:text-gray-400 mr-2">Command:</span>
            <input
              ref={commandInputRef}
              type="text"
              value={editedCommand}
              onChange={(e) => setEditedCommand(e.target.value)}
              onKeyDown={handleCommandKeyDown}
              onBlur={() => {
                setIsEditingCommand(false);
                setEditedCommand(job.command);
              }}
              className="flex-1 text-sm text-gray-600 dark:text-gray-300 font-mono bg-transparent border-b border-gray-300 dark:border-gray-600 focus:outline-none focus:border-blue-500"
            />
          </div>
        ) : (
          <div className="flex items-center">
            <span className="font-medium text-gray-500 dark:text-gray-400 mr-2">Command:</span>
            <div className="text-sm text-gray-600 dark:text-gray-300 font-mono truncate">
              {job.command}
            </div>
            <button
              onClick={() => setIsEditingCommand(true)}
              className="ml-2 opacity-0 group-hover:opacity-100 transition-opacity"
            >
              <PencilIcon className="h-4 w-4 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200" />
            </button>
          </div>
        )}
      </div>

      {/* Line 3: Expression and Description */}
      <div className="group relative">
        {isEditingSchedule ? (
          <div className="space-y-1">
            <div className="flex items-center">
              <span className="font-medium text-gray-500 dark:text-gray-400 mr-2">Schedule:</span>
              <input
                ref={scheduleInputRef}
                type="text"
                value={editedSchedule}
                onChange={(e) => {
                  setEditedSchedule(e.target.value);
                }}
                onKeyDown={handleScheduleKeyDown}
                onBlur={handleScheduleBlur}
                className={`flex-1 text-sm font-mono bg-transparent border-b focus:outline-none ${
                  isScheduleValid 
                    ? 'text-gray-500 dark:text-gray-400 border-gray-300 dark:border-gray-600 focus:border-blue-500' 
                    : 'text-pink-500 dark:text-pink-400 border-pink-300 dark:border-pink-600 focus:border-pink-500'
                }`}
              />
            </div>
            <div className={`text-sm italic ${
              isScheduleValid 
                ? 'text-gray-600 dark:text-gray-300' 
                : 'text-pink-500 dark:text-pink-400'
            }`}>
              {getScheduleDescription(editedSchedule, job.timezone)}
            </div>
          </div>
        ) : (
          <div className="flex flex-col">
            <div className="flex items-center">
              <span className="font-medium text-gray-500 dark:text-gray-400 mr-2">Schedule:</span>
              <span className="text-sm font-mono text-gray-500 dark:text-gray-400">
                {editedSchedule || job.expression}
              </span>
              <button
                onClick={() => {
                  setIsEditingSchedule(true);
                  setEditedSchedule(job.expression || '');
                }}
                className="ml-2 opacity-0 group-hover:opacity-100 transition-opacity"
              >
                <PencilIcon className="h-4 w-4 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200" />
              </button>
            </div>
            <div className="text-sm text-gray-600 dark:text-gray-300 italic mt-1">
              {getScheduleDescription(editedSchedule || job.expression, job.timezone)}
            </div>
          </div>
        )}
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
      </div>

      {/* Instances Table */}
      {showInstances && (
        <div className="mt-2">
          <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
            <thead>
              <tr>
                <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                  PID
                </th>
                <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                  Started
                </th>
                <th className="px-4 py-2 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
              {instances.length > 0 ? (
                instances.map((instance) => (
                  <tr key={instance.pid}>
                    <td className="px-4 py-2 text-sm text-gray-900 dark:text-gray-100">
                      {instance.pid}
                    </td>
                    <td className="px-4 py-2 text-sm text-gray-900 dark:text-gray-100">
                      {instance.started}
                    </td>
                    <td className="px-4 py-2 text-right">
                      <button
                        onClick={() => {
                          console.log('Button clicked for PID:', instance.pid);
                          handleKill([instance.pid]);
                        }}
                        disabled={killingPids.has(instance.pid)}
                        className={`text-xs text-red-600 hover:text-red-800 dark:text-red-400 dark:hover:text-red-300 inline-flex items-center space-x-1 ${
                          killingPids.has(instance.pid) ? 'opacity-30 cursor-not-allowed' : ''
                        }`}
                      >
                        {killingPids.has(instance.pid) ? (
                          <>
                            <ArrowPathIcon className="h-4 w-4 animate-spin" />
                            <span>Killing</span>
                          </>
                        ) : (
                          'Kill Now'
                        )}
                      </button>
                    </td>
                  </tr>
                ))
              ) : (
                <tr>
                  <td colSpan="3" className="px-4 py-2 text-sm text-gray-500 dark:text-gray-400">
                    None
                  </td>
                </tr>
              )}
            </tbody>
          </table>
          {instances.length > 1 && (
            <div className="mt-2 text-right">
              <button
                onClick={() => handleKill(instances.map(i => i.pid), true)}
                disabled={isKillingAll}
                className={`text-xs bg-red-600 hover:bg-red-700 text-white dark:bg-red-500 dark:hover:bg-red-600 px-3 py-1 rounded mr-1 ${
                  isKillingAll ? 'opacity-30 cursor-not-allowed' : ''
                }`}
              >
                {isKillingAll ? (
                  <>
                    <ArrowPathIcon className="h-4 w-4 animate-spin inline-block mr-1" />
                    <span>Killing</span>
                  </>
                ) : (
                  'Kill All'
                )}
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export default function Jobs() {
  const { data: jobs, error } = useSWR('/api/jobs', fetcher, {
    refreshInterval: 5000, // Refresh every 5 seconds
    revalidateOnFocus: true, // Refresh when tab regains focus
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