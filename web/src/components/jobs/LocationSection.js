import React from 'react';

export function LocationSection({
  job,
  isNew,
  cronFiles,
  users,
  selectedLocation,
  selectedUser,
  isUserCrontab,
  onLocationChange,
  onUserChange,
  isMacOS
}) {
  return (
    <div className="">
      {isNew ? (
        <select
          value={selectedLocation}
          onChange={onLocationChange}
          className="mt-1 block w-full pl-3 pr-10 py-2 text-base border-gray-400 dark:border-gray-600 focus:outline-none focus:ring-blue-500 focus:border-blue-500 sm:text-sm rounded-md bg-gray-200 dark:bg-gray-700 text-gray-800 dark:text-gray-100 appearance-none bg-[url('data:image/svg+xml;charset=utf-8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20fill%3D%22none%22%20viewBox%3D%220%200%2020%2020%22%3E%3Cpath%20stroke%3D%22%236B7280%22%20stroke-linecap%3D%22round%22%20stroke-linejoin%3D%22round%22%20stroke-width%3D%221.5%22%20d%3D%22M6%208l4%204%204-4%22%2F%3E%3C%2Fsvg%3E')] bg-[length:1.5em_1.5em] bg-[right_0.5rem_center] bg-no-repeat"
        >
          <option value="">Select a location</option>
          {cronFiles.map((file) => (
            <option key={file.filename} value={file.filename}>
              {file.display_name}
            </option>
          ))}
          {!isMacOS && (
            <option value="/etc/cron.d (New Crontab)">/etc/cron.d (New Crontab)</option>
          )}
        </select>
      ) : (
        <div>
          {job.crontab_display_name} L{job.line_number}
        </div>
      )}
    </div>
  );
} 