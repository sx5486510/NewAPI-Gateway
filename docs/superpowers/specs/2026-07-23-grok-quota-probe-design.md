# Grok Quota Probe Design

## Goal

Add a small read-only Node.js command-line script that reads one Grok CLI
credential JSON file and reports the account plan, quota, usage, remaining
capacity, reset times, and the HTTP status of each upstream source.

## Inputs and execution

The script is invoked with a credential path:

```powershell
node scripts/probe-grok-quota.mjs C:\path\to\xai-account.json
```

The credential must contain an access token under a recognized token field.
The user ID is read from a recognized subject/user field and falls back to the
JWT `sub` claim. Tokens and complete credential contents are never printed.

## Upstream requests

The script sends three independent GET requests to the Grok CLI upstream:

1. `/v1/billing?format=credits` for weekly credit usage and product breakdown.
2. `/v1/billing` for monetary monthly and on-demand fields.
3. `/v1/user?include=subscription` for the live subscription tier.

Requests use the Grok CLI bearer token and client headers, including
`x-userid` when available. Responses are bounded and parsed as JSON. The two
billing responses are merged because neither response shape is assumed to
contain every quota field.

## Output

Human-readable output includes:

- plan name, with JWT tier as the final fallback;
- weekly used and remaining percentages and reset time;
- per-product used and remaining percentages when supplied;
- monthly used, limit, and remaining amounts;
- on-demand used, cap, and remaining amounts;
- each source's HTTP status.

Unknown values are printed as `n/a`; absence is not converted to zero. Monetary
values are normalized from the upstream cent wrapper or numeric representation
and displayed as USD.

## Errors and safety

The script exits nonzero for an unreadable or invalid credential, a missing
access token, or when both billing endpoints fail or yield no usable quota.
Partial responses remain useful: a failed subscription request does not discard
billing data, and one successful billing endpoint can supply the report. HTTP
401/403 errors explicitly identify an expired or unauthorized credential. No
credential is modified and automatic OAuth refresh is outside this probe's
scope.

## Testing

Offline Node tests cover credential field extraction, JWT tier fallback,
credits/monthly response merging, money normalization, remaining calculations,
and partial/invalid responses. After those tests pass, the script is run once
against the discovered local Grok credential to validate the real upstream
contract without exposing secrets.
