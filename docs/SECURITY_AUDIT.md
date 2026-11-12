# Security Audit and Penetration Testing Guide

This document provides comprehensive guidelines for conducting security audits and penetration testing on the Reddit Cluster Map application.

## Table of Contents

1. [Security Audit Checklist](#security-audit-checklist)
2. [Penetration Testing Procedures](#penetration-testing-procedures)
3. [Automated Security Testing](#automated-security-testing)
4. [Manual Security Testing](#manual-security-testing)
5. [Vulnerability Assessment](#vulnerability-assessment)
6. [Reporting and Remediation](#reporting-and-remediation)

## Security Audit Checklist

### Authentication & Authorization

- [ ] **Admin API Token Security**
  - [ ] Verify admin token is configured and meets minimum length (32 chars)
  - [ ] Test admin endpoints reject requests without valid token
  - [ ] Test admin endpoints reject requests with invalid/expired tokens
  - [ ] Test admin endpoints reject requests with malformed Authorization headers
  - [ ] Verify tokens are not logged in plain text
  - [ ] Verify tokens are transmitted only over HTTPS in production

- [ ] **OAuth Token Management**
  - [ ] Verify OAuth credentials are stored securely
  - [ ] Test token refresh mechanism works correctly
  - [ ] Verify expired tokens are rejected
  - [ ] Test token rotation does not cause downtime
  - [ ] Verify refresh tokens are stored encrypted

### Input Validation & Sanitization

- [ ] **API Input Validation**
  - [ ] Test SQL injection on all database queries
  - [ ] Test XSS (Cross-Site Scripting) on all text inputs
  - [ ] Test path traversal on file operations
  - [ ] Test command injection on system calls
  - [ ] Test LDAP injection if applicable
  - [ ] Test XML injection if applicable
  - [ ] Test JSON injection on API endpoints
  - [ ] Verify proper input length limits
  - [ ] Verify special character handling

- [ ] **Query Parameter Validation**
  - [ ] Test negative values where not allowed
  - [ ] Test excessively large values
  - [ ] Test invalid data types
  - [ ] Test boundary conditions
  - [ ] Test null/undefined values

### Rate Limiting & DoS Protection

- [ ] **Rate Limiting Tests**
  - [ ] Verify global rate limit is enforced (default: 100 rps)
  - [ ] Verify per-IP rate limit is enforced (default: 10 rps)
  - [ ] Test burst limits work correctly
  - [ ] Test rate limit headers are set correctly
  - [ ] Verify rate limiting cannot be bypassed with different headers
  - [ ] Test distributed DoS scenarios with multiple IPs

- [ ] **Resource Exhaustion**
  - [ ] Test large payload handling
  - [ ] Test memory exhaustion scenarios
  - [ ] Test database connection pooling limits
  - [ ] Test concurrent request handling

### Security Headers & CSP

- [ ] **HTTP Security Headers**
  - [ ] Verify X-Content-Type-Options: nosniff
  - [ ] Verify X-Frame-Options: DENY
  - [ ] Verify Referrer-Policy is set
  - [ ] Verify Content-Security-Policy is restrictive
  - [ ] Verify Strict-Transport-Security (HSTS) when using HTTPS
  - [ ] Verify Permissions-Policy is set appropriately

- [ ] **CORS Configuration**
  - [ ] Verify only allowed origins can access API
  - [ ] Test CORS preflight requests
  - [ ] Verify credentials are handled correctly
  - [ ] Test wildcard origin restrictions

### Cryptography & Data Protection

- [ ] **Encryption at Rest**
  - [ ] Verify database passwords are not stored in plain text
  - [ ] Verify OAuth tokens are stored securely
  - [ ] Verify API keys/secrets are not in source code
  - [ ] Check for sensitive data in logs

- [ ] **Encryption in Transit**
  - [ ] Verify HTTPS/TLS is enforced in production
  - [ ] Test TLS version (should be 1.2 or higher)
  - [ ] Test cipher suite strength
  - [ ] Verify certificate validity
  - [ ] Test for mixed content issues

### Dependency & Supply Chain Security

- [ ] **Dependency Scanning**
  - [ ] Run `govulncheck` on Go dependencies
  - [ ] Run `npm audit` on Node.js dependencies
  - [ ] Review Dependabot alerts
  - [ ] Check Docker base image vulnerabilities
  - [ ] Verify no known CVEs in dependencies

- [ ] **Code Quality & Static Analysis**
  - [ ] Run CodeQL security queries
  - [ ] Review linter warnings
  - [ ] Check for hardcoded secrets
  - [ ] Review error handling practices

### Database Security

- [ ] **PostgreSQL Security**
  - [ ] Verify strong database passwords
  - [ ] Test SQL injection prevention
  - [ ] Verify database is not exposed publicly
  - [ ] Check database user permissions (principle of least privilege)
  - [ ] Verify backup encryption
  - [ ] Test connection string security

### Session & Cookie Management

- [ ] **Cookie Security** (if applicable)
  - [ ] Verify HttpOnly flag is set
  - [ ] Verify Secure flag is set (HTTPS only)
  - [ ] Verify SameSite attribute
  - [ ] Test session fixation attacks
  - [ ] Test session timeout

### API Security

- [ ] **REST API Security**
  - [ ] Test for information disclosure in error messages
  - [ ] Verify proper HTTP methods are enforced
  - [ ] Test for mass assignment vulnerabilities
  - [ ] Test for IDOR (Insecure Direct Object References)
  - [ ] Verify API versioning strategy
  - [ ] Test API documentation exposure

### Infrastructure Security

- [ ] **Docker Security**
  - [ ] Verify containers run as non-root user
  - [ ] Check for unnecessary exposed ports
  - [ ] Verify secret management in containers
  - [ ] Review Docker Compose configuration
  - [ ] Check for privileged containers

- [ ] **Network Security**
  - [ ] Verify internal services are not exposed
  - [ ] Test firewall rules
  - [ ] Verify database network isolation
  - [ ] Check for open ports

### Monitoring & Logging

- [ ] **Security Monitoring**
  - [ ] Verify failed authentication attempts are logged
  - [ ] Verify rate limit violations are logged
  - [ ] Test alerting for security events
  - [ ] Verify logs do not contain sensitive data
  - [ ] Test log injection attacks

## Penetration Testing Procedures

### Pre-Testing Phase

1. **Scope Definition**
   - Define in-scope systems and endpoints
   - Identify out-of-scope components
   - Get written authorization
   - Set testing windows

2. **Environment Setup**
   - Use dedicated testing environment
   - Backup production data if testing staging
   - Configure monitoring and logging
   - Prepare tools and scripts

3. **Reconnaissance**
   - Map API endpoints
   - Identify technologies and versions
   - Review public documentation
   - Enumerate user roles and permissions

### Active Testing Phase

#### 1. Authentication Testing

**Test Admin API Authentication:**
```bash
# Test without token
curl -v http://localhost:8080/api/admin/services

# Test with invalid token
curl -v -H "Authorization: Bearer invalid-token" \
  http://localhost:8080/api/admin/services

# Test with valid token
curl -v -H "Authorization: Bearer $ADMIN_API_TOKEN" \
  http://localhost:8080/api/admin/services
```

**Test Token Bruteforce Protection:**
```bash
# Run automated script to test rate limiting on auth endpoints
# Should be blocked after multiple failed attempts
for i in {1..20}; do
  curl -H "Authorization: Bearer wrong-token-$i" \
    http://localhost:8080/api/admin/services
  sleep 0.1
done
```

#### 2. Input Validation Testing

**Test SQL Injection:**
```bash
# Test on query parameters
curl "http://localhost:8080/api/graph?max_nodes=' OR '1'='1"

# Test on path parameters
curl "http://localhost:8080/api/communities/'; DROP TABLE users;--"
```

**Test XSS:**
```bash
# Test reflected XSS
curl "http://localhost:8080/api/graph?test=<script>alert('XSS')</script>"

# Test stored XSS via crawl endpoint
curl -X POST http://localhost:8080/api/crawl \
  -H "Content-Type: application/json" \
  -d '{"subreddit": "<script>alert(\"XSS\")</script>"}'
```

**Test Path Traversal:**
```bash
# Test backup download endpoint
curl -H "Authorization: Bearer $ADMIN_API_TOKEN" \
  "http://localhost:8080/api/admin/backups/../../../etc/passwd"
```

#### 3. Authorization Testing

**Test Horizontal Privilege Escalation:**
```bash
# Attempt to access other users' data
curl "http://localhost:8080/api/users/other_user_id"
```

**Test Vertical Privilege Escalation:**
```bash
# Try to access admin endpoints without proper token
curl -X POST http://localhost:8080/api/admin/services \
  -H "Content-Type: application/json" \
  -d '{"crawler": "stop"}'
```

#### 4. Rate Limiting Testing

**Test Global Rate Limit:**
```bash
# Send rapid requests to test rate limiting
ab -n 500 -c 50 http://localhost:8080/api/graph
```

**Test Per-IP Rate Limit:**
```bash
# Test from single IP
for i in {1..100}; do
  curl -w "%{http_code}\n" http://localhost:8080/api/graph
done | grep -c 429
```

#### 5. Business Logic Testing

**Test Resource Exhaustion:**
```bash
# Request maximum nodes/links
curl "http://localhost:8080/api/graph?max_nodes=999999999&max_links=999999999"

# Submit multiple concurrent crawl jobs
for i in {1..50}; do
  curl -X POST http://localhost:8080/api/crawl \
    -H "Content-Type: application/json" \
    -d '{"subreddit": "test'$i'"}' &
done
```

#### 6. API Abuse Testing

**Test Parameter Tampering:**
```bash
# Test negative values
curl "http://localhost:8080/api/graph?max_nodes=-1"

# Test invalid data types
curl "http://localhost:8080/api/graph?max_nodes=abc"

# Test extremely large values
curl "http://localhost:8080/api/graph?max_nodes=9999999999999999999"
```

### Post-Testing Phase

1. **Vulnerability Verification**
   - Confirm all findings
   - Categorize by severity (Critical/High/Medium/Low)
   - Document reproduction steps
   - Capture evidence (screenshots, logs)

2. **Report Generation**
   - Use the Security Audit Report template
   - Include executive summary
   - Provide detailed findings
   - Suggest remediation steps

3. **Cleanup**
   - Remove test data
   - Revert any changes
   - Archive test results

## Automated Security Testing

See `scripts/security-test.sh` for automated security testing script.

### Running Automated Tests

```bash
# Run all security tests
./scripts/security-test.sh

# Run specific test suite
./scripts/security-test.sh --suite auth
./scripts/security-test.sh --suite input
./scripts/security-test.sh --suite rate-limit
./scripts/security-test.sh --suite headers
```

### CI/CD Integration

Security tests run automatically on:
- Every push to main/develop branches
- Every pull request
- Daily scheduled scans
- Manual workflow trigger

See `.github/workflows/security.yml` for configuration.

## Manual Security Testing

### Tools Required

**Essential Tools:**
- `curl` - Command-line HTTP client
- `jq` - JSON processor
- `ab` (Apache Bench) - Load testing
- Browser Developer Tools

**Advanced Tools:**
- Burp Suite - Web vulnerability scanner
- OWASP ZAP - Security testing tool
- Nikto - Web server scanner
- sqlmap - SQL injection testing
- Postman - API testing

### Testing Workflow

1. **Start Testing Environment:**
   ```bash
   cd backend
   docker compose up -d
   ```

2. **Configure Environment:**
   ```bash
   # Set admin token for testing
   export ADMIN_API_TOKEN="test-token-for-security-audit"
   ```

3. **Run Manual Tests:**
   - Use Burp Suite to intercept requests
   - Test each endpoint manually
   - Try various attack vectors
   - Document findings

4. **Analyze Results:**
   - Review server logs
   - Check error responses
   - Verify security headers
   - Confirm rate limiting

## Vulnerability Assessment

### Severity Classification

**Critical:**
- Remote code execution
- SQL injection leading to data breach
- Authentication bypass
- Privilege escalation to admin

**High:**
- Stored XSS
- Sensitive data exposure
- Broken authentication
- Insecure deserialization

**Medium:**
- Reflected XSS
- CSRF without significant impact
- Information disclosure
- Insufficient logging

**Low:**
- Verbose error messages
- Missing security headers (non-critical)
- Weak cipher suites
- Version disclosure

### CVSS Scoring

Use CVSS v3.1 calculator for consistent scoring:
https://www.first.org/cvss/calculator/3.1

### Common Vulnerabilities to Check

Based on OWASP Top 10:
1. Broken Access Control
2. Cryptographic Failures
3. Injection
4. Insecure Design
5. Security Misconfiguration
6. Vulnerable and Outdated Components
7. Identification and Authentication Failures
8. Software and Data Integrity Failures
9. Security Logging and Monitoring Failures
10. Server-Side Request Forgery (SSRF)

## Reporting and Remediation

### Report Structure

1. **Executive Summary**
   - Overall security posture
   - Number of findings by severity
   - Key recommendations

2. **Scope and Methodology**
   - Systems tested
   - Testing approach
   - Tools used
   - Limitations

3. **Detailed Findings**
   For each vulnerability:
   - Title and description
   - Severity rating
   - Affected components
   - Reproduction steps
   - Evidence (screenshots, logs)
   - Impact analysis
   - Remediation recommendations
   - References (CWE, OWASP)

4. **Summary and Recommendations**
   - Quick wins
   - Long-term improvements
   - Security best practices

### Remediation Workflow

1. **Triage**
   - Verify vulnerability
   - Assign severity
   - Prioritize by risk

2. **Fix**
   - Develop patch
   - Test fix thoroughly
   - Code review

3. **Validate**
   - Re-test vulnerability
   - Verify fix doesn't break functionality
   - Run full test suite

4. **Document**
   - Update security documentation
   - Add regression tests
   - Track in issue tracker

### Regular Security Activities

**Weekly:**
- Review Dependabot alerts
- Check security workflow results
- Review access logs for anomalies

**Monthly:**
- Run manual penetration tests
- Review and rotate credentials
- Update security documentation

**Quarterly:**
- Comprehensive security audit
- External penetration test (recommended)
- Security training for team
- Review and update security policies

**Annually:**
- Third-party security assessment
- Compliance audit
- Incident response plan review
- Security architecture review

## References

- [OWASP Testing Guide](https://owasp.org/www-project-web-security-testing-guide/)
- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
- [NIST Cybersecurity Framework](https://www.nist.gov/cyberframework)
- [CWE Top 25](https://cwe.mitre.org/top25/)
- [SANS Penetration Testing](https://www.sans.org/penetration-testing/)

## Contact

For security issues, please create a private security advisory on GitHub at:
https://github.com/subculture-collective/clustr/security/advisories/new

Alternatively, open a confidential issue in the repository.

---

**Last Updated:** 2025-11-01  
**Next Review:** 2026-02-01
