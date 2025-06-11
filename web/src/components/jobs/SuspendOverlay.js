import React, { useState } from 'react';
import { CloseButton } from '../CloseButton';

export function SuspendOverlay({
  job,
  allJobs,
  onClose,
  onToggleSuspension,
  onShowDeleteConfirmation,
  mutate
}) {
  // If job is currently not suspended, we want to suspend it (true)
  // If job is currently suspended, we want to unsuspend it (false)
  const desiredSuspendedState = !job.suspended;
  
  // Track pause_hours locally so we don't lose selection
  const [pauseHours, setPauseHours] = useState(job.pause_hours || '');
  
  // The button text depends on the current state
  const buttonText = job.suspended ? 'Activate Job' : 'Suspend Job';
  
  const handleButtonClick = () => {
    // Pass both the desired suspended state and the selected pause hours
    onToggleSuspension(desiredSuspendedState, pauseHours);
  };
  
  return (
    <div className="absolute -inset-2 bg-black bg-opacity-50 rounded-lg flex items-center justify-center z-10">
      <div className="bg-white dark:bg-gray-800 p-8 rounded-lg shadow-xl max-w-xl w-full mx-4 relative">
        <CloseButton onClick={onClose} />
        <h3 
          className="text-lg font-medium text-gray-900 dark:text-white mb-2 cursor-pointer hover:text-gray-600 dark:hover:text-gray-300"
          onClick={onClose}
          title="Click to close"
        >
          {job.suspended ? 'Suspended Job' : 'Scheduled Job'}
        </h3>
        <p className="text-gray-600 dark:text-gray-300 mb-4">
          {job.suspended 
            ? 'This job is suspended and will not be run at its scheduled time.'
            : 'This job will run at its scheduled time. If you suspend it, it\'ll be commented-out in the crontab.'}
        </p>
        {!job.suspended && job.monitored && (
          <div className="mb-4">
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Pause monitoring
            </label>
            <select
              className="mt-1 block w-full pl-3 pr-10 py-2 text-base border-gray-300 dark:border-gray-600 focus:outline-none focus:ring-blue-500 focus:border-blue-500 sm:text-sm rounded-md bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 appearance-none bg-[url('data:image/svg+xml;charset=utf-8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20fill%3D%22none%22%20viewBox%3D%220%200%2020%2020%22%3E%3Cpath%20stroke%3D%22%236B7280%22%20stroke-linecap%3D%22round%22%20stroke-linejoin%3D%22round%22%20stroke-width%3D%221.5%22%20d%3D%22M6%208l4%204%204-4%22%2F%3E%3C%2Fsvg%3E')] bg-[length:1.5em_1.5em] bg-[right_0.5rem_center] bg-no-repeat"
              value={pauseHours}
              onChange={(e) => setPauseHours(e.target.value)}
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
            onClick={handleButtonClick}
            className={`px-4 py-2 text-sm font-medium rounded-md ${
              job.suspended 
                ? 'bg-green-600 hover:bg-green-700 text-white' 
                : 'bg-red-100 text-red-600 hover:bg-red-200'
            }`}
          >
            {buttonText}
          </button>
          <button
            onClick={onShowDeleteConfirmation}
            className="px-4 py-2 text-sm font-medium text-white bg-red-600 hover:bg-red-700 rounded-md"
          >
            Delete Job
          </button>
        </div>
      </div>
    </div>
  );
} 