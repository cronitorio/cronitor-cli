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

- Add IP-based access restrictions
  - Problem: No way to restrict access to specific IP ranges
  - Implementation: Add configurable IP whitelist/blacklist with CIDR support, implement middleware to check client IPs
  - Details:
    • Accept CRONITOR_ALLOWED_IPS environment variable with comma-separated CIDR notation, if empty then no filtering is applied
    • Use net.ParseCIDR() to parse and validate IP ranges during startup
    • Implement middleware that extracts client IP from X-Forwarded-For, X-Real-IP, or RemoteAddr
    • Create IPFilter middleware that checks client IP against whitelist using net.Contains()
    • Support both IPv4 and IPv6 addresses, with proper handling of IPv4-mapped IPv6 addresses

- Add support for TLS/HTTPS
  - Problem: Current implementation allows unencrypted communication
  - Implementation: Add TLS configuration with modern cipher suites, make HTTPS mandatory in production
  - Details:
    • Implement automated certificate acquisition using ACME/Let's Encrypt protocol
    • Accept CRONITOR_ACME_EMAIL and CRONITOR_ACME_DIRECTORY environment variables for Let's Encrypt configuration
    • Configure TLS with minimum version 1.2 and secure cipher suites (exclude RC4, DES, 3DES)
    • Implement HTTP to HTTPS redirect middleware when TLS is enabled
    • Add CRONITOR_TLS_REQUIRED environment variable to enforce HTTPS-only mode
    • Support automatic certificate renewal with 90-day Let's Encrypt cycle
    • Fallback to manual certificate files via CRONITOR_TLS_CERT_FILE/CRONITOR_TLS_KEY_FILE for custom deployments


### 2. Command Execution Security
#### Current State
- No logging of command execution

#### Proposed Improvements
- Implement comprehensive command execution logging
  - Problem: No audit trail of command execution
  - Implementation: Log all command executions with user, command, arguments, and outcome - log to syslog unless a path is provided in settings
  - Details:
    • Create structured log entries with timestamp, user ID, command, arguments, exit code, and execution duration
    • Use log/syslog package to send logs to system syslog daemon with LOG_AUTH facility
    • Implement CRONITOR_LOG_FILE environment variable to override syslog and write to specific file
    • Include process PID, working directory, and environment variables (sanitized) in log entries
    • Implement log rotation and retention policies when writing to files (max 100MB, keep 10 files)


### 3. Process Management Security
#### Current State
- No validation of PIDs before killing processes
- No permission checks before killing processes
- No logging of process kills
- No process ownership verification

#### Proposed Improvements
- Add PID validation
  - Problem: No validation of process IDs before killing
  - Implementation: Validate PID format and existence before attempting to kill
  - Details:
    • Check PID is a positive integer using strconv.Atoi() and range validation (1-4194304 on Linux)
    • Verify process exists by checking /proc/{pid}/stat file or using os.FindProcess()
    • Implement process state validation to ensure process is not a kernel thread
    • Add safety checks to prevent killing PID 1 (init) or other critical system processes
    • Return descriptive error messages for invalid PIDs or non-existent processes

- Implement process ownership checks
  - Problem: Can kill processes started by any parent
  - Implementation: Verify that the process parent is either cron, crond, launchd (as appropriate) or this same binary or another instance of this "cronitor" binary. When finding associated processes, only return those with this criteria, and check it again during kill process handling.
  - Details:
    • Read process parent from /proc/{pid} or use os/user package
    • Maintain whitelist of allowed process parents: cron, crond, launchd user IDs, cronitor
    • Check process executable path using /proc/{pid}/exe to verify it's cronitor binary
    • Implement process tree validation to ensure child processes belong to cron jobs or to cronitor itself. 

- Add comprehensive process management logging
  - Problem: No audit trail of process management operations
  - Implementation: Log all process management operations with user, action, and outcome - log to syslog unless a path is provided in settings
  - Details:
    • Log process kill attempts with PID, process name, owner, and result status
    • Include process command line and parent PID in log entries using /proc/{pid}/cmdline
    • Use structured logging format with consistent fields for parsing and analysis
    • Send logs to syslog with LOG_CRIT facility for failed operations, LOG_INFO for successful ones
    • Implement configurable log levels and filtering options via CRONITOR_LOG_LEVEL

### 4. File Operations Security
#### Current State
- No validation of file paths
- No permission checks for file operations
- No logging of file operations
- Potential path traversal vulnerabilities

#### Proposed Improvements
- Implement file path validation
  - Problem: No validation of file paths before operations
  - Implementation: Validate and sanitize all file paths, prevent directory traversal. this binary should only be able to write to /etc/cronitor, /etc/cron.d/, /etc/crontab, /tmp
  - Details:
    • Define whitelist of allowed directories: /etc/cronitor, /etc/cron.d/, /etc/crontab, /tmp
    • Use filepath.Clean() and filepath.Abs() to resolve and normalize all paths
    • Check resolved paths start with allowed prefixes using strings.HasPrefix()
    • Reject paths containing "..", symbolic links, or null bytes
    • Implement path validation middleware that runs before any file operation

- Add comprehensive file operation logging
  - Problem: No audit trail of file operations
  - Implementation: Log all file operations with user, action, and file path - log to syslog unless a path is provided in settings
  - Details:
    • Log file operations with timestamp, user, action (read/write/delete), path, and size
    • Include file permissions, ownership, and modification time in log entries
    • Use structured logging with consistent field names for automated analysis
    • Send logs to syslog with LOG_AUTHPRIV facility for security-sensitive operations
    • Add file content hashing (SHA256) for integrity verification in logs

- Implement path traversal protection
  - Problem: Vulnerable to directory traversal attacks
  - Implementation: Sanitize paths and use absolute path resolution
  - Details:
    • Use filepath.Join() exclusively for path construction to prevent manual concatenation
    • Implement chroot-style path resolution that treats allowed directories as filesystem roots
    • Reject any path operation that would access files outside allowed directories
    • Add comprehensive test suite for edge cases like Unicode normalization attacks
    • Implement filesystem permission checks using os.Stat() before file operations


### 5. Network Security
#### Current State
- No TLS/HTTPS enforcement
- No IP-based access restrictions
- No proper CORS configuration
- No request rate limiting

#### Proposed Improvements
- Add TLS/HTTPS support with modern cipher suites
  - Problem: No encryption for network traffic
  - Implementation: Configure TLS with modern cipher suites and proper certificate handling
  - Details:
    • Configure TLS 1.2+ only with secure cipher suites excluding weak algorithms (RC4, DES, MD5)
    • Implement certificate validation with OCSP stapling and proper chain verification
    • Add support for HSTS headers with max-age of 1 year and includeSubDomains
    • Use crypto/tls.Config with MinVersion: tls.VersionTLS12 and CurvePreferences
    • Implement automatic certificate renewal using ACME/Let's Encrypt when available

- Implement IP-based access restrictions
  - Problem: No IP-based access control
  - Implementation: Add configurable IP whitelist/blacklist with CIDR support
  - Details:
    • Parse CRONITOR_ALLOWED_IPS environment variables
    • Use net.IPNet.Contains() for efficient IP range checking in middleware
    • Support IPv4, IPv6, and mixed environments with proper precedence rules
    • Implement X-Forwarded-For header parsing for proxy/load balancer scenarios
    • Add /health endpoint that bypasses IP restrictions for monitoring systems

- Configure proper CORS policies
  - Problem: No CORS configuration
  - Implementation: Add strict CORS policies with configurable allowed origins
  - Details:
    • Implement CORS middleware using gorilla/handlers or custom implementation
    • Set strict default policy: only same-origin requests allowed
    • Add CRONITOR_CORS_ALLOWED_ORIGINS environment variable for configuration
    • Configure secure headers: Access-Control-Allow-Credentials: false by default
    • Implement preflight request handling with proper method and header validation

- Implement request size limits
  - Problem: No limit on request size
  - Implementation: Add size limit, we should never have very large requests
  - Details:
    • Set maximum request body size to 1MB using http.MaxBytesReader()
    • Implement separate limits for different content types (JSON: 100KB, form data: 10KB)
    • Add request header size limits to prevent header-based attacks
    • Use middleware to enforce limits before request processing begins
    • Return HTTP 413 (Payload Too Large) with appropriate error messages

- Add support for reverse proxy configurations
  - Problem: No support for running behind reverse proxy
  - Implementation: Add proper headers and configuration for reverse proxy support
  - Details:
    • Trust X-Forwarded-For, X-Forwarded-Proto, X-Real-IP headers
    • Implement proper client IP extraction with configurable proxy IP ranges
    • Add support for X-Forwarded-Host header for multi-tenant deployments
    • Configure proper redirect URLs and cookie domains for proxy scenarios
    • Validate and sanitize all forwarded headers to prevent header injection attacks

### 6. Logging & Monitoring
#### Current State
- Limited logging of security events
- No audit trail for sensitive operations
- No monitoring for suspicious activities

#### Proposed Improvements
- Implement comprehensive security event logging
  - Problem: Insufficient security event logging
  - Implementation: Add structured logging for all security events - log to syslog unless a path is provided in settings
  - Details:
    • Define security events: authentication attempts, authorization failures, suspicious activity
    • Use structured JSON logging with consistent fields: timestamp, user, action, resource, outcome
    • Send to syslog with appropriate facilities (LOG_AUTH, LOG_AUTHPRIV, LOG_SECURITY)
    • Implement CRONITOR_SECURITY_LOG_FILE environment variable for file-based logging
    • Add log level filtering and configurable verbosity via CRONITOR_LOG_LEVEL

- Add audit trail for sensitive operations
  - Problem: No audit trail for sensitive operations
  - Implementation: Log all sensitive operations with user, action, and outcome
  - Details:
    • Define sensitive operations: process kills, file modifications, configuration changes
    • Create immutable audit log entries with cryptographic signatures using HMAC-SHA256
    • Include before/after state for configuration changes and file modifications
    • Implement audit log rotation with integrity verification and tamper detection
    • Add CRONITOR_AUDIT_RETENTION environment variable for retention period (default 90 days)

- Add support for external logging systems
  - Problem: No integration with external logging
  - Implementation: Add support for syslog and other external logging systems
  - Details:
    • Implement syslog client supporting RFC 5424 format with structured data
    • Add support for remote syslog servers via TCP/UDP with TLS encryption
    • Integrate with popular log aggregation systems (ELK stack, Splunk, Fluentd)
    • Support multiple log outputs simultaneously (local file + remote syslog)
    • Add CRONITOR_LOG_ENDPOINTS environment variable for configuring multiple destinations

