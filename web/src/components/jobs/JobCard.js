import React from 'react';
import { JobHeader } from './JobHeader';
import { StatusBadges } from './StatusBadges';
import { ScheduleSection } from './ScheduleSection';
import { CommandSection } from './CommandSection';
import { MonitoringSection } from './MonitoringSection';
import { LocationSection } from './LocationSection';
import { InstancesTable } from './InstancesTable';
import { SuspendOverlay } from './SuspendOverlay';
import { HideOverlay } from './HideOverlay';
import { DeleteConfirmation } from './DeleteConfirmation';
import { ConsoleModal } from './ConsoleModal';
import { LearnMoreModal } from './LearnMoreModal';
import { useJobOperations } from '../../hooks/useJobOperations';
import { csrfFetch } from '../../utils/api';

export function JobCard({ job: initialJob, mutate, mutateCrontabs, allJobs, isNew = false, onSave, onDiscard, onFormChange, onLocationChange, showToast, isMacOS, onJobChange, crontabMutate, selectedCrontab, setSelectedCrontab, readOnly = false, settings, monitorsLoading = false, users = [], crontabs = [], setPendingMonitoringJobs }) {
  const [isEditing, setIsEditing] = React.useState(isNew);
  const [isEditingCommand, setIsEditingCommand] = React.useState(isNew);
  const [isEditingSchedule, setIsEditingSchedule] = React.useState(isNew);
  const [editedName, setEditedName] = React.useState(initialJob.name || initialJob.default_name);
  const [editedCommand, setEditedCommand] = React.useState(initialJob.command);
  const [editedSchedule, setEditedSchedule] = React.useState(initialJob.expression || '');
  const [editedCronFile, setEditedCronFile] = React.useState(initialJob.crontab_filename || '');
  const [editedRunAsUser, setEditedRunAsUser] = React.useState(initialJob.run_as_user || '');
  const [editedMonitored, setEditedMonitored] = React.useState(initialJob.monitored || false);
  const [selectedLocation, setSelectedLocation] = React.useState('');
  const [selectedUser, setSelectedUser] = React.useState('');
  const [isUserCrontab, setIsUserCrontab] = React.useState(false);
  const [showInstances, setShowInstances] = React.useState(false);
  const [killingPids, setKillingPids] = React.useState(new Set());
  const [isKillingAll, setIsKillingAll] = React.useState(false);
  const [showSuspendedOverlay, setShowSuspendedOverlay] = React.useState(false);
  const [showHideOverlay, setShowHideOverlay] = React.useState(false);
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
  const [pendingChanges, setPendingChanges] = React.useState(null);
  const [pendingMonitoringJob, setPendingMonitoringJob] = React.useState(null);

  const { deleteJob, toggleJobMonitoring, killJobProcess, mutate: jobsMutate } = useJobOperations();

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
      setEditedMonitored(currentJob.monitored || false);
    }
  }, [allJobs, initialJob, isEditing, isEditingCommand, isEditingSchedule, pendingChanges]);

  // Update parent form state when local state changes
  React.useEffect(() => {
    if (isNew) {
      // Find the selected crontab to get display name
      const selectedCrontab = crontabs.find(c => c.filename === editedCronFile);
      
      onFormChange({
        ...initialJob,
        name: editedName,
        expression: editedSchedule,
        command: editedCommand,
        crontab_filename: editedCronFile || initialJob.crontab_filename,
        crontab_display_name: selectedCrontab?.display_name || initialJob.crontab_display_name,
        run_as_user: editedRunAsUser,
        is_draft: false,
        suspended: initialJob.suspended,
        monitored: initialJob.monitored
      });
    }
  }, [isNew, onFormChange, editedName, editedSchedule, editedCommand, editedCronFile, editedRunAsUser, initialJob.monitored, initialJob, crontabs]);

  const handleFormChange = (field, value) => {
    const newData = { ...initialJob, [field]: value };
    // Update local state for the monitored field
    if (field === 'monitored') {
      setEditedMonitored(value);
    }
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
        monitored: editedMonitored
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
        const response = await csrfFetch('/api/jobs', {
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
        if (mutateCrontabs) {
          setTimeout(async () => {
            await mutateCrontabs();
            // Update the selected crontab if we have it
            if (selectedCrontab && setSelectedCrontab) {
              const updatedCrontabs = await mutateCrontabs();
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
      }, 1500);
    } catch (error) {
      setSavingStatus(null);
      setPendingChanges(null); // Clear pending changes on error
      showToast('Failed to update job: ' + error.message);
    }
  };

  const handleSuspendedSubmit = async () => {
    try {
      const response = await csrfFetch(`/api/jobs/${initialJob.id}/suspend`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ reason: suspendedReason }),
      });

      if (!response.ok) {
        throw new Error('Failed to suspend job');
      }

      setShowSuspendedDialog(false);
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
      const response = await csrfFetch(`/api/jobs/${initialJob.id}/schedule`, {
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
    
    const selectedCronFile = crontabs.find(file => file.filename === location);
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
      // Immediately invalidate SWR cache to show updated process list
      mutate();      
      await killJobProcess(pids);
      if (onJobChange) onJobChange();
      showToast(`Successfully killed ${pids.length} process${pids.length > 1 ? 'es' : ''}`, 'success');
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
    // Trigger immediate mutate when button is clicked
    mutate();
    if (onJobChange) onJobChange();
    
    try {
      const response = await csrfFetch('/api/jobs/run', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ 
          command: initialJob.command,
          crontab_filename: initialJob.crontab_filename,
          key: initialJob.key
        })
      });
      if (!response.ok) {
        throw new Error('Failed to run job');
      }
      showToast('Job started successfully', 'success');
      // Mutate again after successful API call to ensure data consistency
      mutate();
      if (onJobChange) onJobChange();
    } catch (error) {
      console.error('Error running job:', error);
      showToast('Failed to run job: ' + error.message, 'error');
      // Mutate once more on error to refresh state
      mutate();
    }
  };

  const handleToggleSuspendedOverlay = () => {
    setShowSuspendedOverlay(!showSuspendedOverlay);
  };

  const handleHideJob = async () => {
    setShowHideOverlay(false); // Close overlay first
    
    // Get the current job state from allJobs to ensure we have latest data
    const currentJob = allJobs.find(j => j.key === initialJob.key) || initialJob;
    
    try {
      // For existing jobs, use direct API call
      const requestBody = {
        key: currentJob.key,
        code: currentJob.code,
        command: currentJob.command,
        expression: currentJob.expression,
        name: currentJob.name,
        crontab_filename: currentJob.crontab_filename,
        run_as_user: currentJob.run_as_user,
        monitored: currentJob.monitored,
        suspended: currentJob.suspended,
        ignored: true
      };
      
      const response = await csrfFetch('/api/jobs', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(requestBody),
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`Failed to hide job: ${response.status} ${response.statusText}${errorText ? ` - ${errorText}` : ''}`);
      }
      
      // Refresh data from server
      mutate();
      if (onJobChange) {
        setTimeout(onJobChange, 100);
      }
      
      // Also refresh crontab data since hiding modifies the crontab
      if (mutateCrontabs) {
        setTimeout(async () => {
          await mutateCrontabs();
          if (selectedCrontab && setSelectedCrontab) {
            const updatedCrontabs = await mutateCrontabs();
            if (updatedCrontabs) {
              const updatedCrontab = updatedCrontabs.find(c => c.filename === selectedCrontab.filename);
              if (updatedCrontab) {
                setSelectedCrontab(updatedCrontab);
              }
            }
          }
        }, 150);
      }
      
      showToast('Job hidden successfully', 'success');
    } catch (error) {
      console.error('Error hiding job:', error);
      showToast('Failed to hide job: ' + error.message);
    }
  };

  const handleToggleSuspend = async (explicitSuspendedState, explicitPauseHours) => {
    setShowSuspendedOverlay(false); // Close overlay first
    
    // Get the current job state from allJobs to ensure we have latest data
    const currentJob = allJobs.find(j => j.key === initialJob.key) || initialJob;
    
    // Use the explicit state if provided, otherwise toggle the current state
    // Convert to explicit boolean with Boolean()
    const newSuspendedState = explicitSuspendedState !== undefined 
      ? Boolean(explicitSuspendedState) 
      : !currentJob.suspended;
    
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
        await onSave(updatedJob, true); // Pass true to indicate it's a suspend/resume toggle
      } else {
        // For existing jobs, use direct API call instead of toggleJobSuspension
        // This bypasses any potential issues with the toggleJobSuspension function
        const job = allJobs.find(j => j.key === currentJob.key);
        if (!job) {
          throw new Error('Job not found');
        }
        
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
        
        const response = await csrfFetch('/api/jobs', {
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
      }
      
      // Refresh data from server with the changes we just made
      mutate();
      if (onJobChange) {
        // Add a small delay to ensure the server has processed the change
        setTimeout(onJobChange, 100);
      }
      
      // Also refresh crontab data since suspension modifies the crontab
      if (mutateCrontabs) {
        setTimeout(async () => {
          await mutateCrontabs();
          // Update the selected crontab if we have it
          if (selectedCrontab && setSelectedCrontab) {
            const updatedCrontabs = await mutateCrontabs();
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

  // Check if there are actual changes to save
  const hasChanges = () => {
    const currentJob = allJobs.find(j => j.key === initialJob.key) || initialJob;
    
    // Compare edited values with current job values
    const nameChanged = editedName !== (currentJob.name || currentJob.default_name);
    const commandChanged = editedCommand !== currentJob.command;
    const scheduleChanged = editedSchedule !== (currentJob.expression || '');
    const monitoringChanged = editedMonitored !== (currentJob.monitored || false);
    
    return nameChanged || commandChanged || scheduleChanged || monitoringChanged;
  };

  return (
    <div className={`bg-white dark:bg-gray-800 shadow rounded-lg p-4 relative group ${initialJob.suspended ? 'bg-gray-100 dark:bg-gray-700' : ''}`}>
      {/* Hide button - positioned outside card on hover */}
      {!readOnly && !isNew && (
        <button
          onClick={() => setShowHideOverlay(true)}
          className="absolute -right-24 top-1/2 -translate-y-1/2 opacity-0 group-hover:opacity-100 hover:opacity-100 transition-opacity duration-200 px-3 py-3 text-base rounded-md bg-gray-100 dark:bg-gray-900 flex items-center gap-1 text-gray-400 dark:text-gray-600 hover:text-gray-600 dark:hover:text-gray-400"
        >
          Hide
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.878 9.878L3 3m6.878 6.878L21 21" />
          </svg>
        </button>
      )}
      
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
              // Auto-save when leaving the field only if there are changes
              if (hasChanges()) {
                handleSave();
              }
            }
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              // Check validation for new jobs before saving
              if (isNew && (!editedName || !editedSchedule || !editedCommand || !editedCronFile || (!editedCronFile.startsWith('user') && !editedRunAsUser))) {
                return;
              }
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
                      setEditedSchedule(value);
                    }}
                    onEditStart={() => {
                      setIsEditingSchedule(true);
                      setEditedSchedule(getDisplayValue('expression') || '');
                    }}
                    onEditEnd={() => {
                      if (!isNew) {
                        setIsEditingSchedule(false);
                        // Auto-save when leaving the field only if there are changes
                        if (hasChanges()) {
                          handleSave();
                        }
                      }
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        // Check validation for new jobs before saving
                        if (isNew && (!editedName || !editedSchedule || !editedCommand || !editedCronFile || (!editedCronFile.startsWith('user') && !editedRunAsUser))) {
                          return;
                        }
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
                        // Auto-save when leaving the field only if there are changes
                        if (hasChanges()) {
                          handleSave();
                        }
                      }
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        // Check validation for new jobs before saving
                        if (isNew && (!editedName || !editedSchedule || !editedCommand || !editedCronFile || (!editedCronFile.startsWith('user') && !editedRunAsUser))) {
                          return;
                        }
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
                  <div className="flex items-start justify-between">
                    <div className="flex-1">
                      <ScheduleSection
                        job={{
                          ...initialJob,
                          expression: getDisplayValue('expression') || ''
                        }}
                        isEditing={isEditingSchedule}
                        editedSchedule={editedSchedule}
                        onScheduleChange={(value) => {
                          setEditedSchedule(value);
                        }}
                        onEditStart={() => {
                          setIsEditingSchedule(true);
                          setEditedSchedule(getDisplayValue('expression') || '');
                        }}
                        onEditEnd={() => {
                          if (!isNew) {
                            setIsEditingSchedule(false);
                            // Auto-save when leaving the field only if there are changes
                            if (hasChanges()) {
                              handleSave();
                            }
                          }
                        }}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') {
                            e.preventDefault();
                            // Check validation for new jobs before saving
                            if (isNew && (!editedName || !editedSchedule || !editedCommand || !editedCronFile || (!editedCronFile.startsWith('user') && !editedRunAsUser))) {
                              return;
                            }
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
                    </div>
                    {!readOnly && (
                      <button
                        onClick={() => setShowConsole(true)}
                        className="flex-shrink-0 ml-4 text-sm text-gray-900 hover:text-black dark:text-gray-300 dark:hover:text-white"
                      >
                        Console
                      </button>
                    )}
                  </div>
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
                  {isNew && selectedLocation && !isUserCrontab ? 'User' : ''}
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
              <tr>
                <td className="py-2 text-sm text-gray-900 dark:text-gray-100" style={{ width: '25%' }}>
                  <MonitoringSection
                    job={{...initialJob, monitored: editedMonitored}}
                    onUpdate={async (updatedJob) => {
                                              // Apply optimistic updates immediately BEFORE any async operations
                        handleFormChange('suspended', updatedJob.suspended);
                        handleFormChange('monitored', updatedJob.monitored);
                        handleFormChange('pause_hours', updatedJob.pause_hours);
                        
                        if (isNew) {
                          onFormChange(updatedJob);
                        } else {
                          // Mark job as pending monitoring BEFORE the API call if monitoring is being enabled
                          if (updatedJob.monitored && !editedMonitored && setPendingMonitoringJobs) {
                            setPendingMonitoringJobs(prev => new Set(prev).add(initialJob.key));
                          }
                          
                          // Apply optimistic update to the jobs list immediately
                          const optimisticData = allJobs.map(j => 
                            j.key === initialJob.key ? {...j, monitored: updatedJob.monitored} : j
                          );
                          mutate(optimisticData, false); // false = don't revalidate yet
                          
                          try {
                            await toggleJobMonitoring(initialJob.key, updatedJob.monitored);
                            // Now revalidate with the server data
                            mutate();
                            if (onJobChange) onJobChange();
                                                  } catch (error) {
                            // Remove from pending if the update failed
                            if (setPendingMonitoringJobs) {
                              setPendingMonitoringJobs(prev => {
                                const next = new Set(prev);
                                next.delete(initialJob.key);
                                return next;
                              });
                            }
                            // Revert the optimistic update on error
                            mutate();
                            showToast('Failed to update monitoring status: ' + error.message);
                          }
                      }
                    }}
                    onShowLearnMore={() => {
                      setPendingMonitoringJob(initialJob);
                      setShowLearnMore(true);
                    }}
                    settings={settings}
                    monitorsLoading={monitorsLoading}
                  />
                </td>
                <td className="py-2 text-sm text-gray-900 dark:text-gray-100" style={{ width: '37.5%' }}>
                  <LocationSection
                    job={initialJob}
                    isNew={isNew}
                    crontabs={crontabs}
                    users={users}
                    selectedLocation={selectedLocation}
                    selectedUser={selectedUser}
                    isUserCrontab={isUserCrontab}
                    onLocationChange={handleLocationChange}
                    onUserChange={handleUserChange}
                    isMacOS={isMacOS}
                  />
                </td>
                <td className="py-2 text-sm text-gray-900 dark:text-gray-100" style={{ width: '37.5%' }}>
                  {isNew && selectedLocation && !isUserCrontab && (
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
                  )}
                </td>
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

        {showHideOverlay && (
          <HideOverlay
            job={initialJob}
            onClose={() => setShowHideOverlay(false)}
            onHideJob={handleHideJob}
          />
        )}

        {showDeleteConfirmation && (
          <DeleteConfirmation
            onClose={() => setShowDeleteConfirmation(false)}
            onDelete={async () => {
              if (deleteConfirmation.toLowerCase() !== 'delete') return;
              try {
                await deleteJob(initialJob.key);
                mutateCrontabs(); // Also refresh crontabs cache since deleting a job modifies the crontab
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
          <LearnMoreModal
            isOpen={showLearnMore}
            onClose={() => {
              setShowLearnMore(false);
              setPendingMonitoringJob(null);
            }}
            onSignupSuccess={async (apiKeys) => {
              setShowLearnMore(false);
              showToast('Account created! Enabling monitoring...', 'success');
              
              // If we have a pending monitoring job, enable monitoring for it
              if (pendingMonitoringJob) {
                try {
                  // For new jobs, just update the form
                  if (isNew) {
                    handleFormChange('monitored', true);
                    onFormChange({
                      ...initialJob,
                      monitored: true
                    });
                                      } else {
                      // Mark job as pending monitoring BEFORE the API call
                      if (setPendingMonitoringJobs) {
                        setPendingMonitoringJobs(prev => new Set(prev).add(pendingMonitoringJob.key));
                      }
                      
                      // Apply optimistic update to show monitoring as enabled immediately
                      const optimisticData = allJobs.map(j => 
                        j.key === pendingMonitoringJob.key ? {...j, monitored: true} : j
                      );
                      mutate(optimisticData, false); // false = don't revalidate yet
                      
                      // For existing jobs, make the API call
                      const response = await csrfFetch('/api/jobs', {
                      method: 'PUT',
                      headers: {
                        'Content-Type': 'application/json',
                      },
                      body: JSON.stringify({
                        ...pendingMonitoringJob,
                        monitored: true
                      }),
                    });

                                          if (!response.ok) {
                        // Remove from pending if the update failed
                        if (setPendingMonitoringJobs) {
                          setPendingMonitoringJobs(prev => {
                            const next = new Set(prev);
                            next.delete(pendingMonitoringJob.key);
                            return next;
                          });
                        }
                        // Revert the optimistic update on error
                        mutate();
                        throw new Error('Failed to enable monitoring');
                      }
                      
                      // Refresh the jobs list with server data
                      mutate();
                      if (onJobChange) onJobChange();
                  }
                  
                  showToast('Monitoring enabled successfully!', 'success');
                } catch (error) {
                  showToast('Failed to enable monitoring: ' + error.message, 'error');
                }
              }
              
              // Reload to refresh settings with new API key
              setTimeout(() => {
                window.location.reload();
              }, 1500);
            }}
            showToast={showToast}
            settings={settings}
          />
        )}

        {(isNew || isEditing || isEditingCommand || isEditingSchedule) && (
          <div className="mt-4 flex justify-end space-x-4">
            <button
              type="button"
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => {
                if (isNew) {
                  onDiscard();
                } else {
                  // For existing jobs, reset all editing states and values
                  setIsEditing(false);
                  setIsEditingCommand(false);
                  setIsEditingSchedule(false);
                  setPendingChanges(null);
                  
                  // Reset to current values
                  const currentJob = allJobs.find(j => j.key === initialJob.key) || initialJob;
                  setEditedName(currentJob.name || currentJob.default_name);
                  setEditedCommand(currentJob.command);
                  setEditedSchedule(currentJob.expression || '');
                  setEditedMonitored(currentJob.monitored || false);
                }
              }}
              className="text-sm text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
            >
              Discard
            </button>
            <button
              type="button"
              onMouseDown={(e) => e.preventDefault()}
              onClick={handleSave}
              disabled={isNew && (!editedName || !editedSchedule || !editedCommand || !editedCronFile || (!editedCronFile.startsWith('user') && !editedRunAsUser))}
              className={`px-4 py-2 text-sm font-medium text-white rounded-md ${
                isNew && (!editedName || !editedSchedule || !editedCommand || !editedCronFile || (!editedCronFile.startsWith('user') && !editedRunAsUser))
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