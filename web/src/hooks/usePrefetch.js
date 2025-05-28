import { useCallback } from 'react';
import { mutate } from 'swr';

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
      throw new Error(`Unable to connect to the dash server. Check that it's running and try again.`);
    }
    throw error;
  }
};

export function usePrefetch() {
  const prefetchCrontabs = useCallback(async () => {
    // Pre-populate the SWR cache
    const data = await fetcher('/api/crontabs');
    mutate('/api/crontabs', data, false);
  }, []);

  const prefetchJobs = useCallback(async () => {
    const data = await fetcher('/api/jobs');
    mutate('/api/jobs', data, false);
  }, []);

  const prefetchSettings = useCallback(async () => {
    const data = await fetcher('/api/settings');
    mutate('/api/settings', data, false);
  }, []);

  const prefetchAll = useCallback(async () => {
    // Stagger the requests to avoid congestion
    await prefetchSettings();
    await new Promise(resolve => setTimeout(resolve, 100));
    await prefetchCrontabs();
    await new Promise(resolve => setTimeout(resolve, 100));
    await prefetchJobs();
  }, [prefetchSettings, prefetchCrontabs, prefetchJobs]);

  return {
    prefetchCrontabs,
    prefetchJobs,
    prefetchSettings,
    prefetchAll
  };
} 