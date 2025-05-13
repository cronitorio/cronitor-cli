import React from 'react';
import useSWR from 'swr';
import { JobCard } from './jobs/JobCard';
import { Toast } from './Toast';
import { Dialog } from '@headlessui/react';
import { XMarkIcon } from '@heroicons/react/24/outline';
import { CloseButton } from './CloseButton';
import { NewCrontabOverlay } from './jobs/NewCrontabOverlay';

const fetcher = async url => {
  try {
    const res = await fetch(url);
    if (!res.ok) {
      const errorText = await res.text();
      throw new Error(`Server error: ${res.status} ${res.statusText}${errorText ? ` - ${errorText}` : ''}`);
    }
    return res.json();
  } catch (error) {
    if (error.name === 'TypeError' && error.message === 'Failed to fetch') {
      throw new Error(`Unable to connect to the dash server.  Check that it's running and try again.`);
    }
    throw error;
  }
};

export default function Jobs() {
  const { data: jobs, error, mutate, isValidating } = useSWR('/api/jobs', fetcher, {
    refreshInterval: 5000,
    revalidateOnFocus: true
  });
  const { data: settings } = useSWR('/api/settings', fetcher);
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
    is_monitored: false,
    is_draft: true
  });
  const [isToastVisible, setIsToastVisible] = React.useState(false);
  const [toastMessage, setToastMessage] = React.useState('');
  const [toastType, setToastType] = React.useState('error');
  const [isReloading, setIsReloading] = React.useState(false);

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
    setTimeout(() => setIsToastVisible(false), 3000);
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
      
      const response = await fetch('/api/crontabs', {
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
    if (location === '/etc/cron.d (New Crontab)') {
      setShowNewCrontab(true);
      return;
    }

    // Handle existing crontab selection
    const isUserCrontab = location.startsWith('user:');
    setNewJobForm(prev => ({
      ...prev,
      crontab_filename: location,
      crontab_display_name: isUserCrontab ? `User ${location.split(':')[1]} Crontab` : location,
      run_as_user: isUserCrontab ? location.split(':')[1] : ''
    }));
  };

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
      showToast('Job created successfully', 'success');
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
          <div className="relative">
            <JobCard 
              job={newJobForm} 
              mutate={mutate} 
              allJobs={jobs} 
              isNew={true}
              onSave={handleSaveNewJob}
              onDiscard={() => setShowNewJob(false)}
              onFormChange={setNewJobForm}
              onLocationChange={handleLocationChange}
              showToast={showToast}
              isMacOS={settings?.os === 'darwin'}
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
        {jobs.map((job, index) => (
          <JobCard 
            key={index} 
            job={job} 
            mutate={mutate} 
            allJobs={jobs} 
            showToast={showToast}
            isMacOS={settings?.os === 'darwin'}
          />
        ))}
      </div>
      {isToastVisible && <Toast message={toastMessage} onClose={() => setIsToastVisible(false)} type={toastType} />}
    </div>
  );
} 