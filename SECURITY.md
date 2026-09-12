# Security

TraceFrugal does not discover credentials, execute trace contents, or upload
telemetry. The local dashboard binds to loopback and checks Host and Origin
for writes. Native import reads local Claude Code and Codex transcript stores;
it exports usage metadata, not transcript bodies.

An explicitly started Claude trial creates one previewed rule file and retains
observations. The opt-in MCP proxy starts the upstream command you configure,
which may itself access network services or credentials. It inherits the host
environment; command arguments are never evaluated by a shell.

MCP packing stores **full original tool text** in its private archive directory.
Treat this as sensitive as the source data. Stop/expiry preserve archives for
recall and do not delete them. The dashboard exposes metadata, not archive
content. Do not put state directories or real exports in a public repository.

Only explicitly allowlisted tools with an upstream read-only hint can have
eligible text shortened. That hint is metadata from the server, not a security
attestation. Use trusted servers. Same-user malicious filesystem races are
outside the local process security boundary.

Keep private traces outside the repository. `/private/` and `/runs/` are ignored by the supplied `.gitignore`.

For a security vulnerability, use this repository's private vulnerability reporting feature when available. Do not place secrets or private traces in a public issue. General accounting mistakes can be reported publicly with synthetic data.

Only the latest released version is supported during the initial 0.x series.
