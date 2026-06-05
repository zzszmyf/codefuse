# Security Policy

## Supported Versions

| Version | Supported          |
|---------|-------------------|
| latest  | :white_check_mark: |
| < latest| :x:                |

## Reporting a Vulnerability

If you discover a security vulnerability in CodeFuse, please report it responsibly.

**Do not** open a public issue. Instead, email the maintainers directly at:

> [INSERT SECURITY EMAIL] — *placeholder, replace with actual contact*

Please include:

- A description of the vulnerability
- Steps to reproduce
- Affected versions
- Potential impact
- Suggested fix (if any)

We will acknowledge receipt within 48 hours and provide a timeline for a fix.

## Disclosure Policy

We follow a coordinated disclosure process:

1. Reporter submits vulnerability privately
2. Maintainers acknowledge and investigate
3. Fix is developed and tested
4. Fix is released
5. Public disclosure after 30 days or sooner if exploit is detected in the wild

## Security Best Practices for Users

- Keep CodeFuse updated to the latest version
- Do not commit `.codefuse/index.json` to version control if it contains sensitive code paths
- FUSE mounts should use restricted permissions
