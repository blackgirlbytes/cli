# Dispatch Workflow Auth Refresh

If a Slack dispatch or release-summary workflow fails because its Entire auth
secret is missing, blank, or rejected, refresh the workflow auth secret using
the device-flow recipe captured in the checkpoint for this commit.

Do not store secret values, token outputs, shell transcripts, or copied command
results in this file.

Notes:

- `ENTIRE_AUTH_TOKEN` is the Entire auth secret used by dispatch generation.
- `ENTIRE_IO_PAT` is GitHub auth for cloning GitHub repositories.
- The dispatch auth secret may need periodic refresh.
- Paste the refreshed secret directly into GitHub Actions secrets.

Search keywords: dispatch workflow auth refresh, `ENTIRE_AUTH_TOKEN`,
`ENTIRE_IO_PAT`, release summary Slack, dispatch notes Slack, device flow,
`ent_` token.
