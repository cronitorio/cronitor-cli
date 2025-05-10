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

export function JobCard({ job: initialJob, mutate, allJobs, isNew = false, onSave, onDiscard, onFormChange, showToast }) {
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
  const [job, setJob] = React.useState(initialJob);

  const { jobs, createJob, updateJob, deleteJob, toggleJobMonitoring, toggleJobSuspension, killJobProcess } = useJobOperations();

  // Get the current job data from allJobs and update local state
  React.useEffect(() => {
    const currentJob = allJobs.find(j => j.key === initialJob.key) || initialJob;
    setJob(currentJob);
  }, [allJobs, initialJob]);

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

  const handleSave = async () => {
    setSavingStatus('saving');
    try {
      await updateJob({
        ...job,
        name: isEditing ? editedName : job.name,
        command: isEditingCommand ? editedCommand : job.command,
        expression: isEditingSchedule ? editedSchedule : job.expression,
      });
      setSavingStatus('saved');
      setTimeout(() => {
        if (isEditing) setIsEditing(false);
        if (isEditingCommand) setIsEditingCommand(false);
        if (isEditingSchedule) setIsEditingSchedule(false);
      }, 100);
    } catch (error) {
      setSavingStatus(null);
      showToast('Failed to update job: ' + error.message);
    }
  };

  const handleKill = async (pids, isAll = false) => {
    setKillingPids(prev => new Set([...prev, ...pids]));
    if (isAll) {
      setIsKillingAll(true);
    }
    try {
      await killJobProcess(pids);
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

  const handleLocationChange = (e) => {
    const location = e.target.value;
    setSelectedLocation(location);
    setEditedCronFile(location);
    
    if (location === "/etc/cron.d") {
      setIsUserCrontab(false);
      setSelectedUser('');
      const timezone = 'UTC';
      setJob(prev => ({ ...prev, timezone }));
      onFormChange(prev => ({
        ...prev,
        timezone,
        crontab_filename: location
      }));
      return;
    }
    
    const selectedCronFile = cronFiles.find(file => file.filename === location);
    if (selectedCronFile) {
      setIsUserCrontab(selectedCronFile.isUserCrontab);
      setSelectedUser('');
      const timezone = selectedCronFile.timezone || 'UTC';
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

  return (
    <div className={`bg-white dark:bg-gray-800 shadow rounded-lg p-4 relative ${job.suspended ? 'bg-gray-100 dark:bg-gray-700' : ''}`}>
      <StatusBadges
        job={job}
        instances={job.instances || []}
        showInstances={showInstances}
        onToggleInstances={() => setShowInstances(!showInstances)}
        onToggleSuspended={() => setShowSuspendedOverlay(true)}
      />

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
            onClick={handleSave}
          >
            {savingStatus === 'saving' ? 'Saving' : savingStatus === 'saved' ? 'Saved' : 'Editing'}
          </div>
        </div>
      )}

      <div className="space-y-2">
        <JobHeader
          job={job}
          isEditing={isEditing}
          editedName={editedName}
          onNameChange={setEditedName}
          onEditStart={() => setIsEditing(true)}
          onEditEnd={() => setIsEditing(false)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              handleSave();
            } else if (e.key === 'Escape') {
              setIsEditing(false);
              setEditedName(job.name || job.default_name);
            }
          }}
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
                    job={job}
                    isEditing={isEditingSchedule}
                    editedSchedule={editedSchedule}
                    onScheduleChange={setEditedSchedule}
                    onEditStart={() => {
                      setIsEditingSchedule(true);
                      setEditedSchedule(job.expression || '');
                    }}
                    onEditEnd={() => setIsEditingSchedule(false)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        handleSave();
                      } else if (e.key === 'Escape') {
                        setIsEditingSchedule(false);
                        setEditedSchedule(job.expression || '');
                      }
                    }}
                    showDescription={false}
                  />
                </td>
                <td className="py-2 text-sm text-gray-900 dark:text-gray-100">
                  <CommandSection
                    job={job}
                    isEditing={isEditingCommand}
                    editedCommand={editedCommand}
                    onCommandChange={setEditedCommand}
                    onEditStart={() => setIsEditingCommand(true)}
                    onEditEnd={() => setIsEditingCommand(false)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        handleSave();
                      } else if (e.key === 'Escape') {
                        setIsEditingCommand(false);
                        setEditedCommand(job.command);
                      }
                    }}
                    onShowConsole={() => setShowConsole(true)}
                  />
                </td>
              </tr>
              <tr>
                <td colSpan="2" className="py-2 text-sm text-gray-900 dark:text-gray-100">
                  <ScheduleSection
                    job={job}
                    isEditing={isEditingSchedule}
                    editedSchedule={editedSchedule}
                    onScheduleChange={setEditedSchedule}
                    onEditStart={() => {
                      setIsEditingSchedule(true);
                      setEditedSchedule(job.expression || '');
                    }}
                    onEditEnd={() => setIsEditingSchedule(false)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        handleSave();
                      } else if (e.key === 'Escape') {
                        setIsEditingSchedule(false);
                        setEditedSchedule(job.expression || '');
                      }
                    }}
                    showDescription={true}
                  />
                </td>
              </tr>
            </tbody>
          </table>
        </div>

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
                  <MonitoringSection
                    job={job}
                    onUpdate={async (updatedJob) => {
                      try {
                        await toggleJobMonitoring(job.key, updatedJob.is_monitored);
                      } catch (error) {
                        showToast('Failed to update monitoring status: ' + error.message);
                      }
                    }}
                    onShowLearnMore={() => setShowLearnMore(true)}
                  />
                </td>
                <td className="py-2 text-sm text-gray-900 dark:text-gray-100">
                  <LocationSection
                    job={job}
                    isNew={isNew}
                    cronFiles={cronFiles}
                    users={users}
                    selectedLocation={selectedLocation}
                    selectedUser={selectedUser}
                    isUserCrontab={isUserCrontab}
                    onLocationChange={handleLocationChange}
                    onUserChange={handleUserChange}
                  />
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        {showInstances && (
          <InstancesTable
            instances={job.instances || []}
            killingPids={killingPids}
            isKillingAll={isKillingAll}
            onKillInstance={(pid) => handleKill([pid])}
            onKillAll={() => handleKill((job.instances || []).map(i => i.pid), true)}
          />
        )}

        {showSuspendedOverlay && (
          <SuspendOverlay
            job={job}
            allJobs={allJobs}
            onClose={() => setShowSuspendedOverlay(false)}
            onToggleSuspension={async () => {
              try {
                await toggleJobSuspension(job.key, !job.suspended);
                setShowSuspendedOverlay(false);
              } catch (error) {
                showToast('Failed to update job status: ' + error.message);
              }
            }}
            onShowDeleteConfirmation={() => {
              setShowSuspendedOverlay(false);
              setShowDeleteConfirmation(true);
            }}
            mutate={mutate}
          />
        )}

        {showDeleteConfirmation && (
          <DeleteConfirmation
            onClose={() => setShowDeleteConfirmation(false)}
            onDelete={async () => {
              if (deleteConfirmation.toLowerCase() !== 'delete') return;
              try {
                await deleteJob(job.key);
                setShowDeleteConfirmation(false);
                setDeleteConfirmation('');
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
            job={job}
            onClose={() => setShowConsole(false)}
            isNew={isNew}
            onFormChange={onFormChange}
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
    </div>
  );
} 