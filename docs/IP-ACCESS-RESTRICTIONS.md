# IP Access Restrictions

The Cronitor dashboard now supports IP-based access restrictions to enhance security by limiting access to specific IP addresses or IP ranges.

## Overview

When IP restrictions are configured, only requests from allowed IP addresses will be able to access the dashboard. This provides an additional layer of security beyond authentication.

## Configuration

### Environment Variable

Set the `CRONITOR_ALLOWED_IPS` environment variable with a comma-separated list of IP addresses and/or CIDR ranges:

```bash
# Allow specific IP addresses and ranges
export CRONITOR_ALLOWED_IPS="192.168.1.0/24,10.0.0.1,2001:db8::/32"

# Start the dashboard
cronitor dash
```

### Command Line Configuration

```bash
# Configure via command line
cronitor configure --allowed-ips "192.168.1.0/24,10.0.0.1"
```

### Settings Page

You can also configure IP restrictions through the web interface:

1. Open the Cronitor dashboard
2. Navigate to Settings
3. Find the "Allowed IP Addresses" field
4. Your current IP address will be displayed to help with configuration
5. Enter comma-separated IP addresses/ranges
6. Click "Save Settings"

## Supported Formats

### IPv4 Examples
- Single IP: `192.168.1.100`
- CIDR range: `192.168.1.0/24` (allows 192.168.1.1 - 192.168.1.254)
- Private networks: `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`

### IPv6 Examples
- Single IP: `2001:db8::1`
- CIDR range: `2001:db8::/32`

### Mixed Example
```bash
export CRONITOR_ALLOWED_IPS="192.168.1.0/24,10.0.0.1,2001:db8::/32,203.0.113.45"
```

## Behavior

- **No restrictions**: If `CRONITOR_ALLOWED_IPS` is empty or not set, all IPs are allowed
- **Stealth blocking**: Requests from non-allowed IPs get no response (connection appears closed/timeout)
- **Real-time updates**: IP restrictions can be updated through settings without restarting the dashboard
- **Proxy support**: Works correctly behind reverse proxies (respects X-Forwarded-For, X-Real-IP headers)

## Security Notes

- IP restrictions are applied **before** authentication, providing early filtering
- **Stealth mode**: Blocked IPs see no response, making it appear like no service is running
- Use CIDR notation for ranges to avoid listing individual IPs
- Consider your network topology (NAT, proxies) when configuring ranges
- Test configuration carefully to avoid locking yourself out

## Error Handling

- Invalid IP addresses or CIDR ranges will be rejected with descriptive error messages
- Configuration errors are logged and prevent the dashboard from starting
- The settings page will show validation errors for invalid IP configurations
- **Blocked IPs see no response** - connection appears closed or times out (stealth mode)

## Examples by Use Case

### Home Office
```bash
# Allow your home IP range
export CRONITOR_ALLOWED_IPS="203.0.113.0/24"
```

### Corporate Network
```bash
# Allow corporate network ranges
export CRONITOR_ALLOWED_IPS="10.0.0.0/8,172.16.0.0/12"
```

### Development Environment
```bash
# Allow localhost and development team IPs
export CRONITOR_ALLOWED_IPS="127.0.0.1,192.168.1.0/24,10.0.0.0/24"
```

### Cloud Environment
```bash
# Allow specific cloud service IPs
export CRONITOR_ALLOWED_IPS="52.0.0.0/8,13.0.0.0/8"
```

## Troubleshooting

### Locked Out
If you accidentally lock yourself out:
1. Access the server directly (SSH, console)
2. Unset the environment variable: `unset CRONITOR_ALLOWED_IPS`
3. Or edit the config file to remove the restriction
4. Restart the dashboard

### Stealth Mode Behavior
If the dashboard appears completely unreachable:
1. **Browser shows**: "This site can't be reached" or connection timeout
2. **curl shows**: "Empty reply from server" or connection closed
3. **Check server logs**: Look for IP filtering debug messages
4. **Test from allowed IP**: Verify service works from permitted addresses

### Behind a Proxy
Ensure your proxy passes the correct headers:
- `X-Forwarded-For`
- `X-Real-IP`

The dashboard will automatically detect the real client IP from these headers. 