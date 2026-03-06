# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| 1.x     | ✅ Active          |
| < 1.0   | ❌ Not supported   |

## Reporting a Vulnerability

If you discover a security vulnerability in Niyantrak, please report it
responsibly:

1. **Do NOT open a public GitHub issue.**
2. Email **abhipray.puttanpal@gmail.com** with:
   - A description of the vulnerability.
   - Steps to reproduce (or a proof-of-concept).
   - The impact / severity you assess.
3. You will receive an acknowledgement within **48 hours**.
4. A fix will be developed privately and a security advisory published
   once a patched version is released.

## Scope

The following are in scope:

- SQL injection through backend prefix parameters.
- Race conditions leading to bypass of rate limits.
- Denial-of-service through crafted keys or configurations.
- Information disclosure via error messages or logging.
- Dependency vulnerabilities in direct dependencies.

## Security Best Practices for Users

- Always validate and sanitize rate-limit keys before passing them to the
  limiter (e.g., avoid user-controlled strings as Redis key prefixes).
- Use TLS when connecting to Redis or PostgreSQL in production.
- Keep dependencies up to date — run `go get -u ./...` and review changes.
- Enable the `FailClosed` strategy if availability of the rate limiter is
  security-critical.
