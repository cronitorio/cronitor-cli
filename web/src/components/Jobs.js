import React from 'react';
import useSWR from 'swr';
import { JobCard } from './jobs/JobCard';
import { Toast } from './Toast';
import { NewCrontabOverlay } from './jobs/NewCrontabOverlay';
import { useSearchParams, useLocation, Link } from 'react-router-dom';
import { FilterBar, FILTER_OPTIONS } from './jobs/FilterBar';
import { csrfFetcher, csrfFetch } from '../utils/api';

export default function Jobs() {
  const location = useLocation();
  const isJobsView = location.pathname === '/' || location.pathname === '/jobs';
  
  // Primary data for Jobs view - fast refresh
  const { data: jobs, error, mutate } = useSWR('/api/jobs', csrfFetcher, {
    refreshInterval: 5000,
    revalidateOnFocus: true
  });
  const { data: monitors } = useSWR('/api/monitors', csrfFetcher, {
    refreshInterval: 5000,
    revalidateOnFocus: true
  });
  
  // Settings data - minimal refresh
  const { data: settings } = useSWR('/api/settings', csrfFetcher, {
    revalidateOnFocus: true,
    refreshInterval: 0
  });
  
  // User data - minimal refresh (rarely changes)
  const { data: users } = useSWR('/api/users', csrfFetcher, {
    refreshInterval: 0, // No auto-refresh - users rarely change
    revalidateOnFocus: true, // Revalidate when window gets focus
    revalidateOnReconnect: false,
    dedupingInterval: 300000 // 5 minutes deduplication
  });
  
  // Crontabs data - slower refresh since it's secondary on Jobs view
  const { data: crontabs, mutate: mutateCrontabs } = useSWR('/api/crontabs', csrfFetcher, {
    refreshInterval: isJobsView ? 30000 : 5000, // 30s on Jobs view, 5s elsewhere
    revalidateOnFocus: true,
    revalidateOnReconnect: false,
    dedupingInterval: isJobsView ? 30000 : 5000 // Match refresh interval
  });

  const [searchParams, setSearchParams] = useSearchParams();
  const [showNewJob, setShowNewJob] = React.useState(false);
  const [showNewCrontab, setShowNewCrontab] = React.useState(false);
  const [newCrontabForm, setNewCrontabForm] = React.useState({
    filename: '',
    timezone: '',
    comments: ''
  });
  const [newJobForm, setNewJobForm] = React.useState({
    name: '',
    expression: '',
    command: '',
    crontab_filename: '',
    run_as_user: '',
    monitored: false,
    is_draft: true
  });
  const [isToastVisible, setIsToastVisible] = React.useState(false);
  const [toastMessage, setToastMessage] = React.useState('');
  const [toastType, setToastType] = React.useState('error');
  const [isReloading, setIsReloading] = React.useState(false);
  
  const [inputValue, setInputValue] = React.useState(searchParams.get('search') || '');
  const [searchTerm, setSearchTerm] = React.useState(searchParams.get('search') || '');

  const [activeFilters, setActiveFilters] = React.useState(() => {
    const initialFilters = {};
    FILTER_OPTIONS.forEach(filter => {
      initialFilters[filter.id] = searchParams.get(filter.id) === 'true';
    });
    return initialFilters;
  });

  React.useEffect(() => {
    const searchFromURL = searchParams.get('search') || '';
    
    // Update inputValue if it differs from URL
    setInputValue(prev => {
      if (searchFromURL !== prev) {
        return searchFromURL;
      }
      return prev;
    });
    
    // Update activeFilters from URL as well, in case of back/forward navigation
    setActiveFilters(prev => {
      let filtersChanged = false;
      const newFiltersFromURL = { ...prev };
      FILTER_OPTIONS.forEach(filter => {
        const paramValue = searchParams.get(filter.id) === 'true';
        if (newFiltersFromURL[filter.id] !== paramValue) {
          newFiltersFromURL[filter.id] = paramValue;
          filtersChanged = true;
        }
      });
      return filtersChanged ? newFiltersFromURL : prev;
    });
  }, [searchParams]);

  React.useEffect(() => {
    const handler = setTimeout(() => {
      if (inputValue !== searchTerm) {
        setSearchTerm(inputValue);
      }
    }, 300);
    return () => clearTimeout(handler);
  }, [inputValue, searchTerm]);

  React.useEffect(() => {
    const newParams = new URLSearchParams(); 
    Object.entries(activeFilters).forEach(([key, value]) => {
      if (value === true) {
        newParams.set(key, 'true');
      }
    });

    if (searchTerm) {
      newParams.set('search', searchTerm);
    } else {
      newParams.delete('search');
    }
    
    // Only update if the string form of params actually changes
    // This prevents loops if searchParams was in the dependency array of this effect.
    if (newParams.toString() !== searchParams.toString()) {
        setSearchParams(newParams, { replace: true });
    }
  }, [activeFilters, searchTerm, setSearchParams, searchParams]);
  
  // Update timezone when settings are loaded
  React.useEffect(() => {
    if (settings?.timezone) {
      setNewCrontabForm(prev => ({
        ...prev,
        timezone: settings.timezone
      }));
    }
  }, [settings]);

  const showToast = (message, type = 'error') => {
    setToastMessage(message);
    setToastType(type);
    setIsToastVisible(true);
    setTimeout(() => setIsToastVisible(false), type === 'error' ? 6000 : 3000);
  };

  const handleReload = async () => {
    setIsReloading(true);
    await mutate();
    // Ensure loading state shows for at least 1 second
    setTimeout(() => setIsReloading(false), 1000);
  };

  const handleCreateCrontab = async () => {
    try {
      if (!newCrontabForm.timezone && settings?.timezone) {
        // Ensure timezone is set even if the form wasn't updated
        setNewCrontabForm(prev => ({
          ...prev,
          timezone: settings.timezone
        }));
      }
      
      const response = await csrfFetch('/api/crontabs', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          filename: newCrontabForm.filename,
          TimezoneLocationName: {
            Name: newCrontabForm.timezone
          }
        }),
      });

      if (!response.ok) {
        throw new Error('Failed to create crontab');
      }

      const crontab = await response.json();
      
      // Update the job form with the new crontab details
      setNewJobForm(prev => ({
        ...prev,
        crontab_filename: crontab.filename,
        crontab_display_name: crontab.display_name,
        timezone: crontab.timezone,
        run_as_user: crontab.is_user_crontab ? crontab.user : ''
      }));

      setShowNewCrontab(false);
      showToast(
        response.status === 201 ? 'Crontab Created' : 'Crontab Already Exists',
        'success'
      );
    } catch (error) {
      console.error('Error creating crontab:', error);
      showToast('Failed to create crontab: ' + error.message);
    }
  };

  const handleLocationChange = (location) => {
    // Handle both direct string values and event objects
    const selectedLocation = typeof location === 'string' ? location : location.target.value;

    if (selectedLocation === '/etc/cron.d (New Crontab)') {
      setShowNewCrontab(true);
      return;
    }

    // Handle existing crontab selection
    const isUserCrontab = selectedLocation.startsWith('user:');
    setNewJobForm(prev => ({
      ...prev,
      crontab_filename: selectedLocation,
      crontab_display_name: isUserCrontab ? `User ${selectedLocation.split(':')[1]} Crontab` : selectedLocation,
      run_as_user: isUserCrontab ? selectedLocation.split(':')[1] : ''
    }));
  };

  // Merge monitor data with jobs
  const jobsWithMonitors = React.useMemo(() => {
    if (!jobs || !monitors) return jobs || [];
    
    return jobs.map(job => {
      // If job has a code (should be monitored) but no monitor found, treat as unmonitored
      if (job.code && job.monitored) {
        const monitor = monitors.find(m => m.key === job.key || m.attributes?.code === job.code);
        if (!monitor) {
          // Monitor not found in response, treat as unmonitored
          return {
            ...job,
            monitored: false,
            passing: false,
            disabled: false,
            paused: false,
            initialized: false
          };
        }
        
        // Monitor found, merge its data
        return {
          ...job,
          name: monitor.name || job.name,
          passing: monitor.passing,
          disabled: monitor.disabled,
          paused: monitor.paused,
          initialized: monitor.initialized
        };
      }
      
      // Job not monitored, return as-is
      return job;
    });
  }, [jobs, monitors]);

  // Filter jobs based on active filters and search term
  const filteredJobs = React.useMemo(() => {
    if (!jobsWithMonitors || jobsWithMonitors.length === 0) return [];
    
    return jobsWithMonitors.filter(job => {
      // Exclude meta cron jobs (system plumbing jobs like run-parts) from Jobs view
      if (job.is_meta_cron_job === true || job.is_meta_cron_job === "true") {
        return false;
      }

      // Group 1: Status (Active/Suspended)
      if (activeFilters.active && job.suspended) return false;
      if (activeFilters.suspended && !job.suspended) return false;

      // Group 2: Running
      if (activeFilters.running && !(job.instances && job.instances.length > 0)) return false;

      // Group 3: Monitoring (Monitored/Unmonitored)
      if (activeFilters.monitored && !job.monitored) return false;
      if (activeFilters.unmonitored && job.monitored) return false;
      
      // Check if job matches (debounced) search term
      if (searchTerm) {
        const term = searchTerm.toLowerCase();
        const nameMatch = job.name?.toLowerCase().includes(term);
        const commandMatch = job.command?.toLowerCase().includes(term);
        const expressionMatch = job.expression?.toLowerCase().includes(term);
        if (!(nameMatch || commandMatch || expressionMatch)) {
          return false;
        }
      }
      
      return true;
    });
  }, [jobsWithMonitors, activeFilters, searchTerm]);

  if (error) return (
    <div className="p-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800">
      <div className="flex">
        <div className="flex-shrink-0">
          <svg className="h-5 w-5 text-red-400" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
          </svg>
        </div>
        <div className="ml-3 flex-1">
          <h3 className="text-sm font-medium text-red-800 dark:text-red-200">Failed to load jobs</h3>
          <div className="mt-2 text-sm text-red-700 dark:text-red-300">
            <pre className="whitespace-pre-wrap break-words">{error.message}</pre>
          </div>
          {error.message.includes('Unable to connect') && (
            <div className="mt-2 text-sm text-red-700 dark:text-red-300">
              <p>Possible causes:</p>
              <ul className="list-disc list-inside mt-1">
                <li>The dash server is not running</li>
                <li>Your IP is not whitelisted</li>
                <li>A VPN connection is required</li>
                <li>Server is restarting</li>
              </ul>
            </div>
          )}
          <div className="mt-4">
            <button
              onClick={handleReload}
              disabled={isReloading}
              className="inline-flex items-center px-4 py-2 border border-gray-300 dark:border-gray-600 text-sm font-medium rounded-md shadow-sm text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-800 hover:bg-gray-50 dark:hover:bg-gray-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isReloading ? (
                <>
                  <svg className="animate-spin -ml-1 mr-2 h-4 w-4 text-gray-500 dark:text-gray-400" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                  </svg>
                  Reloading...
                </>
              ) : (
                <>
                  <svg className="mr-2 -ml-1 h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                  </svg>
                  Reload
                </>
              )}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
  if (jobs === undefined) return <div className="text-gray-600 dark:text-gray-300">Loading...</div>;

  const handleSaveNewJob = async () => {
    try {
      const response = await csrfFetch('/api/jobs', {
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
        crontab_display_name: '',
        run_as_user: '',
        monitored: false,
        is_draft: true
      });
      mutate();
      mutateCrontabs(); // Also refresh crontabs cache since jobs are part of crontabs
      showToast('Job created successfully', 'success');
    } catch (error) {
      console.error('Error creating job:', error);
      showToast('Failed to create job: ' + error.message);
    }
  };

  return (
    <div>
      <div className="flex justify-between items-center mb-6">
        <div className="flex items-center gap-8 w-full overflow-hidden">
          <h1 className="text-2xl font-semibold text-gray-900 dark:text-white flex-shrink-0">Jobs</h1>
          <div className="flex-1 overflow-x-auto">
            <FilterBar 
              activeFilters={activeFilters} 
              setActiveFilters={setActiveFilters}
              inputValue={inputValue}
              onInputChange={e => setInputValue(e.target.value)}
            />
          </div>
          {!settings?.safe_mode && (
            <button
              onClick={() => setShowNewJob(true)}
              className="px-4 py-2.5 bg-blue-600 text-white rounded hover:bg-blue-700 font-medium flex-shrink-0"
            >
              Add Job
            </button>
          )}
        </div>
      </div>

      <div className="space-y-4">
        {showNewJob && (
          <div className="relative">
            <JobCard 
              job={newJobForm} 
              mutate={mutate} 
              mutateCrontabs={mutateCrontabs}
              allJobs={jobsWithMonitors} 
              isNew={true}
              onSave={handleSaveNewJob}
              onDiscard={() => {
                setShowNewJob(false);
                setNewJobForm({
                  name: '',
                  expression: '',
                  command: '',
                  crontab_filename: '',
                  crontab_display_name: '',
                  run_as_user: '',
                  monitored: false,
                  is_draft: true
                });
              }}
              onFormChange={setNewJobForm}
              onLocationChange={handleLocationChange}
              showToast={showToast}
              isMacOS={settings?.os === 'darwin'}
              readOnly={settings?.safe_mode}
              settings={settings}
              monitorsLoading={!monitors}
              users={users || []}
              crontabs={crontabs || []}
              key={showNewCrontab ? 'new-crontab' : 'no-crontab'}
            />
            {showNewCrontab && (
              <NewCrontabOverlay
                formData={newCrontabForm}
                onFormChange={setNewCrontabForm}
                onClose={() => {
                  setShowNewCrontab(false);
                  setNewJobForm(prev => ({
                    ...prev,
                    crontab_filename: '',
                    crontab_display_name: '',
                    run_as_user: ''
                  }));
                  handleLocationChange({ target: { value: '' } });
                }}
                onCreateCrontab={handleCreateCrontab}
                timezones={settings?.timezones}
              />
            )}
          </div>
        )}
        {filteredJobs.length > 0 ? (
          filteredJobs.map((job, index) => (
            <JobCard 
              key={index} 
              job={job} 
              mutate={mutate} 
              mutateCrontabs={mutateCrontabs}
              allJobs={jobsWithMonitors} 
              showToast={showToast}
              isMacOS={settings?.os === 'darwin'}
              readOnly={settings?.safe_mode}
              settings={settings}
              monitorsLoading={!monitors}
              users={users || []}
              crontabs={crontabs || []}
            />
          ))
        ) : (
          <div>
            {jobsWithMonitors.length > 0 ? (
              <div className="bg-white dark:bg-gray-800 shadow rounded-lg p-8 text-center">
                <p className="text-gray-600 dark:text-gray-300">
                  No jobs match your current filters
                </p>
              </div>
            ) : (
              <div className="p-4 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800">
                <div className="flex">
                  <div className="flex-shrink-0">
                    <svg className="h-5 w-5 text-yellow-400" viewBox="0 0 20 20" fill="currentColor">
                      <path fillRule="evenodd" d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.17 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 5zm0 9a1 1 0 100-2 1 1 0 000 2z" clipRule="evenodd" />
                    </svg>
                  </div>
                  <div className="ml-3 flex-1">
                    <h3 className="text-sm font-medium text-yellow-800 dark:text-yellow-200">No cron jobs found</h3>
                    <div className="mt-2 text-sm text-yellow-700 dark:text-yellow-300">
                      <p>This could mean:</p>
                      <ul className="list-disc list-inside mt-1 space-y-1">
                        <li>You don't have any cron jobs</li>
                        <li>You are not scanning the cron users on this host</li>
                        <li>This dashboard does not have permissions to manage crontabs for the cron users on this host</li>
                      </ul>
                      <p className="mt-3">
                        You can adjust which user crontabs are scanned from the{' '}
                        <Link 
                          to="/settings" 
                          className="font-medium text-yellow-800 dark:text-yellow-200 hover:text-yellow-900 dark:hover:text-yellow-100 underline"
                        >
                          Settings page
                        </Link>
                      </p>
                    </div>
                  </div>
                </div>
              </div>
            )}
          </div>
        )}
        {(() => {
          // Only count non-meta jobs as potentially visible
          const nonMetaJobs = (jobsWithMonitors || []).filter(job => !job.is_meta_cron_job);
          
          // Apply only user filters to non-meta jobs (not the automatic meta filter)
          const nonMetaJobsAfterUserFilters = nonMetaJobs.filter(job => {
            // Group 1: Status (Active/Suspended)
            if (activeFilters.active && job.suspended) return false;
            if (activeFilters.suspended && !job.suspended) return false;

            // Group 2: Running
            if (activeFilters.running && !(job.instances && job.instances.length > 0)) return false;

            // Group 3: Monitoring (Monitored/Unmonitored)
            if (activeFilters.monitored && !job.monitored) return false;
            if (activeFilters.unmonitored && job.monitored) return false;
            
            // Check if job matches (debounced) search term
            if (searchTerm) {
              const term = searchTerm.toLowerCase();
              const nameMatch = job.name?.toLowerCase().includes(term);
              const commandMatch = job.command?.toLowerCase().includes(term);
              const expressionMatch = job.expression?.toLowerCase().includes(term);
              if (!(nameMatch || commandMatch || expressionMatch)) {
                return false;
              }
            }
            
            return true;
          });
          
          const hiddenCount = nonMetaJobs.length - nonMetaJobsAfterUserFilters.length;
          
          // Only show the "hidden jobs" section if there are actually jobs hidden by user filters
          return hiddenCount > 0 ? (
            <div className="bg-gray-50 dark:bg-gray-800/50 shadow-sm rounded-lg p-4 text-center">
              <p className="text-gray-500 dark:text-gray-400">
                {`${hiddenCount} Job${hiddenCount === 1 ? '' : 's'} Hidden`} {' '}
                <button
                  onClick={() => {
                    setActiveFilters({});
                    setInputValue('');
                  }}
                  className="ml-2 text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300 font-medium"
                >
                  Clear Filters
                </button>
              </p>
            </div>
          ) : null;
        })()}
      </div>
      {isToastVisible && <Toast message={toastMessage} onClose={() => setIsToastVisible(false)} type={toastType} />}
    </div>
  );
} 