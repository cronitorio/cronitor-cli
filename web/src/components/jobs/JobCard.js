import React from 'react';
import { JobHeader } from './JobHeader';
import { StatusBadges } from './StatusBadges';
import { ScheduleSection } from './ScheduleSection';
import { CommandSection } from './CommandSection';
import { MonitoringSection } from './MonitoringSection';
import { LocationSection } from './LocationSection';
import { InstancesTable } from './InstancesTable';
import { SuspendOverlay } from './SuspendOverlay';
import { DeleteConfirmation } from './DeleteConfirmation';
import { ConsoleModal } from './ConsoleModal';
import { useJobOperations } from '../../hooks/useJobOperations';
import { Switch } from '@headlessui/react';
import { ClockIcon, UserIcon, DocumentTextIcon } from '@heroicons/react/24/outline';

export function JobCard({ job: initialJob, mutate, allJobs, isNew = false, onSave, onDiscard, onFormChange, onLocationChange, showToast, isMacOS, onJobChange, crontabMutate, selectedCrontab, setSelectedCrontab, readOnly = false }) {
  const [isEditing, setIsEditing] = React.useState(isNew);
  const [isEditingCommand, setIsEditingCommand] = React.useState(isNew);
  const [isEditingSchedule, setIsEditingSchedule] = React.useState(isNew);
  const [editedName, setEditedName] = React.useState(initialJob.name || initialJob.default_name);
  const [editedCommand, setEditedCommand] = React.useState(initialJob.command);
  const [editedSchedule, setEditedSchedule] = React.useState(initialJob.expression || '');
  const [editedCronFile, setEditedCronFile] = React.useState(initialJob.crontab_filename || '');
  const [editedRunAsUser, setEditedRunAsUser] = React.useState(initialJob.run_as_user || '');
  const [cronFiles, setCronFiles] = React.useState([]);
  const [users, setUsers] = React.useState([]);
  const [selectedLocation, setSelectedLocation] = React.useState('');
  const [selectedUser, setSelectedUser] = React.useState('');
  const [isUserCrontab, setIsUserCrontab] = React.useState(false);
  const [showInstances, setShowInstances] = React.useState(false);
  const [showNextTimes, setShowNextTimes] = React.useState(false);
  const [killingPids, setKillingPids] = React.useState(new Set());
  const [isKillingAll, setIsKillingAll] = React.useState(false);
  const [showSuspendedOverlay, setShowSuspendedOverlay] = React.useState(false);
  const [savingStatus, setSavingStatus] = React.useState(null);
  const [showConsole, setShowConsole] = React.useState(false);
  const [showDeleteConfirmation, setShowDeleteConfirmation] = React.useState(false);
  const [deleteConfirmation, setDeleteConfirmation] = React.useState('');
  const [showLearnMore, setShowLearnMore] = React.useState(false);
  const [showSuspendedDialog, setShowSuspendedDialog] = React.useState(false);
  const [showScheduledDialog, setShowScheduledDialog] = React.useState(false);
  const [suspendedReason, setSuspendedReason] = React.useState('');
  const [scheduledTime, setScheduledTime] = React.useState('');
  const [scheduledReason, setScheduledReason] = React.useState('');
  const [isSubmitting, setIsSubmitting] = React.useState(false);
  const [wasMonitoredBeforeSuspend, setWasMonitoredBeforeSuspend] = React.useState(false);
  const [pendingChanges, setPendingChanges] = React.useState(null);

  const { jobs, createJob, updateJob, deleteJob, toggleJobMonitoring, toggleJobSuspension, killJobProcess, mutate: jobsMutate } = useJobOperations();

  // Get the current job data from allJobs and update local state
  React.useEffect(() => {
    const currentJob = allJobs.find(j => j.key === initialJob.key) || initialJob;
    
    // If we have pending changes and they match what's now in the server data, clear them
    if (pendingChanges) {
      if (
        currentJob.name === pendingChanges.name &&
        currentJob.command === pendingChanges.command &&
        currentJob.expression === pendingChanges.expression
      ) {
        setPendingChanges(null);
      }
    }
    
    // Only update edited values if we're not editing and don't have pending changes
    if (!isEditing && !isEditingCommand && !isEditingSchedule && !pendingChanges) {
      setEditedName(currentJob.name || currentJob.default_name);
      setEditedCommand(currentJob.command);
      setEditedSchedule(currentJob.expression || '');
    }
  }, [allJobs, initialJob, isEditing, isEditingCommand, isEditingSchedule, pendingChanges]);

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

  // Update parent form state when local state changes
  React.useEffect(() => {
    if (isNew) {
      onFormChange({
        ...initialJob,
        name: editedName,
        expression: editedSchedule,
        command: editedCommand,
        crontab_filename: editedCronFile?.filename || initialJob.crontab_filename,
        crontab_display_name: editedCronFile?.display_name || initialJob.crontab_display_name,
        run_as_user: editedRunAsUser,
        is_draft: false,
        suspended: initialJob.suspended,
        monitored: initialJob.monitored
      });
    }
  }, [isNew, onFormChange, editedName, editedSchedule, editedCommand, editedCronFile, editedRunAsUser, initialJob.monitored, initialJob]);

  const handleFormChange = (field, value) => {
    const newData = { ...initialJob, [field]: value };
    // Only call onFormChange if it's provided (for new jobs)
    if (typeof onFormChange === 'function') {
      onFormChange(newData);
    }
  };

  const handleSave = async () => {
    setSavingStatus('saving');
    
    // Store the values we're saving as pending changes
    setPendingChanges({
      name: editedName,
      command: editedCommand,
      expression: editedSchedule
    });
    
    try {
      // Get the latest state values
      const updatedJob = {
        ...initialJob,
        name: editedName,
        command: editedCommand,
        expression: editedSchedule,
        is_draft: false,
        suspended: initialJob.suspended,
        monitored: initialJob.monitored
      };

      if (isNew && typeof onSave === 'function') {
        await onSave(updatedJob);
      } else {
        // Ensure we're sending the complete job data
        const jobToUpdate = {
          ...updatedJob,
          key: initialJob.key,
          code: initialJob.code,
          crontab_filename: initialJob.crontab_filename,
          run_as_user: initialJob.run_as_user
        };

        // Make the API call
        const response = await fetch('/api/jobs', {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify(jobToUpdate),
        });

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(`Failed to update job: ${response.status} ${response.statusText}${errorText ? ` - ${errorText}` : ''}`);
        }

        // Trigger a revalidation
        mutate();
        if (onJobChange) {
          // Add a small delay to ensure the server has processed the change
          setTimeout(onJobChange, 100);
        }
        
        // Also refresh crontab data since job changes modify the crontab
        if (crontabMutate) {
          setTimeout(async () => {
            await crontabMutate();
            // Update the selected crontab if we have it
            if (selectedCrontab && setSelectedCrontab) {
              const updatedCrontabs = await crontabMutate();
              if (updatedCrontabs) {
                const updatedCrontab = updatedCrontabs.find(c => c.filename === selectedCrontab.filename);
                if (updatedCrontab) {
                  setSelectedCrontab(updatedCrontab);
                }
              }
            }
          }, 150);
        }
      }

      setSavingStatus('saved');
      setTimeout(() => {
        setSavingStatus(null);
        if (isEditing) setIsEditing(false);
        if (isEditingCommand) setIsEditingCommand(false);
        if (isEditingSchedule) setIsEditingSchedule(false);
      }, 1000);
    } catch (error) {
      setSavingStatus(null);
      setPendingChanges(null); // Clear pending changes on error
      showToast('Failed to update job: ' + error.message);
    }
  };

  const handleDiscard = () => {
    if (isNew) {
      onDiscard();
    } else {
      setEditedName(initialJob.name || initialJob.default_name);
      setEditedCommand(initialJob.command);
      setEditedSchedule(initialJob.expression || '');
      setIsEditing(false);
      setIsEditingCommand(false);
      setIsEditingSchedule(false);
    }
  };

  const handleSuspendedToggle = async () => {
    if (!initialJob.is_suspended) {
      setShowSuspendedDialog(true);
      return;
    }

    try {
      const response = await fetch(`/api/jobs/${initialJob.id}/unsuspend`, {
        method: 'POST',
      });

      if (!response.ok) {
        throw new Error('Failed to unsuspend job');
      }

      mutate();
      showToast('Job unsuspended successfully', 'success');
    } catch (error) {
      console.error('Error unsuspending job:', error);
      showToast('Failed to unsuspend job: ' + error.message);
    }
  };

  const handleScheduledToggle = async () => {
    if (!initialJob.is_scheduled) {
      setShowScheduledDialog(true);
      return;
    }

    try {
      const response = await fetch(`/api/jobs/${initialJob.id}/unschedule`, {
        method: 'POST',
      });

      if (!response.ok) {
        throw new Error('Failed to unschedule job');
      }

      mutate();
      showToast('Job unscheduled successfully', 'success');
    } catch (error) {
      console.error('Error unscheduling job:', error);
      showToast('Failed to unschedule job: ' + error.message);
    }
  };

  const handleSuspendedSubmit = async () => {
    try {
      const response = await fetch(`/api/jobs/${initialJob.id}/suspend`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ reason: suspendedReason }),
      });

      if (!response.ok) {
        throw new Error('Failed to suspend job');
      }

      mutate();
      if (onJobChange) onJobChange();
      showToast('Job suspended successfully', 'success');
    } catch (error) {
      console.error('Error suspending job:', error);
      showToast('Failed to suspend job: ' + error.message);
    }
  };

  const handleScheduledSubmit = async () => {
    try {
      const response = await fetch(`/api/jobs/${initialJob.id}/schedule`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          time: scheduledTime,
          reason: scheduledReason,
        }),
      });

      if (!response.ok) {
        throw new Error('Failed to schedule job');
      }

      mutate();
      if (onJobChange) onJobChange();
      showToast('Job scheduled successfully', 'success');
    } catch (error) {
      console.error('Error scheduling job:', error);
      showToast('Failed to schedule job: ' + error.message);
    }
  };

  const handleLocationChange = (e) => {
    const location = e.target.value;
    setSelectedLocation(location);
    
    if (location === "/etc/cron.d (New Crontab)") {
      onLocationChange(location);
      return;
    }
    
    const selectedCronFile = cronFiles.find(file => file.filename === location);
    if (selectedCronFile) {
      setIsUserCrontab(selectedCronFile.isUserCrontab);
      setSelectedUser('');
      const timezone = selectedCronFile.timezone || 'UTC';
      handleFormChange('timezone', timezone);
      setEditedCronFile(selectedCronFile.filename);
      handleFormChange('crontab_filename', selectedCronFile.filename);
      handleFormChange('crontab_display_name', selectedCronFile.display_name);
      handleFormChange('run_as_user', selectedCronFile.isUserCrontab ? selectedCronFile.user : '');
    }
  };

  const handleUserChange = (e) => {
    const user = e.target.value;
    setSelectedUser(user);
    setEditedRunAsUser(user);
    handleFormChange('run_as_user', user);
  };

  const handleKill = async (pids, isAll = false) => {
    setKillingPids(prev => new Set([...prev, ...pids]));
    if (isAll) {
      setIsKillingAll(true);
    }
    try {
      await killJobProcess(pids);
      if (onJobChange) onJobChange();
    } catch (error) {
      showToast('Failed to kill processes: ' + error.message);
    } finally {
      setTimeout(() => {
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

  const handleRunNow = async () => {
    try {
      const response = await fetch('/api/jobs/run', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ command: initialJob.command })
      });
      if (!response.ok) {
        throw new Error('Failed to run job');
      }
      showToast('Job started successfully', 'success');
      mutate();
      if (onJobChange) onJobChange();
    } catch (error) {
      console.error('Error running job:', error);
      showToast('Failed to run job: ' + error.message, 'error');
    }
  };

  const handleToggleSuspendedOverlay = () => {
    setShowSuspendedOverlay(!showSuspendedOverlay);
  };

  const handleToggleSuspend = async (explicitSuspendedState, explicitPauseHours) => {
    setShowSuspendedOverlay(false); // Close overlay first
    
    // Get the current job state from allJobs to ensure we have latest data
    const currentJob = allJobs.find(j => j.key === initialJob.key) || initialJob;
    
    // Add debugging
    console.log('handleToggleSuspend called');
    console.log('initialJob.key:', initialJob.key);
    console.log('allJobs:', allJobs);
    console.log('Found currentJob:', currentJob);
    console.log('isNew:', isNew);
    
    // Use the explicit state if provided, otherwise toggle the current state
    // Convert to explicit boolean with Boolean()
    const newSuspendedState = explicitSuspendedState !== undefined 
      ? Boolean(explicitSuspendedState) 
      : !currentJob.suspended;
    
    console.log('Current suspended state:', currentJob.suspended);
    console.log('New suspended state:', newSuspendedState);
    
    const updatedJob = { 
      ...currentJob,
      suspended: newSuspendedState
    };

    // Set pause_hours if provided
    if (explicitPauseHours !== undefined) {
      updatedJob.pause_hours = explicitPauseHours;
    }

    // Preserve the monitoring state instead of forcing it off when suspending
    updatedJob.monitored = currentJob.monitored;

    // Update local state immediately for optimistic UI update
    handleFormChange('suspended', newSuspendedState);
    handleFormChange('monitored', updatedJob.monitored);
    handleFormChange('pause_hours', updatedJob.pause_hours);
    
    // Create optimistic update for all jobs in the list
    const optimisticData = allJobs.map(j => 
      j.key === currentJob.key ? {...j, suspended: newSuspendedState, monitored: updatedJob.monitored, pause_hours: updatedJob.pause_hours} : j
    );
    // Apply optimistic update to prevent flickering
    mutate(optimisticData, false);
    
    try {
      // For new jobs, use onSave from props
      if (isNew && typeof onSave === 'function') {
        console.log('Calling onSave for new job');
        await onSave(updatedJob, true); // Pass true to indicate it's a suspend/resume toggle
      } else {
        console.log('Making PUT request for existing job');
        // For existing jobs, use direct API call instead of toggleJobSuspension
        // This bypasses any potential issues with the toggleJobSuspension function
        const job = allJobs.find(j => j.key === currentJob.key);
        if (!job) {
          console.error('Job not found in allJobs!');
          throw new Error('Job not found');
        }
        
        console.log('Sending API request with suspended =', newSuspendedState);
        console.log('and monitored =', updatedJob.monitored);
        if (updatedJob.pause_hours !== undefined) {
          console.log('and pause_hours =', updatedJob.pause_hours);
        }
        
        // CREATE A COMPLETELY NEW REQUEST BODY WITH EXPLICIT SUSPENDED STATE
        const requestBody = {
          key: job.key,
          code: job.code, // Include the code field for monitoring API
          command: job.command,
          expression: job.expression,
          name: job.name,
          crontab_filename: job.crontab_filename,
          run_as_user: job.run_as_user,
          // Preserve the monitoring state
          monitored: updatedJob.monitored,
          suspended: newSuspendedState === true,
          // Include pause_hours if it was set
          pause_hours: updatedJob.pause_hours
        };
        
        // For ultra-explicit safety, use a string representation in the body
        const requestBodyStr = JSON.stringify(requestBody);
        console.log('REQUEST BODY STRING:', requestBodyStr);
        
        const response = await fetch('/api/jobs', {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
          },
          body: requestBodyStr,
        });

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(`Failed to update job suspension status: ${response.status} ${response.statusText}${errorText ? ` - ${errorText}` : ''}`);
        }
        
        // Try to read the response body for debugging
        try {
          const responseData = await response.clone().json();
          console.log('Response data:', responseData);
        } catch(e) {
          console.log('Could not parse response as JSON');
        }
      }
      
      // Refresh data from server with the changes we just made
      mutate();
      if (onJobChange) {
        // Add a small delay to ensure the server has processed the change
        setTimeout(onJobChange, 100);
      }
      
      // Also refresh crontab data since suspension modifies the crontab
      if (crontabMutate) {
        setTimeout(async () => {
          await crontabMutate();
          // Update the selected crontab if we have it
          if (selectedCrontab && setSelectedCrontab) {
            const updatedCrontabs = await crontabMutate();
            if (updatedCrontabs) {
              const updatedCrontab = updatedCrontabs.find(c => c.filename === selectedCrontab.filename);
              if (updatedCrontab) {
                setSelectedCrontab(updatedCrontab);
              }
            }
          }
        }, 150);
      }
      
      showToast(`Job ${newSuspendedState ? 'suspended' : 'activated'} successfully`, 'success');
      if (isEditing) setIsEditing(false);
    } catch (error) {
      console.error('Error toggling job suspension:', error);
      showToast('Failed to toggle job suspension: ' + error.message);
      
      // Revert optimistic update on error
      handleFormChange('suspended', currentJob.suspended);
      handleFormChange('monitored', currentJob.monitored);
      handleFormChange('pause_hours', currentJob.pause_hours);
      mutate();
    }
  };

  // Get the display values - either pending changes or current values
  const getDisplayValue = (field) => {
    if (pendingChanges && pendingChanges[field] !== undefined) {
      return pendingChanges[field];
    }
    return initialJob[field];
  };

  return (
    <div className={`bg-white dark:bg-gray-800 shadow rounded-lg p-4 relative ${initialJob.suspended ? 'bg-gray-100 dark:bg-gray-700' : ''}`}>
      <StatusBadges
        job={initialJob}
        instances={initialJob.instances || []}
        showInstances={showInstances}
        onToggleInstances={() => setShowInstances(!showInstances)}
        onToggleSuspended={handleToggleSuspendedOverlay}
      />

      {!isNew && (isEditing || isEditingCommand || isEditingSchedule || savingStatus === 'saved') && (
        <div className="absolute top-0 left-0 right-0 flex items-center justify-center">
          <div 
            className={`inline-flex items-center px-2.5 py-0.5 rounded-b-lg text-sm font-medium z-20 ${
              savingStatus === 'saving' 
                ? 'bg-amber-400 text-gray-900' 
                : savingStatus === 'saved'
                ? 'bg-green-400 text-green-950'
                : 'bg-green-400 text-gray-900'
            } ${savingStatus === 'saved' ? 'pointer-events-none' : 'cursor-pointer'}`}
            onClick={handleSave}
          >
            {savingStatus === 'saving' ? 'Saving' : savingStatus === 'saved' ? 'Saved' : 'Editing'}
          </div>
        </div>
      )}

      <div className="space-y-2">
        <JobHeader
          job={{
            ...initialJob,
            name: getDisplayValue('name') || initialJob.default_name
          }}
          isEditing={isEditing}
          editedName={editedName}
          onNameChange={setEditedName}
          onEditStart={() => {
            setIsEditing(true);
            setEditedName(getDisplayValue('name') || initialJob.default_name);
          }}
          onEditEnd={() => {
            if (!isNew) {
              setIsEditing(false);
            }
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              handleSave();
            } else if (e.key === 'Escape') {
              if (!isNew) {
                setIsEditing(false);
                setEditedName(getDisplayValue('name') || initialJob.default_name);
              }
            }
          }}
          isNew={isNew}
        />

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
                  <ScheduleSection
                    job={{
                      ...initialJob,
                      expression: getDisplayValue('expression') || ''
                    }}
                    isEditing={isEditingSchedule}
                    editedSchedule={editedSchedule}
                    onScheduleChange={(value) => {
                      console.log('Schedule changed to:', value); // Debug log
                      setEditedSchedule(value);
                    }}
                    onEditStart={() => {
                      console.log('Starting edit with schedule:', getDisplayValue('expression')); // Debug log
                      setIsEditingSchedule(true);
                      setEditedSchedule(getDisplayValue('expression') || '');
                    }}
                    onEditEnd={() => {
                      if (!isNew) {
                        setIsEditingSchedule(false);
                      }
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        handleSave();
                      } else if (e.key === 'Escape') {
                        if (!isNew) {
                          setIsEditingSchedule(false);
                          setEditedSchedule(getDisplayValue('expression') || '');
                        }
                      }
                    }}
                    showDescription={false}
                    isNew={isNew}
                  />
                </td>
                <td className="py-2 text-sm text-gray-900 dark:text-gray-100">
                  <CommandSection
                    job={{
                      ...initialJob,
                      command: getDisplayValue('command')
                    }}
                    isEditing={isEditingCommand}
                    editedCommand={editedCommand}
                    onCommandChange={setEditedCommand}
                    onEditStart={() => {
                      if (!readOnly) {
                        setIsEditingCommand(true);
                      }
                    }}
                    onEditEnd={() => {
                      if (!isNew) {
                        setIsEditingCommand(false);
                      }
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        handleSave();
                      } else if (e.key === 'Escape') {
                        if (!isNew) {
                          setIsEditingCommand(false);
                          setEditedCommand(getDisplayValue('command'));
                        }
                      }
                    }}
                    onShowConsole={() => setShowConsole(true)}
                    isNew={isNew}
                    readOnly={readOnly}
                  />
                </td>
              </tr>
              <tr>
                <td colSpan="2" className="py-2 text-sm text-gray-900 dark:text-gray-100">
                  <ScheduleSection
                    job={{
                      ...initialJob,
                      expression: getDisplayValue('expression') || ''
                    }}
                    isEditing={isEditingSchedule}
                    editedSchedule={editedSchedule}
                    onScheduleChange={(value) => {
                      console.log('Schedule changed to:', value); // Debug log
                      setEditedSchedule(value);
                    }}
                    onEditStart={() => {
                      console.log('Starting edit with schedule:', getDisplayValue('expression')); // Debug log
                      setIsEditingSchedule(true);
                      setEditedSchedule(getDisplayValue('expression') || '');
                    }}
                    onEditEnd={() => {
                      if (!isNew) {
                        setIsEditingSchedule(false);
                      }
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        handleSave();
                      } else if (e.key === 'Escape') {
                        if (!isNew) {
                          setIsEditingSchedule(false);
                          setEditedSchedule(getDisplayValue('expression') || '');
                        }
                      }
                    }}
                    showDescription={true}
                    isNew={isNew}
                  />
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div className="group relative">
          <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700" style={{ tableLayout: 'fixed' }}>
            <thead>
              <tr>
                <th className="py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider w-[25%] min-w-[200px]">Monitoring</th>
                <th className="py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider" style={{ width: 'calc(75% * 0.5)' }}>Location</th>
                <th className="py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider" style={{ width: '37.5%' }}>
                  {isNew && selectedLocation && !isUserCrontab && <>User</>}
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
              <tr>
                <td className="py-2 text-sm text-gray-900 dark:text-gray-100" style={{ width: '25%' }}>
                  <MonitoringSection
                    job={initialJob}
                    onUpdate={async (updatedJob) => {
                      handleFormChange('suspended', updatedJob.suspended);
                      handleFormChange('monitored', updatedJob.monitored);
                      handleFormChange('pause_hours', updatedJob.pause_hours);
                      if (isNew) {
                        onFormChange(updatedJob);
                      } else {
                        try {
                          await toggleJobMonitoring(initialJob.key, updatedJob.monitored);
                          mutate();
                          if (onJobChange) onJobChange();
                        } catch (error) {
                          showToast('Failed to update monitoring status: ' + error.message);
                        }
                      }
                    }}
                    onShowLearnMore={() => setShowLearnMore(true)}
                  />
                </td>
                <td className="py-2 text-sm text-gray-900 dark:text-gray-100" style={{ width: '37.5%' }}>
                  <LocationSection
                    job={initialJob}
                    isNew={isNew}
                    cronFiles={cronFiles}
                    users={users}
                    selectedLocation={selectedLocation}
                    selectedUser={selectedUser}
                    isUserCrontab={isUserCrontab}
                    onLocationChange={handleLocationChange}
                    onUserChange={handleUserChange}
                    isMacOS={isMacOS}
                  />
                </td>
                {isNew && selectedLocation && !isUserCrontab && (
                  <td className="py-2 text-sm text-gray-900 dark:text-gray-100" style={{ width: '37.5%' }}>
                    <select
                      value={selectedUser}
                      onChange={handleUserChange}
                      className="mt-1 block w-full pl-3 pr-10 py-2 text-base border-gray-400 dark:border-gray-600 focus:outline-none focus:ring-blue-500 focus:border-blue-500 sm:text-sm rounded-md bg-gray-200 dark:bg-gray-700 text-gray-800 dark:text-gray-100 appearance-none bg-[url('data:image/svg+xml;charset=utf-8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20fill%3D%22none%22%20viewBox%3D%220%200%2020%2020%22%3E%3Cpath%20stroke%3D%22%236B7280%22%20stroke-linecap%3D%22round%22%20stroke-linejoin%3D%22round%22%20stroke-width%3D%221.5%22%20d%3D%22M6%208l4%204%204-4%22%2F%3E%3C%2Fsvg%3E')] bg-[length:1.5em_1.5em] bg-[right_0.5rem_center] bg-no-repeat"
                    >
                      <option value="">Select a user</option>
                      {users.map((user) => (
                        <option key={user} value={user}>{user}</option>
                      ))}
                    </select>
                  </td>
                )}
              </tr>
            </tbody>
          </table>
        </div>

        {showInstances && (
          <InstancesTable
            instances={initialJob.instances || []}
            killingPids={killingPids}
            isKillingAll={isKillingAll}
            onKillInstance={(pid) => handleKill([pid])}
            onKillAll={() => handleKill((initialJob.instances || []).map(i => i.pid), true)}
            onRunNow={handleRunNow}
          />
        )}

        {showSuspendedOverlay && (
          <SuspendOverlay
            job={allJobs.find(j => j.key === initialJob.key) || initialJob}
            allJobs={allJobs}
            onClose={() => setShowSuspendedOverlay(false)}
            onToggleSuspension={handleToggleSuspend}
            onShowDeleteConfirmation={() => {
              setShowSuspendedOverlay(false);
              setShowDeleteConfirmation(true);
            }}
            mutate={jobsMutate}
          />
        )}

        {showDeleteConfirmation && (
          <DeleteConfirmation
            onClose={() => setShowDeleteConfirmation(false)}
            onDelete={async () => {
              if (deleteConfirmation.toLowerCase() !== 'delete') return;
              try {
                await deleteJob(initialJob.key);
                setShowDeleteConfirmation(false);
                setDeleteConfirmation('');
                showToast('Job deleted successfully', 'success');
              } catch (error) {
                showToast('Failed to delete job: ' + error.message);
              }
            }}
            deleteConfirmation={deleteConfirmation}
            onDeleteConfirmationChange={setDeleteConfirmation}
          />
        )}

        {showConsole && (
          <ConsoleModal
            job={initialJob}
            onClose={() => setShowConsole(false)}
            isNew={isNew}
            onFormChange={onFormChange}
            onCommandUpdate={setEditedCommand}
          />
        )}

        {showLearnMore && (
          <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50" style={{ margin: '0px' }}>
            <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl max-w-6xl w-full mx-4 relative">
              <button
                onClick={() => setShowLearnMore(false)}
                className="absolute top-0 right-8 bg-white dark:bg-gray-800 px-3 py-0 rounded-b-sm border border-t-0 border-gray-300 dark:border-gray-600 text-gray-400 hover:text-gray-500 dark:text-gray-400 dark:hover:text-gray-300 z-10 text-xl leading-none"
              >
                ×
              </button>
              <div className="p-8">
                <div className="flex">
                  <div className="w-2/3 pr-8">
                    <h2 className="text-2xl font-black text-gray-900 dark:text-white mb-8">Monitor your jobs with Cronitor</h2>
                    <ul className="space-y-6">
                      <li className="flex items-start">
                        <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth="1.5" stroke="currentColor" aria-hidden="true" data-slot="icon" className="h-7 w-7 text-green-500 mr-3 flex-shrink-0 mt-0.5">
                          <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"></path>
                        </svg>
                        <span className="text-lg text-gray-700 dark:text-gray-300 leading-relaxed">Instant alerts if a job fails or never starts.</span>
                      </li>
                      <li className="flex items-start">
                        <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth="1.5" stroke="currentColor" aria-hidden="true" data-slot="icon" className="h-7 w-7 text-green-500 mr-3 flex-shrink-0 mt-0.5">
                          <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"></path>
                        </svg>
                        <span className="text-lg text-gray-700 dark:text-gray-300 leading-relaxed">See the status, metrics and logs from every job.</span>
                      </li>
                      <li className="flex items-start">
                        <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth="1.5" stroke="currentColor" aria-hidden="true" data-slot="icon" className="h-7 w-7 text-green-500 mr-3 flex-shrink-0 mt-0.5">
                          <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"></path>
                        </svg>
                        <span className="text-lg text-gray-700 dark:text-gray-300 leading-relaxed">Track performance with a full year of data retention.</span>
                      </li>
                      <li className="flex items-start">
                        <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth="1.5" stroke="currentColor" aria-hidden="true" data-slot="icon" className="h-7 w-7 text-green-500 mr-3 flex-shrink-0 mt-0.5">
                          <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"></path>
                        </svg>
                        <span className="text-lg text-gray-700 dark:text-gray-300 leading-relaxed">Start for free, no credit card required.</span>
                      </li>
                    </ul>
                    <div className="mt-10">
                      <a href="https://cronitor.io/cron-job-monitoring?utm_source=cli&utm_campaign=modal&utm_content=1" target="_blank" rel="noopener noreferrer" className="inline-flex items-center px-6 py-3 border border-transparent text-base font-medium rounded-md shadow-sm text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500">
                        Learn More
                      </a>
                    </div>
                  </div>
                  <div className="w-1/3 overflow-hidden relative">
                    <a href="https://cronitor.io/cron-job-monitoring?utm_source=cli&utm_campaign=modal&utm_content=1" target="_blank" rel="noopener noreferrer" className="block">
                      <img src="/static/media/cronitor-screenshot.6101d4163e37020459b5.png" alt="Cronitor Dashboard" className="w-full h-auto" style={{ objectPosition: 'left center', width: '167%', maxWidth: 'none' }} />
                    </a>
                  </div>
                </div>
              </div>
            </div>
          </div>
        )}

        {isNew && (
          <div className="mt-4 flex justify-end space-x-4">
            <button
              onClick={onDiscard}
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

      {/* Suspended Dialog */}
      {showSuspendedDialog && (
        <div className="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center p-4">
          <div className="bg-white dark:bg-gray-800 rounded-lg p-6 max-w-md w-full">
            <h3 className="text-lg font-medium mb-4">Suspend Job</h3>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
                  Reason
                </label>
                <input
                  type="text"
                  value={suspendedReason}
                  onChange={(e) => setSuspendedReason(e.target.value)}
                  className="mt-1 block w-full rounded-md border-gray-400 dark:border-gray-600 shadow-sm focus:border-blue-500 focus:ring-blue-500 dark:bg-gray-700 dark:text-white sm:text-sm text-gray-800 bg-gray-200"
                />
              </div>
              <div className="flex justify-end space-x-3">
                <button
                  onClick={() => setShowSuspendedDialog(false)}
                  className="px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-md hover:bg-gray-50 dark:hover:bg-gray-600"
                >
                  Cancel
                </button>
                <button
                  onClick={handleSuspendedSubmit}
                  className="px-4 py-2 text-sm font-medium text-white bg-red-600 rounded-md hover:bg-red-700"
                >
                  Suspend
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Scheduled Dialog */}
      {showScheduledDialog && (
        <div className="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center p-4">
          <div className="bg-white dark:bg-gray-800 rounded-lg p-6 max-w-md w-full">
            <h3 className="text-lg font-medium mb-4">Schedule Job</h3>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
                  Time
                </label>
                <input
                  type="datetime-local"
                  value={scheduledTime}
                  onChange={(e) => setScheduledTime(e.target.value)}
                  className="mt-1 block w-full rounded-md border-gray-400 dark:border-gray-600 shadow-sm focus:border-blue-500 focus:ring-blue-500 dark:bg-gray-700 dark:text-white sm:text-sm text-gray-800 bg-gray-200"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
                  Reason
                </label>
                <input
                  type="text"
                  value={scheduledReason}
                  onChange={(e) => setScheduledReason(e.target.value)}
                  className="mt-1 block w-full rounded-md border-gray-400 dark:border-gray-600 shadow-sm focus:border-blue-500 focus:ring-blue-500 dark:bg-gray-700 dark:text-white sm:text-sm text-gray-800 bg-gray-200"
                />
              </div>
              <div className="flex justify-end space-x-3">
                <button
                  onClick={() => setShowScheduledDialog(false)}
                  className="px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-md hover:bg-gray-50 dark:hover:bg-gray-600"
                >
                  Cancel
                </button>
                <button
                  onClick={handleScheduledSubmit}
                  className="px-4 py-2 text-sm font-medium text-white bg-yellow-600 rounded-md hover:bg-yellow-700"
                >
                  Schedule
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
} 