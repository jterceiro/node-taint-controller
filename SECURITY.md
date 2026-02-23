# Security Policy

## Supported Versions

Only the latest code on the `main` branch receives security patches. We do not backport fixes to older releases.

| Version / Branch | Supported |
|---|---|
| `main` (latest) | ✅ Yes |
| Older tags | ❌ No |

If you are running an older version, please upgrade to the latest release before reporting an issue.

---

## Reporting a Vulnerability

**Please do NOT open a public GitHub issue for security vulnerabilities.**

We use [GitHub's Private Vulnerability Reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability) to handle security disclosures confidentially.

### How to report

1. Navigate to the [Security tab](https://github.com/jterceiro/node-taint-controller/security) of this repository.
2. Click **"Report a vulnerability"**.
3. Fill in the advisory form with as much detail as possible:
   - A clear description of the vulnerability and its potential impact.
   - Steps to reproduce or a proof-of-concept.
   - The affected component(s) and version(s).
   - Any suggested mitigations or fixes, if known.

A maintainer will acknowledge your report within **5 business days** and work with you on a coordinated disclosure timeline.

---

## Security Tooling & Supply Chain

This project uses the following tooling to maintain a secure software supply chain:

### Container Image Scanning — Trivy

Every container image build is scanned with [Trivy](https://github.com/aquasecurity/trivy) to detect known CVEs in both OS packages and Go dependencies before an image is published to the registry.

```sh
# Scan a locally built image
trivy image ghcr.io/jterceiro/node-taint-controller:latest
```

### Dependency Management — Dependabot

[Dependabot](https://docs.github.com/en/code-security/dependabot) is enabled for this repository and automatically opens pull requests to keep Go modules and GitHub Actions up to date, reducing exposure to vulnerable dependencies.

---

## Disclosure Policy

Once a vulnerability is confirmed and a fix is ready, we will:

1. Publish a patched release and tag it with a new semantic version.
2. Create a [GitHub Security Advisory](https://github.com/jterceiro/node-taint-controller/security/advisories) with full details and a CVE identifier (if applicable).
3. Credit the reporter in the advisory unless they prefer to remain anonymous.
