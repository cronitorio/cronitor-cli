import { useCallback } from 'react';
import useSWR from 'swr';

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
      throw new Error('Unable to connect to server. Please check if the server is running and try again.');
    }
    throw error;
  }
};

export function useJobOperations() {
  const { data: jobs, mutate } = useSWR('/api/jobs', fetcher, {
    refreshInterval: 5000,
    revalidateOnFocus: true
  });

  const handleNetworkError = (error, operation) => {
    if (error.name === 'TypeError' && error.message === 'Failed to fetch') {
      throw new Error(`Unable to ${operation}. Server is not responding. Please check if the server is running and try again.`);
    }
    throw error;
  };

  const createJob = useCallback(async (jobData) => {
    // Optimistic update
    const optimisticData = [...(jobs || []), { ...jobData, key: Date.now().toString() }];
    mutate(optimisticData, false);

    try {
      const response = await fetch('/api/jobs', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(jobData),
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`Failed to create job: ${response.status} ${response.statusText}${errorText ? ` - ${errorText}` : ''}`);
      }

      // Revalidate to get the actual data
      mutate();
      return true;
    } catch (error) {
      // Revert optimistic update
      mutate(jobs, false);
      handleNetworkError(error, 'create job');
    }
  }, [jobs, mutate]);

  const updateJob = useCallback(async (jobData) => {
    // Optimistic update
    const optimisticData = jobs.map(j => 
      j.key === jobData.key ? { ...j, ...jobData } : j
    );
    mutate(optimisticData, false);

    try {
      const response = await fetch('/api/jobs', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(jobData),
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`Failed to update job: ${response.status} ${response.statusText}${errorText ? ` - ${errorText}` : ''}`);
      }

      // Revalidate to get the actual data
      mutate();
      return true;
    } catch (error) {
      // Revert optimistic update
      mutate(jobs, false);
      handleNetworkError(error, 'update job');
    }
  }, [jobs, mutate]);

  const deleteJob = useCallback(async (jobKey) => {
    // Optimistic update
    const optimisticData = jobs.filter(j => j.key !== jobKey);
    mutate(optimisticData, false);

    try {
      const response = await fetch('/api/jobs', {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ key: jobKey }),
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`Failed to delete job: ${response.status} ${response.statusText}${errorText ? ` - ${errorText}` : ''}`);
      }

      // Revalidate to get the actual data
      mutate();
      return true;
    } catch (error) {
      // Revert optimistic update
      mutate(jobs, false);
      handleNetworkError(error, 'delete job');
    }
  }, [jobs, mutate]);

  const toggleJobMonitoring = useCallback(async (jobKey, isMonitored) => {
    const job = jobs.find(j => j.key === jobKey);
    if (!job) return;

    // Optimistic update
    const optimisticData = jobs.map(j => 
      j.key === jobKey ? { ...j, is_monitored: isMonitored } : j
    );
    mutate(optimisticData, false);

    try {
      const response = await fetch('/api/jobs', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          ...job,
          is_monitored: isMonitored,
        }),
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`Failed to update job monitoring status: ${response.status} ${response.statusText}${errorText ? ` - ${errorText}` : ''}`);
      }

      // Revalidate to get the actual data
      mutate();
      return true;
    } catch (error) {
      // Revert optimistic update
      mutate(jobs, false);
      handleNetworkError(error, 'update monitoring status');
    }
  }, [jobs, mutate]);

  const toggleJobSuspension = useCallback(async (jobKey, suspended) => {
    const job = jobs.find(j => j.key === jobKey);
    if (!job) return;

    // Optimistic update
    const optimisticData = jobs.map(j => 
      j.key === jobKey ? { ...j, suspended } : j
    );
    mutate(optimisticData, false);

    try {
      const response = await fetch('/api/jobs', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          ...job,
          suspended,
          pause_hours: suspended && job.is_monitored ? job.pause_hours : !suspended && job.is_monitored ? "0" : null,
        }),
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`Failed to update job suspension status: ${response.status} ${response.statusText}${errorText ? ` - ${errorText}` : ''}`);
      }

      // Revalidate to get the actual data
      mutate();
      return true;
    } catch (error) {
      // Revert optimistic update
      mutate(jobs, false);
      handleNetworkError(error, 'update suspension status');
    }
  }, [jobs, mutate]);

  const killJobProcess = useCallback(async (pids) => {
    try {
      // Convert string PIDs to integers
      const numericPids = pids.map(pid => parseInt(pid, 10));
      
      const response = await fetch('/api/jobs/kill', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ pids: numericPids }),
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`Failed to kill job process: ${response.status} ${response.statusText}${errorText ? ` - ${errorText}` : ''}`);
      }

      // Revalidate to get the actual data
      mutate();
      return true;
    } catch (error) {
      handleNetworkError(error, 'kill job process');
    }
  }, [mutate]);

  return {
    jobs,
    createJob,
    updateJob,
    deleteJob,
    toggleJobMonitoring,
    toggleJobSuspension,
    killJobProcess,
    mutate
  };
} 