import React from 'react';
import useSWR from 'swr';
import { CheckCircleIcon, XCircleIcon, PencilIcon, ArrowPathIcon, BellSlashIcon } from '@heroicons/react/24/outline';
import guru, { getNextExecutionTimes } from '../lib/guru';
import cronitorScreenshot from '../assets/cronitor-screenshot.png';

const fetcher = url => fetch(url).then(res => res.json());


function CloseButton({ onClick }) {
  return (
    <button
      onClick={onClick}
      className="absolute top-0 right-8 bg-white dark:bg-gray-800 px-3 py-0 rounded-b-sm border border-t-0 border-gray-300 dark:border-gray-600 text-gray-400 hover:text-gray-500 dark:text-gray-400 dark:hover:text-gray-300 z-10 text-xl leading-none"
    >
      ×
    </button>
  );
}

function Toast({ message, onClose, type = 'error' }) {
  const bgColor = type === 'error' ? 'bg-red-500' : 'bg-green-500';
  return (
    <div className={`fixed bottom-4 left-4 ${bgColor} text-white px-4 py-2 rounded shadow-lg flex items-center space-x-2 z-50`}>
      {type === 'error' ? <XCircleIcon className="h-5 w-5" /> : <CheckCircleIcon className="h-5 w-5" />}
      <span className="text-white dark:text-gray-100">{message}</span>
      <button onClick={onClose} className="ml-2">
        ×
      </button>
    </div>
  );
}

function LearnMoreModal({ onClose }) {
  const modalRef = React.useRef(null);

  React.useEffect(() => {
    const handleEscape = (e) => {
      if (e.key === 'Escape') {
        onClose();
      }
    };

    const handleClickOutside = (e) => {
      if (modalRef.current && !modalRef.current.contains(e.target)) {
        onClose();
      }
    };

    document.addEventListener('keydown', handleEscape);
    document.addEventListener('mousedown', handleClickOutside);

    return () => {
      document.removeEventListener('keydown', handleEscape);
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, [onClose]);

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50" style={{ margin: 0 }}>
      <div ref={modalRef} className="bg-white dark:bg-gray-800 rounded-lg shadow-xl max-w-6xl w-full mx-4 relative">
        <CloseButton onClick={onClose} />
        <div className="p-8">
          <div className="flex">
            <div className="w-2/3 pr-8">
              <h2 className="text-2xl font-black text-gray-900 dark:text-white mb-8">Monitor your jobs with Cronitor</h2>
              <ul className="space-y-6">
                <li className="flex items-start">
                  <CheckCircleIcon className="h-7 w-7 text-green-500 mr-3 flex-shrink-0 mt-0.5" />
                  <span className="text-lg text-gray-700 dark:text-gray-300 leading-relaxed">Instant alerts if a job fails or never starts.</span>
                </li>
                <li className="flex items-start">
                  <CheckCircleIcon className="h-7 w-7 text-green-500 mr-3 flex-shrink-0 mt-0.5" />
                  <span className="text-lg text-gray-700 dark:text-gray-300 leading-relaxed">See the status, metrics and logs from every job.</span>
                </li>
                <li className="flex items-start">
                  <CheckCircleIcon className="h-7 w-7 text-green-500 mr-3 flex-shrink-0 mt-0.5" />
                  <span className="text-lg text-gray-700 dark:text-gray-300 leading-relaxed">Track performance with a full year of data retention.</span>
                </li>
                <li className="flex items-start">
                  <CheckCircleIcon className="h-7 w-7 text-green-500 mr-3 flex-shrink-0 mt-0.5" />
                  <span className="text-lg text-gray-700 dark:text-gray-300 leading-relaxed">Start for free, no credit card required.</span>
                </li>
              </ul>
              <div className="mt-10">
                <a
                  href="https://cronitor.io/cron-job-monitoring?utm_source=cli&utm_campaign=modal&utm_content=1"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center px-6 py-3 border border-transparent text-base font-medium rounded-md shadow-sm text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
                >
                  Learn More
                </a>
              </div>
            </div>
            <div className="w-1/3 overflow-hidden relative">
              <a
                href="https://cronitor.io/cron-job-monitoring?utm_source=cli&utm_campaign=modal&utm_content=1"
                target="_blank"
                rel="noopener noreferrer"
                className="block"
              >
                <img
                  src={cronitorScreenshot}
                  alt="Cronitor Dashboard"
                  className="w-full h-auto"
                  style={{ 
                    objectPosition: 'left center',
                    width: '167%',
                    maxWidth: 'none'
                  }}
                />
              </a>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function StatusIndicator({ job, mutate, allJobs, isNew = false, onFormChange }) {
  const [isLoading, setIsLoading] = React.useState(false);
  const [showLearnMore, setShowLearnMore] = React.useState(false);

  const handleToggle = async () => {
    if (isNew) {
      // For new jobs, just update the form state
      onFormChange({
        ...job,
        is_monitored: !job.is_monitored
      });
      return;
    }

    setIsLoading(true);
    
    // Optimistic update
    const optimisticData = allJobs.map(j => {
      if (j.key === job.key) {
        return { ...j, is_monitored: !job.is_monitored };
      }
      return j;
    });
    mutate(optimisticData, false);

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
          is_monitored: !job.is_monitored,
        }),
      });

      if (!response.ok) {
        throw new Error('Failed to update job status');
      }

      // Revalidate to ensure we have the latest data
      mutate();
    } catch (error) {
      // Revert optimistic update on error
      const revertedData = allJobs.map(j => {
        if (j.key === job.key) {
          return { ...j, is_monitored: job.is_monitored };
        }
        return j;
      });
      mutate(revertedData, false);
      console.error('Error updating job status:', error);
    } finally {
      setIsLoading(false);
    }
  };

  let statusColor = '';
  let statusText = '';
  let statusTitle = '';

  if (job.disabled) {
    statusColor = 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300';
    statusText = 'Upgrade to activate';
    statusTitle = 'You have exceeded your free tier limit. Upgrade to monitor all your jobs.';
  } else if (!!job.code && !job.initialized) {
    statusColor = 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300';
    statusText = 'Waiting';
    statusTitle = 'Monitoring will begin after the next scheduled run.';
  } else if (!!job.code && !job.passing) {
    statusColor = 'bg-red-100 text-red-600 dark:bg-red-900/30 dark:text-red-400';
    statusText = 'Failing';
    statusTitle = 'There is a problem with this job. Check Cronitor for more details.';
  } else if (!!job.code) {
    statusColor = 'bg-green-100 text-green-600 dark:bg-green-900/30 dark:text-green-400';
    statusText = 'Healthy';
    statusTitle = 'This job is running as expected.';
  } else if (!isNew) {
    statusColor = 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300';
    statusText = 'Syncing...';
  }

  const StatusTag = ({ children, ...props }) => {
    if (job.code) {
      return (
        <a
          href={`https://cronitor.io/app/monitors/${job.code}`}
          target="_blank"
          rel="noopener noreferrer"
          {...props}
        >
          {children}
        </a>
      );
    }
    return <div {...props}>{children}</div>;
  };

  return (
    <div className="flex items-center justify-between space-x-4">
      <div className="flex items-center">
        <button
          onClick={handleToggle}
          disabled={isLoading}
          className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 mr-4 ${
            job.is_monitored ? 'bg-green-500' : 'bg-red-500'
          }`}
          title={job.is_monitored ? 'Monitoring enabled' : 'Monitoring disabled'}
        >
          <span
            className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
              job.is_monitored ? 'translate-x-6' : 'translate-x-1'
            }`}
          />
        </button>
        {job.is_monitored ? (
          <>
            {job.paused || job.disabled ? (
              <StatusTag
                className="inline-flex items-center px-2.5 py-0.5 text-sm font-medium bg-gray-100 text-red-600 dark:bg-gray-700 dark:text-red-400 rounded-l-full border-r border-white dark:border-gray-600"
                title="Alerts: Off"
              >
                <BellSlashIcon className="h-5 w-5" />
              </StatusTag>
            ) : null}
            <StatusTag
              className={`inline-flex items-center px-2.5 py-0.5 text-sm font-medium ${statusColor} ${(job.paused || job.disabled) ? 'rounded-l-none' : 'rounded-l-full'} rounded-r-full`}
              title={statusTitle}
            >
              <span>{statusText}</span>
            </StatusTag>
          </>
        ) : (
          <button
            onClick={() => setShowLearnMore(true)}
            className="text-sm text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200"
            title="Learn more about monitoring"
          >
            Learn More
          </button>
        )}
      </div>
      {showLearnMore && <LearnMoreModal onClose={() => setShowLearnMore(false)} />}
    </div>
  );
}

function ConsoleModal({ job, onClose, isNew = false, onFormChange }) {
  const [output, setOutput] = React.useState('');
  const [isRunning, setIsRunning] = React.useState(false);
  const [currentPid, setCurrentPid] = React.useState(null);
  const [command, setCommand] = React.useState(job.command);
  const [hasChanges, setHasChanges] = React.useState(false);
  const [isSaving, setIsSaving] = React.useState(false);
  const outputRef = React.useRef(null);
  const commandInputRef = React.useRef(null);
  const eventSourceRef = React.useRef(null);
  const outputLinesRef = React.useRef([]);

  // Update hasChanges when command changes
  React.useEffect(() => {
    setHasChanges(command !== job.command);
  }, [command, job.command]);

  React.useEffect(() => {
    if (commandInputRef.current) {
      commandInputRef.current.focus();
    }
  }, []);

  React.useEffect(() => {
    if (outputRef.current) {
      outputRef.current.scrollTop = outputRef.current.scrollHeight;
    }
  }, [output]);

  // Cleanup event source on unmount
  React.useEffect(() => {
    return () => {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
      }
    };
  }, []);

  const addOutput = (newOutput) => {
    if (!newOutput) return;

    // Split the new output into lines
    const newLines = newOutput.split('\n');
    
    // Add new lines to our array
    outputLinesRef.current = [...outputLinesRef.current, ...newLines];
    
    // Keep only the last 1000 lines
    if (outputLinesRef.current.length > 1000) {
      outputLinesRef.current = outputLinesRef.current.slice(-1000);
    }
    
    // Update the output state
    setOutput(outputLinesRef.current.join('\n'));
  };

  const runCommand = async () => {
    setIsRunning(true);
    setOutput('');
    outputLinesRef.current = [];
    setCurrentPid(null);

    // Close any existing event source
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
    }

    try {
      const eventSource = new EventSource(`/api/jobs/run?command=${encodeURIComponent(command)}`);
      eventSourceRef.current = eventSource;

      eventSource.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          
          if (data.pid) {
            setCurrentPid(data.pid);
          } else if (data.output) {
            addOutput(data.output);
          } else if (data.error) {
            addOutput(data.error);
          } else if (data.completion) {
            addOutput(data.completion);
            setIsRunning(false);
          }
        } catch (e) {
          console.error('Failed to parse SSE data:', e);
        }
      };

      eventSource.onerror = (error) => {
        console.error('EventSource failed:', error);
        eventSource.close();
        setIsRunning(false);
      };
    } catch (error) {
      addOutput(`Error: ${error.message}\n`);
      setIsRunning(false);
    }
  };

  const handleKill = async () => {
    if (!currentPid) return;

    try {
      await fetch('/api/jobs/kill', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ pids: [currentPid] }), // Send the PID directly, not as a string
      });
      
      // Close the event source
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
      }
      setIsRunning(false);
    } catch (error) {
      console.error('Failed to kill process:', error);
    }
  };

  const handleKeyDown = (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      runCommand();
    }
  };

  const handleCommandBlur = () => {
    if (command === job.command) {
      setHasChanges(false);
    }
  };

  const handleSave = async () => {
    setIsSaving(true);
    try {
      if (isNew) {
        // For new jobs, just update the form state
        onFormChange({
          ...job,
          command: command,
        });
        setHasChanges(false);
      } else {
        const response = await fetch('/api/jobs', {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            ...job,
            command: command,
          }),
        });

        if (!response.ok) {
          throw new Error('Failed to update job command');
        }

        // Update the job in the parent component
        job.command = command;
        setHasChanges(false);
      }
    } catch (error) {
      console.error('Error saving command:', error);
      addOutput(`Error saving command: ${error.message}\n`);
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 rounded-lg shadow-xl w-[calc(100%-2rem)] max-w-6xl mx-4 relative">
        <CloseButton onClick={onClose} />
        <div className="p-4">
          <div className="mb-4">
            <div className="flex items-center space-x-2">
              <span className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Command</span>
            </div>
            <div className="flex items-center space-x-2 mt-1">
              <span className="text-green-500">$</span>
              <input
                ref={commandInputRef}
                type="text"
                value={command}
                onChange={(e) => setCommand(e.target.value)}
                onKeyDown={handleKeyDown}
                onBlur={handleCommandBlur}
                className="w-full text-sm text-gray-900 dark:text-gray-100 font-mono bg-transparent border-b border-gray-300 dark:border-gray-600 focus:outline-none"
                disabled={isRunning}
              />
            </div>
          </div>
          
          <div className="mb-2">
            <span className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Output</span>
          </div>
          <div
            ref={outputRef}
            className="bg-black p-4 rounded font-mono text-sm h-96 overflow-y-auto whitespace-pre-wrap"
          >
            {output}
          </div>
          
          <div className="mt-4 flex justify-end space-x-2">
            {hasChanges && !isRunning && (
              <button
                onClick={handleSave}
                disabled={isSaving}
                className={`px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 font-medium ${
                  isSaving ? 'opacity-50 cursor-not-allowed' : ''
                }`}
              >
                {isSaving ? 'Saving...' : 'Save Changes'}
              </button>
            )}
            {isRunning ? (
              <button
                onClick={handleKill}
                className="px-4 py-2 bg-red-600 text-white rounded hover:bg-red-700 font-medium"
              >
                Kill Now
              </button>
            ) : (
              <button
                onClick={runCommand}
                className="px-4 py-2 bg-green-600 text-white rounded hover:bg-green-700 font-medium"
              >
                Run Command
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function JobCard({ job: initialJob, mutate, allJobs, isNew = false, onSave, onDiscard, onFormChange, showToast }) {
  const [isEditing, setIsEditing] = React.useState(isNew);
  const [isEditingCommand, setIsEditingCommand] = React.useState(isNew);
  const [isEditingSchedule, setIsEditingSchedule] = React.useState(isNew);
  const [editedName, setEditedName] = React.useState(initialJob.name || initialJob.default_name);
  const [editedCommand, setEditedCommand] = React.useState(initialJob.command);
  const [editedSchedule, setEditedSchedule] = React.useState(initialJob.expression || '');
  const [editedCronFile, setEditedCronFile] = React.useState(initialJob.crontab_filename || '');
  const [editedRunAsUser, setEditedRunAsUser] = React.useState(initialJob.run_as_user || '');
  const [isScheduleValid, setIsScheduleValid] = React.useState(true);
  const [cronFiles, setCronFiles] = React.useState([]);
  const [users, setUsers] = React.useState([]);
  const [selectedLocation, setSelectedLocation] = React.useState('');
  const [selectedTimezone, setSelectedTimezone] = React.useState('');
  const [selectedUser, setSelectedUser] = React.useState('');
  const [isUserCrontab, setIsUserCrontab] = React.useState(false);
  const [showInstances, setShowInstances] = React.useState(false);
  const [showNextTimes, setShowNextTimes] = React.useState(false);
  const [nextExecutionTimes, setNextExecutionTimes] = React.useState([]);
  const [killingPids, setKillingPids] = React.useState(new Set());
  const [isKillingAll, setIsKillingAll] = React.useState(false);
  const [error, setError] = React.useState(null);
  const [showSuspendedOverlay, setShowSuspendedOverlay] = React.useState(false);
  const [savingStatus, setSavingStatus] = React.useState(null);
  const [showConsole, setShowConsole] = React.useState(false);
  const [showDeleteConfirmation, setShowDeleteConfirmation] = React.useState(false);
  const [deleteConfirmation, setDeleteConfirmation] = React.useState('');
  const [job, setJob] = React.useState(initialJob);
  const [scheduleDescription, setScheduleDescription] = React.useState('');
  const inputRef = React.useRef(null);
  const commandInputRef = React.useRef(null);
  const scheduleInputRef = React.useRef(null);

  // Get the current job data from allJobs and update local state
  React.useEffect(() => {
    const currentJob = allJobs.find(j => j.key === initialJob.key) || initialJob;
    setJob(currentJob);
  }, [allJobs, initialJob]);

  // Update schedule description when job or editedSchedule changes
  React.useEffect(() => {
    if (job.timezone) {
      const { description, isValid } = getScheduleDescription(editedSchedule || job.expression);
      setScheduleDescription(description);
      setIsScheduleValid(isValid);
    }
  }, [job.timezone, editedSchedule, job.expression]);

  const browserTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const showTimezoneTooltip = job.timezone !== browserTimezone;

  // Ensure instances is always an array
  const instances = job.instances || [];

  const { mutate: jobsMutate, data: jobs } = useSWR('/api/jobs', fetcher, {
    refreshInterval: 5000, // Refresh every 5 seconds
    revalidateOnFocus: true, // Refresh when tab regains focus
  });

  React.useEffect(() => {
    // Fetch cron files and users when component mounts
    const fetchData = async () => {
      try {
        const [cronFilesRes, usersRes] = await Promise.all([
          fetch('/api/crontabs'),
          fetch('/api/users')
        ]);
        
        if (cronFilesRes.ok) {
          const data = await cronFilesRes.json();
          setCronFiles(data);
        }
        
        if (usersRes.ok) {
          const data = await usersRes.json();
          setUsers(data);
        }
      } catch (error) {
        console.error('Error fetching data:', error);
      }
    };

    fetchData();
  }, []);

  const getScheduleDescription = (schedule) => {
    if (!schedule || typeof schedule !== 'string' || !schedule.trim()) {
      return { description: 'Enter a valid cron schedule', isValid: false };
    }
    try {
      const description = guru(schedule, job.timezone);
      return { description, isValid: true };
    } catch (error) {
      console.error('Error parsing schedule:', error);
      return { description: 'Invalid schedule format', isValid: false };
    }
  };

  // Update parent form state when local state changes
  React.useEffect(() => {
    if (isNew && onFormChange) {
      onFormChange({
        name: editedName,
        expression: editedSchedule,
        command: editedCommand,
        crontab_filename: editedCronFile,
        run_as_user: editedRunAsUser,
        is_monitored: job.is_monitored,
        is_draft: true
      });
    }
  }, [isNew, onFormChange, editedName, editedSchedule, editedCommand, editedCronFile, editedRunAsUser, job.is_monitored]);

  // Calculate next execution times when schedule changes
  React.useEffect(() => {
    const calculateTimes = () => {
      if ((isEditingSchedule ? editedSchedule : job.expression) && isScheduleValid) {
        try {
          const nextTimes = getNextExecutionTimes(
            isEditingSchedule ? editedSchedule : job.expression,
            job.timezone
          );
          setNextExecutionTimes(nextTimes);
        } catch (error) {
          console.error('Error calculating next execution times:', error);
          setNextExecutionTimes([]);
        }
      }
    };

    // Calculate immediately
    calculateTimes();

    // Calculate the time until the next minute
    const now = new Date();
    const msUntilNextMinute = (60 - now.getSeconds()) * 1000 - now.getMilliseconds();

    // Set initial timeout to align with the next minute
    const initialTimeout = setTimeout(() => {
      calculateTimes();
      // Then set up interval for every minute
      const interval = setInterval(calculateTimes, 60000);
      return () => clearInterval(interval);
    }, msUntilNextMinute);

    // Clean up timeout and interval on unmount or when dependencies change
    return () => {
      clearTimeout(initialTimeout);
    };
  }, [job.expression, editedSchedule, isEditingSchedule, isScheduleValid, job.timezone]);

  // Initialize timezone when component mounts or when cronFiles changes
  React.useEffect(() => {
    if (isNew && cronFiles.length > 0) {
      const defaultCronFile = cronFiles[0];
      const timezone = defaultCronFile.timezone || 'UTC';
      setSelectedTimezone(timezone);
      setJob(prev => ({ ...prev, timezone }));
      onFormChange(prev => ({
        ...prev,
        timezone
      }));
    }
  }, [isNew, cronFiles]);

  // Update job state when timezone changes
  React.useEffect(() => {
    if (selectedTimezone) {
      setJob(prev => ({ ...prev, timezone: selectedTimezone }));
      onFormChange(prev => ({
        ...prev,
        timezone: selectedTimezone
      }));
    }
  }, [selectedTimezone]);

  const handleSave = async () => {
    const originalName = job.name || job.default_name;
    const newName = editedName;
    const newCommand = editedCommand;
    const newSchedule = editedSchedule;
    
    setSavingStatus('saving');
    
    // Optimistic update using SWR's mutate
    const optimisticData = jobs.map(j => {
      if (j.key === job.key) {
        return { 
          ...j, 
          name: isEditing ? newName : j.name,
          command: isEditingCommand ? newCommand : j.command,
          expression: isEditingSchedule ? newSchedule : j.expression
        };
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
          name: isEditing ? newName : job.name,
          command: isEditingCommand ? newCommand : job.command,
          expression: isEditingSchedule ? newSchedule : job.expression,
        }),
      });

      if (!response.ok) {
        throw new Error('Failed to update job');
      }

      // Set saved state before updating editing state
      setSavingStatus('saved');
      // Wait a moment before clearing editing state
      setTimeout(() => {
        if (isEditing) setIsEditing(false);
        if (isEditingCommand) setIsEditingCommand(false);
        if (isEditingSchedule) setIsEditingSchedule(false);
      }, 100);
      // Revalidate to ensure we have the latest data
      mutate();
    } catch (error) {
      // Revert optimistic update on error
      const revertedData = jobs.map(j => {
        if (j.key === job.key) {
          return { 
            ...j, 
            name: originalName,
            command: job.command,
            expression: job.expression
          };
        }
        return j;
      });
      mutate(revertedData, false);
      setSavingStatus(null);
      showToast('Failed to update job: ' + error.message);
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
    
    setSavingStatus('saving');
    
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

      // Set saved state before updating editing state
      setSavingStatus('saved');
      // Wait a moment before clearing editing state
      setTimeout(() => {
        setIsEditingSchedule(false);
        setIsScheduleValid(true);
      }, 100);
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
      setSavingStatus(null);
      showToast('Failed to update job schedule: ' + error.message);
    }
  };

  // Add effect to handle the fade out of the saved status
  React.useEffect(() => {
    let timer;
    if (savingStatus === 'saved') {
      timer = setTimeout(() => {
        setSavingStatus(null);
      }, 4000);
    }
    return () => {
      if (timer) clearTimeout(timer);
    };
  }, [savingStatus]);

  const handleKeyDown = (e) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      handleSave();
    } else if (e.key === 'Escape') {
      setIsEditing(false);
      setEditedName(job.name || job.default_name);
    }
  };

  const handleCommandKeyDown = (e) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      handleSave();
    } else if (e.key === 'Escape') {
      setIsEditingCommand(false);
      setEditedCommand(job.command);
    }
  };

  const handleScheduleKeyDown = (e) => {
    if (e.key === 'Enter') {
      e.preventDefault();
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
    if (editedSchedule === job.expression) {
      setIsEditingSchedule(false);
    }
  };

  const handleNameBlur = () => {
    if (editedName === (job.name || job.default_name)) {
      setIsEditing(false);
    }
  };

  const handleCommandBlur = () => {
    if (editedCommand === job.command) {
      setIsEditingCommand(false);
    }
  };

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

  const handleDiscard = () => {
    // Reset all local state
    setEditedName('');
    setEditedCommand('');
    setEditedSchedule('');
    setEditedCronFile('');
    setEditedRunAsUser('');
    setIsEditing(false);
    setIsEditingCommand(false);
    setIsEditingSchedule(false);
    setShowConsole(false);
    setShowSuspendedOverlay(false);
    setSavingStatus(null);
    
    // Reset the parent form state
    onFormChange({
      name: '',
      expression: '',
      command: '',
      crontab_filename: '',
      run_as_user: '',
      is_monitored: false,
      is_draft: true
    });
    
    // Call the parent's discard handler
    onDiscard();
  };

  const handleLocationChange = (e) => {
    const location = e.target.value;
    setSelectedLocation(location);
    setEditedCronFile(location);
    
    // Special case for /etc/cron.d (New Crontab)
    if (location === "/etc/cron.d") {
      setIsUserCrontab(false);
      setSelectedUser('');
      // Set default timezone for new crontab
      const timezone = 'UTC';
      setSelectedTimezone(timezone);
      setJob(prev => ({ ...prev, timezone }));
      onFormChange(prev => ({
        ...prev,
        timezone,
        crontab_filename: location
      }));
      return;
    }
    
    // Find the crontab info for the selected location
    const selectedCronFile = cronFiles.find(file => file.filename === location);
    console.log('Selected crontab:', selectedCronFile); // Debug log
    
    if (selectedCronFile) {
      setIsUserCrontab(selectedCronFile.isUserCrontab);
      // Reset user selection when location changes
      setSelectedUser('');
      
      // Set timezone and cronFile from the selected crontab
      const timezone = selectedCronFile.timezone || 'UTC';
      console.log('Setting timezone to:', timezone); // Debug log
      setSelectedTimezone(timezone);
      setJob(prev => ({ ...prev, timezone }));

      onFormChange(prev => ({
        ...prev,
        timezone,
        crontab_filename: location
      }));
    }
  };

  const handleUserChange = (e) => {
    const user = e.target.value;
    setSelectedUser(user);
    setEditedRunAsUser(user);
    setJob(prev => ({ ...prev, run_as_user: user }));
    onFormChange(prev => ({
      ...prev,
      run_as_user: user
    }));
  };

  // Add debug logging for form state
  React.useEffect(() => {
    console.log('Form state:', {
      editedName,
      editedSchedule,
      editedCommand,
      editedCronFile,
      editedRunAsUser,
      isUserCrontab,
      selectedLocation
    });
  }, [editedName, editedSchedule, editedCommand, editedCronFile, editedRunAsUser, isUserCrontab, selectedLocation]);

  return (
    <div className={`bg-white dark:bg-gray-800 shadow rounded-lg p-4 relative ${job.suspended ? 'bg-gray-100 dark:bg-gray-700' : ''}`}>
      {/* Status Indicators */}
      <div className="absolute top-0 right-0 flex items-center">
        {isNew ? (
          <div className="inline-flex items-center px-2.5 py-0.5 rounded-tr-lg rounded-bl-lg text-sm font-medium bg-yellow-100 dark:bg-yellow-900/30 text-yellow-700 dark:text-yellow-300 z-20">
            DRAFT
          </div>
        ) : (
          <>
            {job.suspended ? (
              <div 
                onClick={() => setShowSuspendedOverlay(prev => !prev)}
                className="inline-flex items-center px-2.5 py-0.5 rounded-bl-lg text-sm font-medium bg-pink-100 dark:bg-pink-900/30 text-pink-700 dark:text-pink-300 cursor-pointer hover:bg-pink-200 dark:hover:bg-pink-800/30 border-r border-white dark:border-gray-600 z-20"
              >
                SUSPENDED
              </div>
            ) : (
              <div 
                onClick={() => setShowSuspendedOverlay(prev => !prev)}
                className="inline-flex items-center px-2.5 py-0.5 rounded-bl-lg text-sm font-medium bg-gray-200 dark:bg-gray-700 text-gray-600 dark:text-gray-300 cursor-pointer hover:bg-gray-300 dark:hover:bg-gray-600 border-r border-white dark:border-gray-600 z-20"
              >
                SCHEDULED
              </div>
            )}
            <button
              onClick={() => setShowInstances(!showInstances)}
              title={instances.length > 0 ? `${instances.length} instances of this job are running` : 'Job is not currently running'}
              className={`inline-flex items-center px-2.5 py-0.5 rounded-tr-lg text-sm font-medium ${
                instances.length > 0
                  ? 'bg-blue-400 text-white hover:bg-blue-400/90'
                  : 'bg-gray-200 dark:bg-gray-700 text-gray-600 dark:text-gray-300 hover:bg-gray-300 dark:hover:bg-gray-600'
              } z-20`}
            >
              {instances.length > 0 ? `RUNNING: ${instances.length}` : 'IDLE'}
            </button>
          </>
        )}
      </div>

      {/* Editing Indicator */}
      {!isNew && (isEditing || isEditingCommand || isEditingSchedule || savingStatus === 'saved') && (
        <div className="absolute top-0 left-0 right-0 flex items-center justify-center">
          <div 
            className={`inline-flex items-center px-2.5 py-0.5 rounded-b-lg text-sm font-medium z-20 ${
              savingStatus === 'saving' 
                ? 'bg-amber-400 text-white' 
                : savingStatus === 'saved'
                ? 'bg-green-400 text-white'
                : 'bg-green-400 text-green-900'
            } ${savingStatus === 'saved' ? 'pointer-events-none' : 'cursor-pointer'}`}
            onClick={() => {
              if (savingStatus === 'saved') return;
              if (isEditing) handleSave();
              else if (isEditingCommand) handleSave();
              else if (isEditingSchedule) handleScheduleSave();
            }}
          >
            {savingStatus === 'saving' ? 'Saving' : savingStatus === 'saved' ? 'Saved' : 'Editing'}
          </div>
        </div>
      )}

      <div className="space-y-2">
        {/* Line 1: Job Name */}
        <div className="group relative">
          {isNew ? (
            <input
              ref={inputRef}
              type="text"
              value={editedName}
              onChange={(e) => setEditedName(e.target.value)}
              onKeyDown={handleKeyDown}
              onBlur={handleNameBlur}
              placeholder="Job Name"
              className="w-[80%] block text-lg font-medium text-gray-900 dark:text-white bg-gray-50 dark:bg-gray-700/50 border-0 rounded-md px-3 py-2 focus:outline-none"
            />
          ) : (
            <div className="flex items-center">
              {isEditing ? (
                <input
                  ref={inputRef}
                  type="text"
                  value={editedName}
                  onChange={(e) => setEditedName(e.target.value)}
                  onKeyDown={handleKeyDown}
                  onBlur={handleNameBlur}
                  className="w-full text-lg font-medium text-gray-900 dark:text-white bg-transparent border-b border-gray-300 dark:border-gray-600 focus:outline-none"
                />
              ) : (
                <div className="text-lg font-medium text-gray-900 dark:text-white truncate">
                  {job.name || job.default_name}
                </div>
              )}
              <button
                onClick={() => setIsEditing(true)}
                className="ml-2 opacity-0 group-hover:opacity-100 transition-opacity"
              >
                <PencilIcon className="h-4 w-4 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200" />
              </button>
            </div>
          )}
        </div>

        {/* Line 2: Command and Schedule Table */}
        <div className="group relative">
          <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
            <thead>
              <tr>
                <th className="py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider w-[25%] min-w-[200px]">Schedule</th>
                <th className="py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Command</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
              <tr>
                <td className="py-2 text-sm text-gray-900 dark:text-gray-100">
                  {isNew ? (
                    <div className="space-y-1">
                      <input
                        ref={scheduleInputRef}
                        type="text"
                        value={editedSchedule}
                        onChange={(e) => {
                          setEditedSchedule(e.target.value);
                        }}
                        onKeyDown={handleScheduleKeyDown}
                        onBlur={handleScheduleBlur}
                        placeholder="* * * * *"
                        className="w-full text-sm font-mono bg-gray-50 dark:bg-gray-700/50 border-0 rounded-l-md rounded-r-none px-3 py-2 focus:outline-none"
                      />
                    </div>
                  ) : (
                    <div className="flex items-center">
                      {isEditingSchedule ? (
                        <input
                          ref={scheduleInputRef}
                          type="text"
                          value={editedSchedule}
                          onChange={(e) => {
                            setEditedSchedule(e.target.value);
                          }}
                          onKeyDown={handleScheduleKeyDown}
                          onBlur={handleScheduleBlur}
                          className="w-full text-sm font-mono bg-transparent border-b border-gray-300 dark:border-gray-600 focus:outline-none"
                        />
                      ) : (
                        <span className="text-sm font-mono text-gray-500 dark:text-gray-400">
                          {editedSchedule || job.expression}
                        </span>
                      )}
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
                  )}
                </td>
                <td className="py-2 text-sm text-gray-900 dark:text-gray-100">
                  <div className="flex items-center justify-between">
                    <div className="flex-1">
                      {isNew ? (
                        <input
                          ref={commandInputRef}
                          type="text"
                          value={editedCommand}
                          onChange={(e) => setEditedCommand(e.target.value)}
                          onKeyDown={handleCommandKeyDown}
                          onBlur={handleCommandBlur}
                          placeholder="Command to run"
                          className="w-full text-sm text-gray-900 dark:text-gray-100 font-mono bg-gray-50 dark:bg-gray-700/50 border-0 rounded-r-md rounded-l-none px-3 py-2 focus:outline-none"
                        />
                      ) : (
                        <div className="flex items-center">
                          {isEditingCommand ? (
                            <input
                              ref={commandInputRef}
                              type="text"
                              value={editedCommand}
                              onChange={(e) => setEditedCommand(e.target.value)}
                              onKeyDown={handleCommandKeyDown}
                              onBlur={handleCommandBlur}
                              className="w-full text-sm text-gray-900 dark:text-gray-100 font-mono bg-transparent border-b border-gray-300 dark:border-gray-600 focus:outline-none"
                            />
                          ) : (
                            <div className="text-sm text-gray-900 dark:text-gray-100 font-mono truncate">
                              {job.command}
                            </div>
                          )}
                          <button
                            onClick={() => setIsEditingCommand(true)}
                            className="ml-2 opacity-0 group-hover:opacity-100 transition-opacity"
                          >
                            <PencilIcon className="h-4 w-4 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200" />
                          </button>
                        </div>
                      )}
                    </div>
                    <button
                      onClick={() => setShowConsole(true)}
                      className="ml-4 text-sm text-gray-900 hover:text-black dark:text-gray-300 dark:hover:text-white"
                    >
                      Console
                    </button>
                  </div>
                </td>
              </tr>
              <tr>
                <td colSpan="2" className="py-2 text-sm text-gray-500 dark:text-gray-400">
                  <div className={`text-sm ${
                    isScheduleValid 
                      ? 'text-gray-600 dark:text-gray-300' 
                      : 'text-pink-500 dark:text-pink-400'
                  }`}>
                    {scheduleDescription}
                    {job.run_as_user && <span className="text-gray-400 dark:text-gray-500"> as "{job.run_as_user}"</span>}
                    {nextExecutionTimes.length > 0 ? (
                      <span className="text-gray-400 dark:text-gray-500">
                        {' '}<span className="text-gray-700 dark:text-gray-200 ml-4">Next at</span>{' '}
                        <span className="relative">
                          <span 
                            className="text-gray-700 dark:text-gray-200 cursor-pointer"
                            onMouseEnter={(e) => {
                              const tooltip = e.currentTarget.nextElementSibling;
                              if (showTimezoneTooltip) {
                                tooltip.classList.remove('hidden');
                              }
                            }}
                            onMouseLeave={(e) => {
                              const tooltip = e.currentTarget.nextElementSibling;
                              if (showTimezoneTooltip) {
                                tooltip.classList.add('hidden');
                              }
                            }}
                          >
                            {nextExecutionTimes[0].job} {nextExecutionTimes[0].jobTimezone}
                          </span>
                          {showTimezoneTooltip && (
                            <div className="absolute left-0 bottom-full mb-2 hidden bg-gray-900 text-white text-xs rounded py-1 px-2 whitespace-nowrap z-10">
                              {nextExecutionTimes[0].local} {nextExecutionTimes[0].localTimezone} Browser Time
                            </div>
                          )}
                        </span>
                        <button
                          onClick={() => setShowNextTimes(!showNextTimes)}
                          className="ml-2 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-400"
                        >
                          {showNextTimes ? 'Hide More' : 'Show More'}
                        </button>
                      </span>
                    ) : (
                      <span className="text-gray-400 dark:text-gray-500">
                        {' '}No upcoming executions
                      </span>
                    )}
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        {/* Next Execution Times Table */}
        {showNextTimes && nextExecutionTimes.length > 0 && (
          <div className="mt-2">
            <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
              <thead>
                <tr>
                  <th className="py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    Job Time ({job.timezone})
                  </th>
                  <th className="py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    Browser Time ({browserTimezone})
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                {nextExecutionTimes.map((time, index) => (
                  <tr key={index}>
                    <td className="py-2 text-sm text-gray-900 dark:text-gray-100">
                      {time.job}
                    </td>
                    <td className="py-2 text-sm text-gray-900 dark:text-gray-100">
                      {time.local}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {/* Line 4: Monitoring and Location Table */}
        <div className="group relative">
          <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
            <thead>
              <tr>
                <th className="py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider w-[25%] min-w-[200px]">Monitoring</th>
                <th className="py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider w-[37.5%]">Location</th>
                {isNew && selectedLocation && !isUserCrontab && (
                  <th className="py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider w-[37.5%]">User</th>
                )}
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
              <tr>
                <td className="py-2 text-sm text-gray-900 dark:text-gray-100">
                  <div className="flex items-center space-x-2">
                    <StatusIndicator job={job} mutate={mutate} allJobs={allJobs} isNew={isNew} onFormChange={onFormChange} />
                  </div>
                </td>
                <td className="py-2 text-sm text-gray-900 dark:text-gray-100">
                  {isNew ? (
                    <div className="mb-4">
                      <select
                        value={selectedLocation}
                        onChange={handleLocationChange}
                        className="w-full bg-gray-50 dark:bg-gray-700/50 border-0 focus:ring-1 focus:ring-blue-500/50 rounded-md px-3 py-2"
                      >
                        <option value="">Select a location</option>
                        {cronFiles.map((file) => (
                          <option key={file.filename} value={file.filename}>
                            {file.display_name}
                          </option>
                        ))}
                        <option value="/etc/cron.d">/etc/cron.d (New Crontab)</option>
                      </select>
                    </div>
                  ) : (
                    <div>
                      {job.crontab_filename.startsWith('user') ? 'User' + job.crontab_filename.slice(4) : job.crontab_filename} L{job.line_number}
                    </div>
                  )}
                </td>
                {isNew && (
                  <td className="py-2 text-sm text-gray-900 dark:text-gray-100">
                    <div className="mb-4">
                      {selectedLocation && !isUserCrontab ? (
                        <select
                          value={selectedUser}
                          onChange={handleUserChange}
                          className="w-full bg-gray-50 dark:bg-gray-700/50 border-0 focus:ring-1 focus:ring-blue-500/50 rounded-md px-3 py-2"
                        >
                          <option value="">Select a user</option>
                          {users.map((user) => (
                            <option key={user} value={user}>{user}</option>
                          ))}
                        </select>
                      ) : null}
                    </div>
                  </td>
                )}
              </tr>
            </tbody>
          </table>
        </div>

        {/* Instances Table */}
        {showInstances && (
          <div className="mt-2">
            <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
              <thead>
                <tr>
                  <th className="py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    PID
                  </th>
                  <th className="py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
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
                      <td className="py-2 text-sm text-gray-900 dark:text-gray-100">
                        {instance.pid}
                      </td>
                      <td className="py-2 text-sm text-gray-900 dark:text-gray-100">
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
                    <td colSpan="3" className="py-2 text-sm text-gray-500 dark:text-gray-400">
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

        {/* Suspended Job Overlay */}
        {showSuspendedOverlay && (
          <div className="absolute inset-0 bg-black bg-opacity-50 rounded-lg flex items-center justify-center z-10">
            <div className="bg-white dark:bg-gray-800 p-8 rounded-lg shadow-xl max-w-lg w-full mx-4 relative">
              <CloseButton onClick={() => setShowSuspendedOverlay(false)} />
              <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">
                {job.suspended ? 'Suspended Job' : 'Scheduled Job'}
              </h3>
              <p className="text-gray-600 dark:text-gray-300 mb-4">
                {job.suspended 
                  ? 'This job is suspended and will not be run at its scheduled time.'
                  : 'This job will run at its scheduled time. If you suspend this job, it will be commented-out in the crontab.'}
              </p>
              {!job.suspended && job.is_monitored && (
                <div className="mb-4">
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                    Pause monitoring
                  </label>
                  <select
                    className="mt-1 block w-full pl-3 pr-10 py-2 text-base border-gray-300 dark:border-gray-600 focus:outline-none focus:ring-blue-500 focus:border-blue-500 sm:text-sm rounded-md bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 appearance-none bg-[url('data:image/svg+xml;charset=utf-8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20fill%3D%22none%22%20viewBox%3D%220%200%2020%2020%22%3E%3Cpath%20stroke%3D%22%236B7280%22%20stroke-linecap%3D%22round%22%20stroke-linejoin%3D%22round%22%20stroke-width%3D%221.5%22%20d%3D%22M6%208l4%204%204-4%22%2F%3E%3C%2Fsvg%3E')] bg-[length:1.5em_1.5em] bg-[right_0.5rem_center] bg-no-repeat"
                    value={job.pause_hours || ''}
                    onChange={(e) => {
                      const updatedJobs = allJobs.map(j => {
                        if (j.key === job.key) {
                          return { ...j, pause_hours: e.target.value };
                        }
                        return j;
                      });
                      mutate(updatedJobs, false);
                    }}
                  >
                    <option value="">Indefinitely</option>
                    <option value="1">1 hour</option>
                    <option value="12">12 hours</option>
                    <option value="24">1 day</option>
                    <option value="48">2 days</option>
                    <option value="120">5 days</option>
                    <option value="240">10 days</option>
                    <option value="720">30 days</option>
                  </select>
                </div>
              )}
              <div className="flex justify-end space-x-4">
                <button
                  onClick={async () => {
                    // Dismiss modal immediately
                    setShowSuspendedOverlay(false);

                    // Optimistic update
                    const optimisticData = allJobs.map(j => {
                      if (j.key === job.key) {
                        return { ...j, suspended: !job.suspended };
                      }
                      return j;
                    });
                    mutate(optimisticData, false);

                    try {
                      const response = await fetch('/api/jobs', {
                        method: 'PUT',
                        headers: {
                          'Content-Type': 'application/json',
                        },
                        body: JSON.stringify({
                          ...job,
                          suspended: !job.suspended,
                          pause_hours: !job.suspended && job.is_monitored ? job.pause_hours : job.suspended && job.is_monitored ? "0" : null,
                        }),
                      });

                      if (!response.ok) {
                        throw new Error('Failed to update job status');
                      }

                      // Invalidate the cache to trigger a refresh
                      mutate();
                    } catch (error) {
                      // Revert optimistic update on error
                      const revertedData = allJobs.map(j => {
                        if (j.key === job.key) {
                          return { ...j, suspended: job.suspended };
                        }
                        return j;
                      });
                      mutate(revertedData, false);
                      console.error('Error updating job status:', error);
                    }
                  }}
                  className={`px-4 py-2 text-sm font-medium text-white rounded-md ${
                    job.suspended 
                      ? 'bg-green-600 hover:bg-green-700' 
                      : 'bg-red-600 hover:bg-red-700'
                  }`}
                >
                  {job.suspended ? 'Activate Job' : 'Suspend Job'}
                </button>
                <button
                  onClick={() => {
                    setShowSuspendedOverlay(false);
                    setShowDeleteConfirmation(true);
                  }}
                  className="px-4 py-2 text-sm font-medium text-white bg-red-600 hover:bg-red-700 rounded-md"
                >
                  Delete Job
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Delete Confirmation Modal */}
        {showDeleteConfirmation && (
          <div className="absolute inset-0 bg-black bg-opacity-50 rounded-lg flex items-center justify-center z-10">
            <div className="bg-white dark:bg-gray-800 p-8 rounded-lg shadow-xl max-w-lg w-full mx-4 relative">
              <CloseButton onClick={() => setShowDeleteConfirmation(false)} />
              <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">
                Delete Job
              </h3>
              <p className="text-gray-600 dark:text-gray-300 mb-4">
                Type DELETE to confirm. This cannot be undone.
              </p>
              <input
                type="text"
                placeholder="DELETE"
                value={deleteConfirmation}
                onChange={(e) => setDeleteConfirmation(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md focus:outline-none focus:ring-red-500 focus:border-red-500"
              />
              <div className="mt-4 flex justify-end">
                <button
                  onClick={async () => {
                    if (deleteConfirmation.toLowerCase() !== 'delete') {
                      return;
                    }

                    // Optimistic update - remove the job from the list
                    const optimisticData = allJobs.filter(j => j.key !== job.key);
                    mutate(optimisticData, false);

                    try {
                      const response = await fetch('/api/jobs', {
                        method: 'DELETE',
                        headers: {
                          'Content-Type': 'application/json',
                        },
                        body: JSON.stringify(job),
                      });

                      if (!response.ok) {
                        throw new Error('Failed to delete job');
                      }

                      // Invalidate the cache to trigger a refresh
                      mutate();
                    } catch (error) {
                      // Revert optimistic update on error
                      mutate(allJobs, false);
                      console.error('Error deleting job:', error);
                    }

                    setShowDeleteConfirmation(false);
                    setDeleteConfirmation('');
                  }}
                  disabled={deleteConfirmation.toLowerCase() !== 'delete'}
                  className={`px-4 py-2 text-sm font-medium text-white rounded-md ${
                    deleteConfirmation.toLowerCase() === 'delete'
                      ? 'bg-red-600 hover:bg-red-700'
                      : 'bg-gray-400 cursor-not-allowed'
                  }`}
                >
                  Delete Job
                </button>
              </div>
            </div>
          </div>
        )}

        {showConsole && <ConsoleModal 
          job={job} 
          onClose={() => setShowConsole(false)} 
          isNew={isNew}
          onFormChange={onFormChange}
        />}

        {/* Save/Discard Buttons for New Jobs */}
        {isNew && (
          <div className="mt-4 flex justify-end space-x-4">
            <button
              onClick={handleDiscard}
              className="text-sm text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
            >
              Discard
            </button>
            <button
              onClick={onSave}
              disabled={!editedName || !editedSchedule || !editedCommand || !editedCronFile || (!editedCronFile.startsWith('user') && !editedRunAsUser)}
              className={`px-4 py-2 text-sm font-medium text-white rounded-md ${
                !editedName || !editedSchedule || !editedCommand || !editedCronFile || (!editedCronFile.startsWith('user') && !editedRunAsUser)
                  ? 'bg-gray-400 cursor-not-allowed'
                  : 'bg-blue-600 hover:bg-blue-700'
              }`}
            >
              Save
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

export default function Jobs() {
  const { data: jobs, error, mutate } = useSWR('/api/jobs', fetcher, {
    refreshInterval: 5000, // Refresh every 5 seconds
    revalidateOnFocus: true, // Refresh when tab regains focus
  });
  const [showNewJob, setShowNewJob] = React.useState(false);
  const [newJobForm, setNewJobForm] = React.useState({
    name: '',
    expression: '',
    command: '',
    crontab_filename: '',
    run_as_user: '',
    is_monitored: false,
    is_draft: true
  });
  const [isToastVisible, setIsToastVisible] = React.useState(false);
  const [toastMessage, setToastMessage] = React.useState('');
  const [toastType, setToastType] = React.useState('error');

  const showToast = (message, type = 'error') => {
    setToastMessage(message);
    setToastType(type);
    setIsToastVisible(true);
    setTimeout(() => setIsToastVisible(false), 3000);
  };

  if (error) return <div className="text-gray-600 dark:text-gray-300">Failed to load jobs</div>;
  if (!jobs) return <div className="text-gray-600 dark:text-gray-300">Loading...</div>;

  const handleSaveNewJob = async () => {
    try {
      const response = await fetch('/api/jobs', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(newJobForm),
      });

      if (!response.ok) {
        throw new Error('Failed to create job');
      }

      setShowNewJob(false);
      setNewJobForm({
        name: '',
        expression: '',
        command: '',
        crontab_filename: '',
        run_as_user: '',
        is_monitored: false,
        is_draft: true
      });
      mutate();
    } catch (error) {
      console.error('Error creating job:', error);
      showToast('Failed to create job: ' + error.message);
    }
  };

  return (
    <div>
      <div className="flex justify-between items-center mb-6">
        <h1 className="text-2xl font-semibold text-gray-900 dark:text-white -mt-1.5">Jobs</h1>
        <button
          onClick={() => setShowNewJob(true)}
          className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 font-medium"
        >
          Add Job
        </button>
      </div>
      <div className="space-y-4">
        {showNewJob && (
          <JobCard 
            job={newJobForm} 
            mutate={mutate} 
            allJobs={jobs} 
            isNew={true}
            onSave={handleSaveNewJob}
            onDiscard={() => setShowNewJob(false)}
            onFormChange={setNewJobForm}
            showToast={showToast}
          />
        )}
        {jobs.map((job, index) => (
          <JobCard 
            key={index} 
            job={job} 
            mutate={mutate} 
            allJobs={jobs} 
            showToast={showToast}
          />
        ))}
      </div>
      {isToastVisible && <Toast message={toastMessage} onClose={() => setIsToastVisible(false)} type={toastType} />}
    </div>
  );
} 