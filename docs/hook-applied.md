# Audit protection hook

This project has a hook (`.claude/settings.json` + `protect-audit.js`) that blocks `Edit`/`Write`/`MultiEdit` on any file path containing `"audit"` — which covers exactly the files this change needs:

- `audit.go`
- `postgres_repository.go`
- `handler.go`

I'm not going to route around that block (e.g. via shell redirection or `sed` through Bash) — a hook that explicitly protects the audit service is very likely intentional (audit trails are often deliberately locked down so agents/tools can't tamper with the record).