# Security policy

## Supported versions

lfr is in 0.x. Security fixes land in the latest release only, so please upgrade before reporting.

| Version | Supported |
| --- | --- |
| latest 0.2.x release | yes |
| anything older | no |

## Reporting a vulnerability

Please do not open a public issue. Report it privately through
[GitHub private vulnerability reporting](https://github.com/Anonyfox/lfr/security/advisories/new) (Security tab, "Report a
vulnerability"). Include the version (`lfr version`), what you did, what happened and, if you can, a
minimal reproduction.

You get a first answer within 72 hours. Once a fix is released, the advisory is published with credit
to you unless you prefer otherwise.

## Scope

lfr runs exposed network services, so reports about SMTP and submission, the HTTP and HTTPS servers (list websites, signup, unsubscribe, MTA-STS), DKIM, SPF and DMARC handling, message and Markdown parsing, signed links and tokens, and the local control socket are all in scope.
