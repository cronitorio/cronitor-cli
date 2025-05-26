import React, { useState, useEffect } from 'react';
import useSWR from 'swr';
import { NewCrontabOverlay } from './jobs/NewCrontabOverlay';
import { Toast } from './Toast';

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

export default function Crontabs() {
  const { data: crontabs, error, mutate } = useSWR('/api/crontabs', fetcher, {
    refreshInterval: 5000,
    revalidateOnFocus: true
  });
  const { data: settings } = useSWR('/api/settings', fetcher);
  
  const [selectedCrontab, setSelectedCrontab] = useState(null);
  const [selectedLine, setSelectedLine] = useState(null);
  const [showNewCrontab, setShowNewCrontab] = useState(false);
  const [isToastVisible, setIsToastVisible] = useState(false);
  const [toastMessage, setToastMessage] = useState('');
  const [toastType, setToastType] = useState('error');
  
  const [newCrontabForm, setNewCrontabForm] = useState({
    filename: '',
    timezone: '',
    comments: ''
  });

  // Update timezone when settings are loaded
  useEffect(() => {
    if (settings?.timezone) {
      setNewCrontabForm(prev => ({
        ...prev,
        timezone: settings.timezone
      }));
    }
  }, [settings]);

  // Select first crontab when data loads
  useEffect(() => {
    if (crontabs && crontabs.length > 0 && !selectedCrontab) {
      setSelectedCrontab(crontabs[0]);
    }
  }, [crontabs, selectedCrontab]);

  const showToast = (message, type = 'error') => {
    setToastMessage(message);
    setToastType(type);
    setIsToastVisible(true);
    setTimeout(() => setIsToastVisible(false), type === 'error' ? 6000 : 3000);
  };

  const handleCreateCrontab = async () => {
    try {
      if (!newCrontabForm.timezone && settings?.timezone) {
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
      
      setShowNewCrontab(false);
      showToast(
        response.status === 201 ? 'Crontab Created' : 'Crontab Already Exists',
        'success'
      );
      
      // Refresh crontabs list
      mutate();
      
      // Select the new crontab
      setSelectedCrontab(crontab);
      
      // Reset form
      setNewCrontabForm({
        filename: '',
        timezone: settings?.timezone || '',
        comments: ''
      });
    } catch (error) {
      console.error('Error creating crontab:', error);
      showToast('Failed to create crontab: ' + error.message);
    }
  };

  const handleLineClick = (lineIndex) => {
    if (selectedCrontab && selectedCrontab.lines) {
      setSelectedLine(selectedCrontab.lines[lineIndex]);
    }
  };

  const formatCrontabContent = () => {
    if (!selectedCrontab || !selectedCrontab.lines) return [];
    
    const formattedLines = [];
    let lineNumber = 1;
    
    selectedCrontab.lines.forEach((line) => {
      // If this is a job line with a name, add the Name comment first
      if (line.is_job && line.name) {
        formattedLines.push({
          lineNumber: lineNumber++,
          text: `# Name: ${line.name}`,
          originalLine: null,
          isNameComment: true
        });
      }
      
      // Add the actual line
      formattedLines.push({
        lineNumber: lineNumber++,
        text: line.line_text || '',
        originalLine: line,
        isNameComment: false
      });
    });
    
    return formattedLines;
  };

  if (error) {
    return (
      <div className="p-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg">
        <h3 className="text-sm font-medium text-red-800 dark:text-red-200">Failed to load crontabs</h3>
        <div className="mt-2 text-sm text-red-700 dark:text-red-300">
          <pre className="whitespace-pre-wrap break-words">{error.message}</pre>
        </div>
      </div>
    );
  }

  if (!crontabs) {
    return <div className="text-gray-600 dark:text-gray-300">Loading...</div>;
  }

  return (
    <div className="h-full flex flex-col">
      <div className="flex justify-between items-center mb-6">
        <h1 className="text-2xl font-semibold text-gray-900 dark:text-white">Crontabs</h1>
        <button
          onClick={() => setShowNewCrontab(true)}
          className="px-4 py-2.5 bg-blue-600 text-white rounded hover:bg-blue-700 font-medium"
        >
          Add Crontab
        </button>
      </div>

      <div className="mb-4">
        <select
          value={selectedCrontab?.filename || ''}
          onChange={(e) => {
            const crontab = crontabs.find(c => c.filename === e.target.value);
            setSelectedCrontab(crontab);
            setSelectedLine(null);
          }}
          className="block w-full pl-3 pr-10 py-2 text-base border-gray-300 dark:border-gray-600 focus:outline-none focus:ring-blue-500 focus:border-blue-500 rounded-md bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 appearance-none bg-[url('data:image/svg+xml;charset=utf-8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20fill%3D%22none%22%20viewBox%3D%220%200%2020%2020%22%3E%3Cpath%20stroke%3D%22%236B7280%22%20stroke-linecap%3D%22round%22%20stroke-linejoin%3D%22round%22%20stroke-width%3D%221.5%22%20d%3D%22M6%208l4%204%204-4%22%2F%3E%3C%2Fsvg%3E')] bg-[length:1.5em_1.5em] bg-[right_0.5rem_center] bg-no-repeat"
        >
          {crontabs.map((crontab) => (
            <option key={crontab.filename} value={crontab.filename}>
              {(crontab.display_name || crontab.filename).replace(/^user /, 'User ')}
            </option>
          ))}
        </select>
      </div>

      <div className="flex-1 flex gap-4 min-h-0">
        {/* Main content area - 3/4 width */}
        <div className="flex-[3] bg-white dark:bg-gray-800 rounded-lg shadow overflow-hidden">
          <div className="h-full p-4">
            <div className="h-full bg-gray-50 dark:bg-gray-900 rounded-lg overflow-hidden">
              <div className="h-full flex">
                {/* Line numbers column */}
                <div className="flex-shrink-0 py-4 pl-4 pr-2 text-gray-400 dark:text-gray-500 font-mono text-sm select-none" style={{ lineHeight: '1.5' }}>
                  {formatCrontabContent().map((line, index) => (
                    <div key={index}>{String(line.lineNumber).padStart(3, ' ')}</div>
                  ))}
                </div>
                {/* Separator */}
                <div className="flex-shrink-0 py-4 text-gray-300 dark:text-gray-600 font-mono text-sm select-none" style={{ lineHeight: '1.5' }}>
                  {formatCrontabContent().map((_, index) => (
                    <div key={index}>│</div>
                  ))}
                </div>
                {/* Content area */}
                <div className="flex-1 overflow-auto">
                  <div 
                    className="py-4 px-3 font-mono text-sm text-gray-900 dark:text-gray-100 cursor-text min-h-full"
                    style={{ lineHeight: '1.5' }}
                    tabIndex={0}
                    onKeyDown={(e) => {
                      // Handle keyboard navigation
                      const formattedLines = formatCrontabContent();
                      const currentIndex = selectedLine ? formattedLines.findIndex(fl => fl.originalLine === selectedLine) : -1;
                      let newIndex = currentIndex;
                      
                      if (e.key === 'ArrowUp' && currentIndex > 0) {
                        newIndex = currentIndex - 1;
                        // Skip name comment lines
                        while (newIndex >= 0 && formattedLines[newIndex].isNameComment) {
                          newIndex--;
                        }
                        e.preventDefault();
                      } else if (e.key === 'ArrowDown' && currentIndex < formattedLines.length - 1) {
                        newIndex = currentIndex + 1;
                        // Skip name comment lines
                        while (newIndex < formattedLines.length && formattedLines[newIndex].isNameComment) {
                          newIndex++;
                        }
                        e.preventDefault();
                      }
                      
                      if (newIndex !== currentIndex && newIndex >= 0 && newIndex < formattedLines.length && !formattedLines[newIndex].isNameComment) {
                        setSelectedLine(formattedLines[newIndex].originalLine);
                      }
                    }}
                  >
                    {formatCrontabContent().map((line, index) => (
                      <div
                        key={index}
                        onClick={() => !line.isNameComment && line.originalLine && handleLineClick(selectedCrontab.lines.indexOf(line.originalLine))}
                        className={`pr-4 ${
                          !line.isNameComment ? 'hover:bg-gray-100 dark:hover:bg-gray-800 cursor-pointer' : ''
                        } ${
                          line.originalLine === selectedLine ? 'bg-blue-50 dark:bg-blue-900/20' : ''
                        } ${
                          line.isNameComment ? 'text-gray-500 dark:text-gray-400' : ''
                        }`}
                      >
                        {line.text || '\u00A0'}
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* Sidebar - 1/4 width with fixed sizing */}
        <div className="w-80 flex-shrink-0 bg-white dark:bg-gray-800 rounded-lg shadow">
          <div className="p-4 h-full overflow-y-auto">
            <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-4">Line Details</h3>
            
            {selectedLine ? (
              <div className="space-y-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">Type</label>
                  <p className="mt-1 text-sm text-gray-900 dark:text-gray-100">
                    {selectedLine.is_job ? 'Job' : 
                     selectedLine.is_comment ? 'Comment' : 
                     selectedLine.is_env_var ? 'Environment Variable' : 
                     'Other'}
                  </p>
                </div>

                {selectedLine.is_job && (
                  <>
                    <div>
                      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">Expression</label>
                      <p className="mt-1 text-sm text-gray-900 dark:text-gray-100 font-mono">
                        {selectedLine.cron_expression || 'N/A'}
                      </p>
                    </div>
                    
                    <div>
                      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">Command</label>
                      <p className="mt-1 text-sm text-gray-900 dark:text-gray-100 font-mono break-all">
                        {selectedLine.command_to_run || 'N/A'}
                      </p>
                    </div>

                    {selectedLine.name && (
                      <div>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">Name</label>
                        <p className="mt-1 text-sm text-gray-900 dark:text-gray-100">
                          {selectedLine.name}
                        </p>
                      </div>
                    )}

                    {selectedLine.run_as && (
                      <div>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">Run As</label>
                        <p className="mt-1 text-sm text-gray-900 dark:text-gray-100">
                          {selectedLine.run_as}
                        </p>
                      </div>
                    )}

                    <div>
                      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">Monitored</label>
                      <p className="mt-1 text-sm text-gray-900 dark:text-gray-100">
                        {selectedLine.code ? 'Yes' : 'No'}
                      </p>
                    </div>
                  </>
                )}

                {selectedLine.is_env_var && (
                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">Variable</label>
                    <p className="mt-1 text-sm text-gray-900 dark:text-gray-100 font-mono">
                      {selectedLine.env_var_key} = {selectedLine.env_var_value}
                    </p>
                  </div>
                )}
              </div>
            ) : (
              <p className="text-sm text-gray-500 dark:text-gray-400">
                Click on a line in the crontab to view its details
              </p>
            )}
          </div>
        </div>
      </div>

      {showNewCrontab && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black bg-opacity-50">
          <NewCrontabOverlay
            formData={newCrontabForm}
            onFormChange={setNewCrontabForm}
            onClose={() => {
              setShowNewCrontab(false);
              setNewCrontabForm({
                filename: '',
                timezone: settings?.timezone || '',
                comments: ''
              });
            }}
            onCreateCrontab={handleCreateCrontab}
            timezones={settings?.timezones}
          />
        </div>
      )}

      {isToastVisible && (
        <Toast message={toastMessage} onClose={() => setIsToastVisible(false)} type={toastType} />
      )}
    </div>
  );
} 