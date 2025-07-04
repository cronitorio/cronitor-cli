import { useState, useEffect } from 'react';
import useSWR from 'swr';
import { ExclamationTriangleIcon, EyeIcon, EyeSlashIcon, QuestionMarkCircleIcon } from '@heroicons/react/24/outline';
import { useLocation } from 'react-router-dom';
import { csrfFetcher, csrfFetch } from '../utils/api';

// Custom tooltip component
const Tooltip = ({ children, text }) => {
  const [show, setShow] = useState(false);

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

export default function Settings() {
  const location = useLocation();
  const isSettingsView = location.pathname === '/settings';
  
  const { data, error, mutate } = useSWR('/api/settings', csrfFetcher, {
    refreshInterval: isSettingsView ? 30000 : 0, // 30s on Settings view, no refresh elsewhere
    revalidateOnFocus: true, // Revalidate when user comes back to the tab
    revalidateOnMount: true, // Always fetch fresh data when component mounts
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
    CRONITOR_ALLOWED_IPS: '',
    CRONITOR_CORS_ALLOWED_ORIGINS: '',
    CRONITOR_USERS: '',
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
    CRONITOR_ALLOWED_IPS: false,
    CRONITOR_CORS_ALLOWED_ORIGINS: false,
    CRONITOR_USERS: false,
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
        CRONITOR_EXCLUDE_TEXT: data.CRONITOR_EXCLUDE_TEXT || [],
        CRONITOR_CORS_ALLOWED_ORIGINS: data.cors_allowed_origins || ''
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
      const response = await csrfFetch('/api/settings', {
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

  const fieldDescriptions = {
    "CRONITOR_LOG": "Where to write a comprehensive log file for auditing and troubleshooting.<br/><br/>Overridden by CRONITOR_LOG environment variable.",
    "CRONITOR_ENV": "Environment identifier (e.g. staging, production) added to monitor data.<br/><br/>Overridden by CRONITOR_ENV environment variable.",
    "CRONITOR_API_KEY": "Your Cronitor API key for syncing monitors and job data.<br/><br/>Overridden by CRONITOR_API_KEY environment variable.",
    "CRONITOR_PING_API_KEY": "API key for sending telemetry pings (optional, uses API key if not set).<br/><br/>Overridden by CRONITOR_PING_API_KEY environment variable.",
    "CRONITOR_EXCLUDE_TEXT": "Text to exclude when generating monitor names from commands.<br/><br/>Overridden by CRONITOR_EXCLUDE_TEXT environment variable.",
    "CRONITOR_HOSTNAME": "Set a hostname for generating monitor names.<br/><br/>Overridden by CRONITOR_HOSTNAME environment variable.",
    "CRONITOR_DASH_USER": "Username for accessing the dashboard.<br/><br/>Overridden by CRONITOR_DASH_USER environment variable.",
    "CRONITOR_DASH_PASS": "Password for accessing the dashboard.<br/><br/>Overridden by CRONITOR_DASH_PASS environment variable.",
    "CRONITOR_ALLOWED_IPS": "Restrict dashboard access to specific IP addresses or ranges.<br/><br/>Overridden by CRONITOR_ALLOWED_IPS environment variable.",
    "CRONITOR_CORS_ALLOWED_ORIGINS": "Allow cross-origin requests from specific domains for API access.<br/><br/>Overridden by CRONITOR_CORS_ALLOWED_ORIGINS environment variable.",
    "CRONITOR_USERS": "Comma-separated list of users whose crontabs to include when scanning (default: current user only).<br/><br/>Overridden by CRONITOR_USERS environment variable."
  };

  const renderInput = (name, label, type = "text", value, onChange, disabled = false) => (
    <div>
      <label className="flex items-center text-sm font-medium text-gray-700 dark:text-gray-300">
        {label}
        <Tooltip text={fieldDescriptions[name]}>
          <QuestionMarkCircleIcon 
            className="ml-2 h-4 w-4 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300" 
          />
        </Tooltip>
      </label>
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

      <form onSubmit={handleSubmit} className="space-y-8">
        {/* All Commands Section */}
        <section>
          <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 border-b border-gray-200 dark:border-gray-700 pb-2">
            General
          </h2>
          <div className="space-y-4">
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

            <div>
              <label className="flex items-center text-sm font-medium text-gray-700 dark:text-gray-300">
                Users
                <Tooltip text={fieldDescriptions["CRONITOR_USERS"]}>
                  <QuestionMarkCircleIcon 
                    className="ml-2 h-4 w-4 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300" 
                  />
                </Tooltip>
              </label>
              <div className="mt-1">
                <input
                  type="text"
                  name="CRONITOR_USERS"
                  value={formData.CRONITOR_USERS || ''}
                  onChange={handleChange}
                  disabled={envVars["CRONITOR_USERS"]}
                  placeholder="root, admin, user1"
                  className={`block w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 dark:bg-gray-700 dark:border-gray-600 dark:text-white px-4 py-2 ${
                    envVars["CRONITOR_USERS"] ? 'bg-gray-100 dark:bg-gray-800 cursor-not-allowed' : ''
                  }`}
                />
              </div>
              <div className="mt-1 text-sm text-gray-500 dark:text-gray-400">
                Comma-separated list of users whose crontabs to include when scanning for jobs. Leave empty to use current user only.
                Example: <code className="bg-gray-100 dark:bg-gray-800 px-1 rounded">root</code>, <code className="bg-gray-100 dark:bg-gray-800 px-1 rounded">admin</code>, <code className="bg-gray-100 dark:bg-gray-800 px-1 rounded">user1</code>
              </div>
              {envVars["CRONITOR_USERS"] && (
                <div className="mt-1 flex items-center text-sm text-yellow-600 dark:text-yellow-400">
                  <ExclamationTriangleIcon className="h-4 w-4 mr-1" />
                  Currently set as an environment variable
                </div>
              )}
            </div>
          </div>
        </section>

        {/* Cronitor Sync Section */}
        <section>
          <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 border-b border-gray-200 dark:border-gray-700 pb-2">
            Cronitor Sync
          </h2>
          <div className="space-y-4">
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
              "Telemetry API Key",
              "text",
              formData.CRONITOR_PING_API_KEY,
              handleChange,
              envVars["CRONITOR_PING_API_KEY"]
            )}

            <div>
              <label className="flex items-center text-sm font-medium text-gray-700 dark:text-gray-300">
                Exclude Text (comma-separated)
                <Tooltip text={fieldDescriptions["CRONITOR_EXCLUDE_TEXT"]}>
                  <QuestionMarkCircleIcon 
                    className="ml-2 h-4 w-4 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300" 
                  />
                </Tooltip>
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
          </div>
        </section>

        {/* Dashboard Section */}
        <section>
          <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 border-b border-gray-200 dark:border-gray-700 pb-2">
            Dashboard
          </h2>
          <div className="space-y-4">
            {renderInput(
              "CRONITOR_DASH_USER",
              "Username",
              "text",
              formData.CRONITOR_DASH_USER,
              handleChange,
              envVars["CRONITOR_DASH_USER"]
            )}

            {renderInput(
              "CRONITOR_DASH_PASS",
              "Password",
              showPassword ? "text" : "password",
              formData.CRONITOR_DASH_PASS,
              handleChange,
              envVars["CRONITOR_DASH_PASS"]
            )}

            <div>
              <label className="flex items-center text-sm font-medium text-gray-700 dark:text-gray-300">
                Allowed IP Addresses
                <Tooltip text={fieldDescriptions["CRONITOR_ALLOWED_IPS"]}>
                  <QuestionMarkCircleIcon 
                    className="ml-2 h-4 w-4 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300" 
                  />
                </Tooltip>
              </label>
              <div className="mt-1">
                <input
                  type="text"
                  name="CRONITOR_ALLOWED_IPS"
                  value={formData.CRONITOR_ALLOWED_IPS || ''}
                  onChange={handleChange}
                  disabled={envVars["CRONITOR_ALLOWED_IPS"]}
                  placeholder="192.168.1.0/24, 10.0.0.1, 2001:db8::/32"
                  className={`block w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 dark:bg-gray-700 dark:border-gray-600 dark:text-white px-4 py-2 ${
                    envVars["CRONITOR_ALLOWED_IPS"] ? 'bg-gray-100 dark:bg-gray-800 cursor-not-allowed' : ''
                  }`}
                />
              </div>
              <div className="mt-1 text-sm text-gray-500 dark:text-gray-400">
                Comma-separated list of IP addresses and CIDR ranges. Leave empty to allow all IPs.
                Example: <code className="bg-gray-100 dark:bg-gray-800 px-1 rounded">192.168.1.0/24</code>, <code className="bg-gray-100 dark:bg-gray-800 px-1 rounded">10.0.0.1</code>, <code className="bg-gray-100 dark:bg-gray-800 px-1 rounded">2001:db8::/32</code>
                {data.client_ip && (
                  <>
                    <br />
                    <span className="mt-1 text-sm text-gray-500 dark:text-gray-400">Your current IP: </span>
                    <code className="bg-gray-100 dark:bg-gray-800 px-1 rounded">{data.client_ip}</code>
                  </>
                )}
              </div>
              {envVars["CRONITOR_ALLOWED_IPS"] && (
                <div className="mt-1 flex items-center text-sm text-yellow-600 dark:text-yellow-400">
                  <ExclamationTriangleIcon className="h-4 w-4 mr-1" />
                  Currently set as an environment variable
                </div>
              )}
            </div>

            <div>
              <label className="flex items-center text-sm font-medium text-gray-700 dark:text-gray-300">
                CORS Allowed Origins
                <Tooltip text={fieldDescriptions["CRONITOR_CORS_ALLOWED_ORIGINS"]}>
                  <QuestionMarkCircleIcon 
                    className="ml-2 h-4 w-4 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300" 
                  />
                </Tooltip>
              </label>
              <div className="mt-1">
                <input
                  type="text"
                  name="CRONITOR_CORS_ALLOWED_ORIGINS"
                  value={formData.CRONITOR_CORS_ALLOWED_ORIGINS || ''}
                  onChange={handleChange}
                  disabled={envVars["CRONITOR_CORS_ALLOWED_ORIGINS"]}
                  placeholder="https://example.com, https://app.example.com"
                  className={`block w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 dark:bg-gray-700 dark:border-gray-600 dark:text-white px-4 py-2 ${
                    envVars["CRONITOR_CORS_ALLOWED_ORIGINS"] ? 'bg-gray-100 dark:bg-gray-800 cursor-not-allowed' : ''
                  }`}
                />
              </div>
              <div className="mt-1 text-sm text-gray-500 dark:text-gray-400">
                Comma-separated list of allowed origins for Cross-Origin Resource Sharing (CORS). Leave empty for strict same-origin policy (recommended). 
                Example: <code className="bg-gray-100 dark:bg-gray-800 px-1 rounded">https://app.example.com</code>, <code className="bg-gray-100 dark:bg-gray-800 px-1 rounded">https://dashboard.example.com</code>
                <br />
                <strong>Warning:</strong> Use <code className="bg-gray-100 dark:bg-gray-800 px-1 rounded">*</code> to allow all origins (not recommended for production).
              </div>
              {envVars["CRONITOR_CORS_ALLOWED_ORIGINS"] && (
                <div className="mt-1 flex items-center text-sm text-yellow-600 dark:text-yellow-400">
                  <ExclamationTriangleIcon className="h-4 w-4 mr-1" />
                  Currently set as an environment variable
                </div>
              )}
            </div>
          </div>
        </section>

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