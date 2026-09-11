# Security

TraceFrugal performs no network calls at runtime. It reads the files or stdin you specify and writes reports to stdout. It does not discover credentials, rewrite agent settings, or execute trace contents.

Keep private traces outside the repository. `/private/` and `/runs/` are ignored by the supplied `.gitignore`.

For a security vulnerability, use this repository's private vulnerability reporting feature when available. Do not place secrets or private traces in a public issue. General accounting mistakes can be reported publicly with synthetic data.

Only the latest released version is supported during the initial 0.x series.
