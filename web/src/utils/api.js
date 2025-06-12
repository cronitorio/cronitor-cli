/**
 * CSRF-aware fetch utility
 * Automatically includes CSRF tokens in POST/PUT/DELETE requests
 */

// Get CSRF token from cookie
function getCSRFTokenFromCookie() {
  const value = `; ${document.cookie}`;
  const parts = value.split(`; csrf_token=`);
  if (parts.length === 2) return parts.pop().split(';').shift();
  return null;
}

// Get CSRF token from the most recent response header (stored in memory)
let lastCSRFToken = null;

// CSRF-aware fetch function
export async function csrfFetch(url, options = {}) {
  const method = options.method?.toUpperCase() || 'GET';
  
  // For state-changing requests, include CSRF token
  if (method === 'POST' || method === 'PUT' || method === 'DELETE') {
    const token = lastCSRFToken || getCSRFTokenFromCookie();
    
    if (token) {
      // Ensure headers object exists
      options.headers = options.headers || {};
      
      // Add CSRF token to X-CSRF-Token header
      options.headers['X-CSRF-Token'] = token;
    }
  }
  
  // Make the request
  const response = await fetch(url, options);
  
  // Store new CSRF token if provided in response
  const newToken = response.headers.get('X-CSRF-Token');
  if (newToken) {
    lastCSRFToken = newToken;
  }
  
  return response;
}

// CSRF-aware fetcher for SWR
export const csrfFetcher = async (url) => {
  const response = await csrfFetch(url);
  
  if (!response.ok) {
    throw new Error(`HTTP ${response.status}: ${response.statusText}`);
  }
  
  return response.json();
};

// Convenience methods
export const api = {
  get: (url, options = {}) => csrfFetch(url, { ...options, method: 'GET' }),
  post: (url, data, options = {}) => {
    const headers = { 'Content-Type': 'application/json', ...options.headers };
    return csrfFetch(url, {
      ...options,
      method: 'POST',
      headers,
      body: JSON.stringify(data)
    });
  },
  put: (url, data, options = {}) => {
    const headers = { 'Content-Type': 'application/json', ...options.headers };
    return csrfFetch(url, {
      ...options,
      method: 'PUT',
      headers,
      body: JSON.stringify(data)
    });
  },
  delete: (url, options = {}) => csrfFetch(url, { ...options, method: 'DELETE' }),
};

export default csrfFetch; 