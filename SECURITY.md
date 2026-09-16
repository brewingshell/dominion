# Security policy

## Design context

dominion is a **LAN tool that grants shell access**. It is not hardened for the
public internet and should not be exposed to it. If you need remote access, put
it behind a VPN rather than a port forward.

Known, deliberate characteristics (not vulnerabilities):

- A single shared PIN (weak by default, `3232`) — change it via `DOMINION_PIN`.
- HTTPS with a self-signed local CA. Encryption, but no certificate verification
  unless the CA is installed or pinned by a client.
- Session tokens are held in memory only; a restart logs everyone out.
- The `dominion` session hosts the server and is intentionally unkillable.

## Reporting a vulnerability

Please **do not** open a public issue for anything exploitable. Report privately
via GitHub's "Report a vulnerability" (Security → Advisories) or by email to the
maintainer listed on the repository profile.

Include: a description, reproduction steps, the affected version/commit, and any
suggested mitigation. You will get an acknowledgement as soon as possible, and
credit in the fix unless you prefer otherwise.
