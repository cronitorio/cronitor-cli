import { useState, useEffect } from 'react';
import useSWR from 'swr';
import { ExclamationTriangleIcon, EyeIcon, EyeSlashIcon } from '@heroicons/react/24/outline';

const fetcher = url => fetch(url).then(res => res.json());

export default function Settings() {
  const { data, error, mutate } = useSWR('/api/settings', fetcher, {
    refreshInterval: 5000, // Refresh every 5 seconds
  });

  const [formData, setFormData] = useState({
    CRONITOR_API_KEY: '',
    CRONITOR_PING_API_KEY: '',
    CRONITOR_EXCLUDE_TEXT: '',
    CRONITOR_HOSTNAME: '',
    CRONITOR_LOG: '',
    CRONITOR_ENV: '',
    CRONITOR_DASH_USER: '',
    CRONITOR_DASH_PASS: '',
  });

  const [envVars, setEnvVars] = useState({
    CRONITOR_API_KEY: false,
    CRONITOR_PING_API_KEY: false,
    CRONITOR_EXCLUDE_TEXT: false,
    CRONITOR_HOSTNAME: false,
    CRONITOR_LOG: false,
    CRONITOR_ENV: false,
    CRONITOR_DASH_USER: false,
    CRONITOR_DASH_PASS: false,
  });

  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState(null);
  const [saveSuccess, setSaveSuccess] = useState(false);
  const [showPassword, setShowPassword] = useState(false);

  // Update form data when API data is loaded
  useEffect(() => {
    if (data) {
      setFormData({
        ...data,
        CRONITOR_EXCLUDE_TEXT: data.CRONITOR_EXCLUDE_TEXT || []
      });
      setEnvVars(data.env_vars || {});
    }
  }, [data]);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setIsSaving(true);
    setSaveError(null);
    setSaveSuccess(false);

    try {
      const response = await fetch('/api/settings', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(formData),
      });

      if (!response.ok) {
        throw new Error('Failed to save settings');
      }

      setSaveSuccess(true);
      mutate(); // Refresh the data
    } catch (err) {
      setSaveError(err.message);
    } finally {
      setIsSaving(false);
    }
  };

  const handleChange = (e) => {
    const { name, value } = e.target;
    setFormData(prev => ({
      ...prev,
      [name]: value
    }));
  };

  const handleExcludeTextChange = (e) => {
    const value = e.target.value;
    setFormData(prev => ({
      ...prev,
      CRONITOR_EXCLUDE_TEXT: value.split(',').map(item => item.trim())
    }));
  };

  if (error) return <div className="text-red-600 dark:text-red-400">Failed to load settings</div>;
  if (!data) return <div className="text-gray-900 dark:text-gray-100">Loading...</div>;

  const renderInput = (name, label, type = "text", value, onChange, disabled = false) => (
    <div>
      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">{label}</label>
      <div className="relative mt-1">
        <input
          type={type}
          name={name}
          value={value}
          onChange={onChange}
          disabled={disabled}
          className={`block w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 dark:bg-gray-700 dark:border-gray-600 dark:text-white px-4 py-2 ${
            disabled ? 'bg-gray-100 dark:bg-gray-800 cursor-not-allowed' : ''
          }`}
        />
        {name === "CRONITOR_DASH_PASS" && (
          <button
            type="button"
            onClick={() => setShowPassword(!showPassword)}
            className="absolute inset-y-0 right-0 pr-3 flex items-center"
          >
            {showPassword ? (
              <EyeSlashIcon className="h-5 w-5 text-gray-400" />
            ) : (
              <EyeIcon className="h-5 w-5 text-gray-400" />
            )}
          </button>
        )}
      </div>
      {disabled && (
        <div className="mt-1 flex items-center text-sm text-yellow-600 dark:text-yellow-400">
          <ExclamationTriangleIcon className="h-4 w-4 mr-1" />
          Currently set as an environment variable
        </div>
      )}
    </div>
  );

  return (
    <div className="max-w-2xl mx-auto p-4">
      <h1 className="text-2xl font-bold mb-6 text-gray-900 dark:text-white">Settings</h1>
      
      <div className="mb-6 p-4 bg-gray-50 dark:bg-gray-800 rounded-md">
        <div className="text-sm text-gray-600 dark:text-gray-300">
          <span className="font-medium">Config File:</span> {data.config_file_path}
        </div>
      </div>

      <form onSubmit={handleSubmit} className="space-y-4">
        {renderInput(
          "CRONITOR_API_KEY",
          "API Key",
          "text",
          formData.CRONITOR_API_KEY,
          handleChange,
          envVars["CRONITOR_API_KEY"]
        )}

        {renderInput(
          "CRONITOR_PING_API_KEY",
          "Ping API Key",
          "text",
          formData.CRONITOR_PING_API_KEY,
          handleChange,
          envVars["CRONITOR_PING_API_KEY"]
        )}

        <div>
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
            Exclude Text (comma-separated)
          </label>
          <input
            type="text"
            value={(formData.CRONITOR_EXCLUDE_TEXT || []).join(', ')}
            onChange={handleExcludeTextChange}
            disabled={envVars["CRONITOR_EXCLUDE_TEXT"]}
            className={`mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 dark:bg-gray-700 dark:border-gray-600 dark:text-white px-4 py-2 ${
              envVars["CRONITOR_EXCLUDE_TEXT"] ? 'bg-gray-100 dark:bg-gray-800 cursor-not-allowed' : ''
            }`}
          />
          {envVars["CRONITOR_EXCLUDE_TEXT"] && (
            <div className="mt-1 flex items-center text-sm text-yellow-600 dark:text-yellow-400">
              <ExclamationTriangleIcon className="h-4 w-4 mr-1" />
              Currently set as an environment variable
            </div>
          )}
        </div>

        {renderInput(
          "CRONITOR_HOSTNAME",
          "Hostname",
          "text",
          formData.CRONITOR_HOSTNAME,
          handleChange,
          envVars["CRONITOR_HOSTNAME"]
        )}

        {renderInput(
          "CRONITOR_LOG",
          "Log File",
          "text",
          formData.CRONITOR_LOG,
          handleChange,
          envVars["CRONITOR_LOG"]
        )}

        {renderInput(
          "CRONITOR_ENV",
          "Environment",
          "text",
          formData.CRONITOR_ENV,
          handleChange,
          envVars["CRONITOR_ENV"]
        )}

        {renderInput(
          "CRONITOR_DASH_USER",
          "LocalDash Username",
          "text",
          formData.CRONITOR_DASH_USER,
          handleChange,
          envVars["CRONITOR_DASH_USER"]
        )}

        {renderInput(
          "CRONITOR_DASH_PASS",
          "LocalDash Password",
          showPassword ? "text" : "password",
          formData.CRONITOR_DASH_PASS,
          handleChange,
          envVars["CRONITOR_DASH_PASS"]
        )}

        {saveError && (
          <div className="text-red-600 dark:text-red-400 text-sm">{saveError}</div>
        )}

        {saveSuccess && (
          <div className="text-green-600 dark:text-green-400 text-sm">Settings saved successfully!</div>
        )}

        <button
          type="submit"
          disabled={isSaving}
          className="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50"
        >
          {isSaving ? 'Saving...' : 'Save Settings'}
        </button>
      </form>
    </div>
  );
} 