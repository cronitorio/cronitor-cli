# Cronitor Dashboard Security Plan

## Overview
The Cronitor dashboard provides a web interface for managing cron jobs and monitors. Due to its ability to execute commands and manage system processes, it requires careful security considerations to prevent unauthorized access and potential privilege escalation.

## Current Security Features
- Basic authentication using username/password
- Configurable credentials via environment variables
- Embedded static file serving

## Security Concerns

### 1. Authentication & Access Control
#### Current State
- Basic authentication is implemented
- No rate limiting on authentication attempts
- No CSRF protection
- No IP-based access restrictions

#### Proposed Improvements
- ✅ Implement rate limiting for authentication attempts
  - Problem: Prevents brute force attacks by limiting failed login attempts
  - Implementation: Use golang.org/x/time/rate package to implement a token bucket rate limiter, storing attempt counts in memory with a 5-minute window
  - Details:
    • ✅ Create a map[string]*rate.Limiter indexed by client IP address to track per-IP rate limits
    • ✅ Set limit to 5 failed attempts per minute using rate.NewLimiter(rate.Every(12*time.Second), 5)
    • ✅ On authentication failure, check if client IP has exceeded rate limit before processing
    • ✅ Implement cleanup goroutine to remove stale entries from the map every 10 minutes
    • ✅ Return HTTP 429 (Too Many Requests) with Retry-After header when rate limit exceeded

- ✅ Add CSRF protection for all POST/PUT/DELETE requests
  - Problem: Current implementation vulnerable to cross-site request forgery attacks
  - Implementation: Add CSRF tokens to all forms and API requests, validate tokens on server side
  - Details:
    • ✅ Generate cryptographically secure random CSRF tokens using crypto/rand package
    • ✅ Store tokens in secure HTTP-only cookies with SameSite=Strict attribute
    • ✅ Add hidden CSRF token fields to all HTML forms and X-CSRF-Token header to AJAX requests
    • ✅ Implement middleware to validate CSRF tokens on all state-changing requests (POST/PUT/DELETE)
    • ✅ Regenerate CSRF tokens after successful authentication to prevent session fixation

- ✅ Add IP-based access restrictions
  - Problem: No way to restrict access to specific IP ranges
  - Implementation: Add configurable IP whitelist/blacklist with CIDR support, implement middleware to check client IPs
  - Details:
    • ✅ Accept CRONITOR_ALLOWED_IPS environment variable with comma-separated CIDR notation, if empty then no filtering is applied
    • ✅ Use net.ParseCIDR() to parse and validate IP ranges during startup
    • ✅ Implement middleware that extracts client IP from X-Forwarded-For, X-Real-IP, or RemoteAddr
    • ✅ Create IPFilter middleware that checks client IP against whitelist using net.Contains()
    • ✅ Support both IPv4 and IPv6 addresses, with proper handling of IPv4-mapped IPv6 addresses
    • ✅ Add this field to the Settings page


### 3. Process Management Security
#### Current State
- No validation of PIDs before killing processes
- No permission checks before killing processes
- No logging of process kills
- No process ownership verification

#### Proposed Improvements
- ✅ Add PID validation
  - Problem: No validation of process IDs before killing
  - Implementation: Validate PID format and existence before attempting to kill
  - Details:
    • ✅ Check PID is a positive integer using strconv.Atoi() and range validation (1-4194304 on Linux)
    • ✅ Verify process exists by checking /proc/{pid}/stat file or using os.FindProcess()
    • ✅ Implement process state validation to ensure process is not a kernel thread
    • ✅ Add safety checks to prevent killing PID 1 (init) or other critical system processes
    • ✅ Return descriptive error messages for invalid PIDs or non-existent processes

### 5. Network Security
#### Current State
- No TLS/HTTPS enforcement
- No IP-based access restrictions
- No proper CORS configuration
- No request rate limiting

#### Proposed Improvements

- ✅ Configure proper CORS policies
  - Problem: No CORS configuration
  - Implementation: Add strict CORS policies with configurable allowed origins
  - Details:
    • ✅ Implement CORS middleware using gorilla/handlers or custom implementation
    • ✅ Set strict default policy: only same-origin requests allowed
    • ✅ Add CRONITOR_CORS_ALLOWED_ORIGINS environment variable for configuration
    • ✅ Configure secure headers: Access-Control-Allow-Credentials: false by default
    • ✅ Implement preflight request handling with proper method and header validation

- ✅ Implement request size limits
  - Problem: No limit on request size
  - Implementation: Add size limit, we should never have very large requests
  - Details:
    • ✅ Set maximum request body size to 1MB using http.MaxBytesReader()
    • ✅ Implement separate limits for different content types (JSON: 100KB, form data: 10KB)
    • ✅ Add request header size limits to prevent header-based attacks
    • ✅ Use middleware to enforce limits before request processing begins
    • ✅ Return HTTP 413 (Payload Too Large) with appropriate error messages

- Add support for reverse proxy configurations
  - Problem: No support for running behind reverse proxy
  - Implementation: Add proper headers and configuration for reverse proxy support
  - Details:
    • Trust X-Forwarded-For, X-Forwarded-Proto, X-Real-IP headers
    • Implement proper client IP extraction with configurable proxy IP ranges
    • Add support for X-Forwarded-Host header for multi-tenant deployments
    • Configure proper redirect URLs and cookie domains for proxy scenarios
    • Validate and sanitize all forwarded headers to prevent header injection attacks