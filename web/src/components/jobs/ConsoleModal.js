import React from 'react';
import { CloseButton } from '../CloseButton';
import { csrfFetch } from '../../utils/api';
import { QuestionMarkCircleIcon } from '@heroicons/react/24/outline';

// Custom tooltip component
const Tooltip = ({ children, text }) => {
  const [show, setShow] = React.useState(false);

  return (
    <div className="relative inline-block">
      <div
        onMouseEnter={() => setShow(true)}
        onMouseLeave={() => setShow(false)}
      >
        {children}
      </div>
      {show && (
        <div className="absolute z-50 px-3 py-2 text-sm font-medium text-white bg-gray-900 rounded-lg shadow-lg dark:bg-gray-700 bottom-full left-1/2 transform -translate-x-1/2 mb-2 w-64">
          <div dangerouslySetInnerHTML={{ __html: text }} />
          <div className="absolute top-full left-1/2 transform -translate-x-1/2 w-0 h-0 border-l-4 border-r-4 border-t-4 border-transparent border-t-gray-900 dark:border-t-gray-700"></div>
        </div>
      )}
    </div>
  );
};

export function ConsoleModal({ job, onClose, isNew = false, onFormChange, onCommandUpdate }) {
  const [output, setOutput] = React.useState('');
  const [isRunning, setIsRunning] = React.useState(false);
  const [currentPid, setCurrentPid] = React.useState(null);
  const [command, setCommand] = React.useState(job.command);
  const [hasChanges, setHasChanges] = React.useState(false);
  const [isSaving, setIsSaving] = React.useState(false);
  const [withMonitoring, setWithMonitoring] = React.useState(true);
  const outputRef = React.useRef(null);
  const commandInputRef = React.useRef(null);
  const eventSourceRef = React.useRef(null);
  const outputLinesRef = React.useRef([]);

  // Update hasChanges when command changes
  React.useEffect(() => {
    setHasChanges(command !== job.command);
  }, [command, job.command]);

  React.useEffect(() => {
    if (commandInputRef.current) {
      commandInputRef.current.focus();
    }
  }, []);

  React.useEffect(() => {
    if (outputRef.current) {
      outputRef.current.scrollTop = outputRef.current.scrollHeight;
    }
  }, [output]);

  // Cleanup event source on unmount
  React.useEffect(() => {
    const eventSource = eventSourceRef.current;
    return () => {
      if (eventSource) {
        eventSource.close();
      }
    };
  }, []);

  const addOutput = (newOutput) => {
    if (!newOutput) return;

    // Split the new output into lines
    const newLines = newOutput.split('\n');
    
    // Add new lines to our array
    outputLinesRef.current = [...outputLinesRef.current, ...newLines];
    
    // Keep only the last 1000 lines
    if (outputLinesRef.current.length > 1000) {
      outputLinesRef.current = outputLinesRef.current.slice(-1000);
    }
    
    // Update the output state
    setOutput(outputLinesRef.current.join('\n'));
  };

  const runCommand = async () => {
    setIsRunning(true);
    setOutput('');
    outputLinesRef.current = [];
    setCurrentPid(null);

    // Close any existing event source
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
    }

    try {
      const response = await csrfFetch('/api/jobs/run', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ 
          command,
          crontab_filename: job.crontab_filename,
          key: job.key,
          with_monitoring: job.monitored ? withMonitoring : false
        })
      });

      if (!response.ok) {
        throw new Error('Failed to start job');
      }

      // Set up the reader for the response stream
      const reader = response.body.getReader();
      const decoder = new TextDecoder();

      while (true) {
        const { value, done } = await reader.read();
        if (done) break;

        const chunk = decoder.decode(value);
        const lines = chunk.split('\n');
        
        for (const line of lines) {
          if (line.startsWith('data: ')) {
            try {
              const data = JSON.parse(line.slice(6));
              if (data.pid) {
                setCurrentPid(data.pid);
              } else if (data.output) {
                addOutput(data.output);
              } else if (data.error) {
                addOutput(data.error);
              } else if (data.completion) {
                addOutput(data.completion);
                setIsRunning(false);
              }
            } catch (e) {
              console.error('Failed to parse SSE data:', e);
            }
          }
        }
      }
    } catch (error) {
      addOutput(`Error: ${error.message}\n`);
      setIsRunning(false);
    }
  };

  const handleKill = async () => {
    if (!currentPid) return;

    try {
      await csrfFetch('/api/jobs/kill', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ pids: [currentPid] }),
      });
      
      // Don't close the event source immediately - let the backend send the completion message
      // The connection will close naturally when the backend detects the process was killed
      addOutput('\n[Process kill signal sent...]\n');
    } catch (error) {
      console.error('Failed to kill process:', error);
      addOutput(`\nError killing process: ${error.message}\n`);
      setIsRunning(false);
    }
  };

  const handleKeyDown = (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      runCommand();
    }
  };

  const handleCommandBlur = () => {
    if (command === job.command) {
      setHasChanges(false);
    }
  };

  const handleSave = async () => {
    setIsSaving(true);
    try {
      if (isNew) {
        // For new jobs, update the form state and the command in parent
        onFormChange({
          ...job,
          command: command,
        });
        // Also update the editedCommand state if callback is provided
        if (onCommandUpdate) {
          onCommandUpdate(command);
        }
        setHasChanges(false);
        // Close the modal after saving in Add Mode to provide feedback
        setTimeout(() => {
          onClose();
        }, 500);
      } else {
        const response = await csrfFetch('/api/jobs', {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            ...job,
            command: command,
          }),
        });

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(`Failed to update job command: ${response.status} ${response.statusText}${errorText ? ` - ${errorText}` : ''}`);
        }

        // Update the job in the parent component
        job.command = command;
        setHasChanges(false);
      }
    } catch (error) {
      console.error('Error saving command:', error);
      addOutput(`Error saving command: ${error.message}\n`);
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50" style={{ margin: '0px' }}>
      <div className="bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 rounded-lg shadow-xl w-[calc(100%-2rem)] max-w-6xl mx-4 relative">
        <CloseButton onClick={onClose} />
        <div className="p-4 px-4">
          <div className="mb-4">
            <div className="flex items-center space-x-2">
              <span className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Command</span>
            </div>
            <div className="flex items-center space-x-2 mt-1 bg-black p-4 rounded">
              <span className="text-green-500">$</span>
              <input
                ref={commandInputRef}
                type="text"
                value={command}
                onChange={(e) => setCommand(e.target.value)}
                onKeyDown={handleKeyDown}
                onBlur={handleCommandBlur}
                className="w-full text-sm text-white font-mono bg-transparent focus:outline-none"
                disabled={isRunning}
              />
            </div>
          </div>
          
          <div className="mb-2">
            <span className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Output</span>
          </div>
          <div
            ref={outputRef}
            className="bg-black text-white p-4 rounded font-mono text-sm h-96 overflow-y-auto whitespace-pre-wrap"
          >
            {output}
          </div>
          
          <div className="mt-4 flex justify-between items-center">
            <div className="flex items-center">
              {job.monitored && (
                <>
                  <input
                    type="checkbox"
                    id="with-monitoring"
                    checked={withMonitoring}
                    onChange={(e) => setWithMonitoring(e.target.checked)}
                    className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
                    disabled={isRunning}
                  />
                  <label htmlFor="with-monitoring" className="ml-2 text-base text-gray-700 dark:text-gray-300 flex items-center">
                    Run with monitoring enabled
                    <Tooltip text="Capture job output and send monitoring telemetry events for this job.">
                      <QuestionMarkCircleIcon 
                        className="ml-2 h-4 w-4 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300" 
                      />
                    </Tooltip>
                  </label>
                </>
              )}
            </div>
            <div className="flex space-x-2">
              {hasChanges && !isRunning && (
                <button
                  onClick={handleSave}
                  disabled={isSaving}
                  className={`px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 font-medium ${
                    isSaving ? 'opacity-50 cursor-not-allowed' : ''
                  }`}
                >
                  {isSaving ? 'Saving...' : 'Save Changes'}
                </button>
              )}
              {isRunning ? (
                <button
                  onClick={handleKill}
                  className="px-4 py-2 bg-red-600 text-white rounded hover:bg-red-700 font-medium"
                >
                  Kill Now
                </button>
              ) : (
                <button
                  onClick={runCommand}
                  className="px-4 py-2 bg-green-600 text-white rounded hover:bg-green-700 font-medium"
                >
                  Run Command
                </button>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
} 