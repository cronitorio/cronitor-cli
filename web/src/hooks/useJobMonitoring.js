import { useState, useCallback } from 'react';

export function useJobMonitoring(job, onUpdate, monitorsLoading = false) {
  const [isLoading, setIsLoading] = useState(false);

  const handleToggle = useCallback(async () => {
    if (!job.key) {
      // For new jobs, just update the form state
      onUpdate({
        ...job,
        monitored: !job.monitored
      });
      return;
    }

    setIsLoading(true);
    
    try {
      await onUpdate({
        ...job,
        monitored: !job.monitored
      });
    } catch (error) {
      console.error('Error updating job status:', error);
    } finally {
      setIsLoading(false);
    }
  }, [job, onUpdate]);

  const getStatusInfo = useCallback(() => {
    // If monitors are still loading and job is monitored, show syncing
    if (monitorsLoading && job.monitored && job.key) {
      return {
        color: 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300',
        text: 'Syncing',
        title: 'Loading monitoring status...'
      };
    }

    if (job.disabled) {
      return {
        color: 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300',
        text: 'Upgrade to activate',
        title: 'You have exceeded your free tier limit. Upgrade to monitor all your jobs.'
      };
    }
    
    if (!!job.code && !job.initialized) {
      return {
        color: 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300',
        text: 'Waiting',
        title: 'Monitoring will begin after the next scheduled run.'
      };
    }
    
    if (!!job.code && !job.passing) {
      return {
        color: 'bg-red-100 text-red-600 dark:bg-red-900/30 dark:text-red-400',
        text: 'Failing',
        title: 'There is a problem with this job. Check Cronitor for more details.'
      };
    }
    
    if (!!job.code) {
      return {
        color: 'bg-green-100 text-green-600 dark:bg-green-900/30 dark:text-green-400',
        text: 'Healthy',
        title: 'This job is running as expected.'
      };
    }
    
    if (!job.key) {
      return {
        color: 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300',
        text: 'Syncing...'
      };
    }

    // Return a status for non-monitored jobs
    return {
      color: 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300',
      text: 'Not Monitored',
      title: 'This job is not being monitored. Enable monitoring to track its health.'
    };
  }, [job, monitorsLoading]);

  return {
    isLoading,
    handleToggle,
    getStatusInfo
  };
} 