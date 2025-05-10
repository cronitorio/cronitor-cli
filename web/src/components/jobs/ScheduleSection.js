import React from 'react';
import { PencilIcon } from '@heroicons/react/24/outline';
import { useJobSchedule } from '../../hooks/useJobSchedule';
import { getBrowserTimezone } from '../../utils/timezone';

export function ScheduleSection({
  job,
  isEditing,
  editedSchedule,
  onScheduleChange,
  onEditStart,
  onEditEnd,
  onKeyDown,
  showDescription = true
}) {
  const [showNextTimes, setShowNextTimes] = React.useState(false);
  const { scheduleDescription, isValid, nextExecutionTimes } = useJobSchedule(
    isEditing ? editedSchedule : job.expression,
    job.timezone
  );

  const browserTimezone = getBrowserTimezone();
  const showTimezoneTooltip = job.timezone !== browserTimezone;

  if (!showDescription) {
    return (
      <div className="flex items-center">
        {isEditing ? (
          <input
            type="text"
            value={editedSchedule}
            onChange={(e) => onScheduleChange(e.target.value)}
            onKeyDown={onKeyDown}
            onBlur={onEditEnd}
            className="w-full text-sm font-mono bg-transparent border-b border-gray-300 dark:border-gray-600 focus:outline-none"
          />
        ) : (
          <span className="text-sm font-mono text-gray-500 dark:text-gray-400">
            {editedSchedule || job.expression}
          </span>
        )}
        <button
          onClick={onEditStart}
          className="ml-2 opacity-0 group-hover:opacity-100 transition-opacity"
        >
          <PencilIcon className="h-4 w-4 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200" />
        </button>
      </div>
    );
  }

  return (
    <div className={`text-sm ${
      isValid 
        ? 'text-gray-600 dark:text-gray-300' 
        : 'text-pink-500 dark:text-pink-400'
    }`}>
      {scheduleDescription}
      {job.run_as_user && <span className="text-gray-400 dark:text-gray-500"> as "{job.run_as_user}"</span>}
      {nextExecutionTimes.length > 0 ? (
        <span className="text-gray-400 dark:text-gray-500">
          {' '}<span className="text-gray-700 dark:text-gray-200 ml-4">Next at</span>{' '}
          <span className="relative">
            <span 
              className="text-gray-700 dark:text-gray-200 cursor-pointer"
              onMouseEnter={(e) => {
                const tooltip = e.currentTarget.nextElementSibling;
                if (showTimezoneTooltip) {
                  tooltip.classList.remove('hidden');
                }
              }}
              onMouseLeave={(e) => {
                const tooltip = e.currentTarget.nextElementSibling;
                if (showTimezoneTooltip) {
                  tooltip.classList.add('hidden');
                }
              }}
            >
              {nextExecutionTimes[0].job} {nextExecutionTimes[0].jobTimezone}
            </span>
            {showTimezoneTooltip && (
              <div className="absolute left-0 bottom-full mb-2 hidden bg-gray-900 text-white text-xs rounded py-1 px-2 whitespace-nowrap z-10">
                {nextExecutionTimes[0].local} {nextExecutionTimes[0].localTimezone} Browser Time
              </div>
            )}
          </span>
          {nextExecutionTimes.length > 1 && (
            <span className="text-gray-400 dark:text-gray-500">
              {' '}<button
                onClick={() => setShowNextTimes(!showNextTimes)}
                className="text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300"
              >
                {showNextTimes ? 'Show Less' : 'Show More'}
              </button>
            </span>
          )}
        </span>
      ) : (
        <span className="text-gray-400 dark:text-gray-500">
          {' '}No upcoming executions
        </span>
      )}
      {showNextTimes && nextExecutionTimes.length > 0 && (
        <div className="mt-2">
          <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
            <thead>
              <tr>
                <th className="py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                  Job Time ({job.timezone})
                </th>
                <th className="py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                  Browser Time ({browserTimezone})
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
              {nextExecutionTimes.map((time, index) => (
                <tr key={index}>
                  <td className="py-2 text-sm text-gray-900 dark:text-gray-100">
                    {time.job}
                  </td>
                  <td className="py-2 text-sm text-gray-900 dark:text-gray-100">
                    {time.local}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
} 