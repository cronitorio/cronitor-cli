import React from 'react';
import { useJobMonitoring } from '../../hooks/useJobMonitoring';
import { BellSlashIcon } from '@heroicons/react/24/outline';

export function MonitoringSection({ job, onUpdate, onShowLearnMore, settings, monitorsLoading = false }) {
  const { isLoading, handleToggle, getStatusInfo } = useJobMonitoring(job, onUpdate, monitorsLoading);
  const statusInfo = getStatusInfo();

  if (!statusInfo) return null;

  // Determine orb color based on status
  const getOrbColor = () => {
    if (statusInfo.text === 'Healthy') {
      return 'bg-green-500';
    } else if (statusInfo.text === 'Failing') {
      return 'bg-red-500';
    } else {
      return 'bg-gray-400';
    }
  };

  // Determine toggle color - red if monitored and failing, otherwise default
  const getToggleColor = () => {
    if (job.monitored && statusInfo.text === 'Failing') {
      return 'bg-red-500';
    } else if (job.monitored) {
      return 'bg-green-500';
    } else {
      return 'bg-gray-200 dark:bg-gray-600';
    }
  };

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

  const handleLearnMoreClick = (e) => {
    e.preventDefault();
    e.stopPropagation();
    if (typeof onShowLearnMore === 'function') {
      onShowLearnMore();
    } else {
      console.error('onShowLearnMore is not a function:', onShowLearnMore);
    }
  };

  const handleToggleClick = () => {
    // Check if trying to enable monitoring without a valid API key
    const hasValidApiKey = settings?.CRONITOR_API_KEY && settings.CRONITOR_API_KEY.trim() !== '';
    
    if (!job.monitored && !hasValidApiKey) {
      // Show the learn more modal instead
      if (typeof onShowLearnMore === 'function') {
        onShowLearnMore();
      }
      return;
    }

    // For new jobs (no key), just update the form state
    if (!job.key) {
      onUpdate({
        ...job,
        monitored: !job.monitored
      });
      return;
    }
    // For existing jobs, use the handleToggle from useJobMonitoring
    handleToggle();
  };

  return (
    <div className="flex items-center space-x-2">
      <div className="flex items-center justify-between space-x-4">
        <div className="flex items-center">
          <button
            onClick={handleToggleClick}
            disabled={isLoading}
            className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 mr-4 ${getToggleColor()}`}
            title={job.monitored ? 'Monitoring enabled' : 'Monitoring disabled'}
          >
            <span
              className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                job.monitored ? 'translate-x-6' : 'translate-x-1'
              }`}
            />
          </button>
          {job.monitored ? (
            <>
              {!job.key ? null : (
                <>
                  {job.paused || job.disabled ? (
                    <StatusTag
                      className="inline-flex items-center pl-2 pr-1.5 py-0.5 text-sm font-medium bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-100 rounded-l-full border-r border-white dark:border-gray-800"
                      title="Alerts: Off"
                    >
                      <BellSlashIcon className="h-5 w-5 text-gray-600 dark:text-gray-300" />
                    </StatusTag>
                  ) : null}
                  <StatusTag
                    className={`inline-flex items-center px-1.5 py-0.5 text-sm font-medium bg-gray-100 text-gray-900 dark:bg-gray-700 dark:text-gray-100 ${(job.paused || job.disabled) ? 'rounded-l-none' : 'rounded-l-full'} rounded-r-full`}
                    title={statusInfo.title}
                  >
                    <div className={`w-2.5 h-2.5 rounded-full ${getOrbColor()} mr-2`}></div>
                    <span>{statusInfo.text}</span>
                  </StatusTag>
                </>
              )}
            </>
          ) : (
            <button
              onClick={handleLearnMoreClick}
              className="text-sm text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200"
              title="Learn more about monitoring"
            >
              Learn More
            </button>
          )}
        </div>
      </div>
    </div>
  );
} 