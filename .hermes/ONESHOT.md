# Hermes remote bootstrap prompt

Use this when Hermes is already installed but this repository has not been
downloaded. Copy the complete prompt below into Hermes as one message.

This starts the full guided installation. Hermes owns every terminal command.
The user interacts only with browser consent, project choices, administrator
approval, or other UI that requires human authority. After the user completes
a checkpoint, Hermes resumes the same task instead of restarting the
installation.

```text
Install and configure Google Search Console MCP for me.

Use this as the only repository source:
https://github.com/geniushub-seo/gsc-mcp

Starting condition:
The repository may not exist on this computer yet. Continue until Hermes has a
working gsc MCP server, all six tools are discovered, and real read-only GSC
access is verified with list_sites.

Execution rules:

1. Inspect the operating system and check for git, Hermes, gcloud, and any
   existing gsc-mcp installation.
2. If the repository is absent, clone it into a new gsc-mcp directory under
   the current working directory.
3. If a gsc-mcp directory already exists, verify that its git remote matches
   the repository URL above. Never delete, overwrite, reset, or reuse an
   unrelated directory.
4. Enter the repository, read the root AGENTS.md completely, and follow its
   instructions. Read only the additional files that AGENTS.md directs you to.
   Prefer the repository's existing install.sh and .hermes/setup.sh; do not
   invent or duplicate an installation flow.
5. You own every shell and PowerShell operation. Never tell me to open
   Terminal, copy a command, paste a command, or return terminal output. If the
   official Google Cloud CLI is missing, install it using the
   operating-system-specific procedure in the repository instructions. Ask
   only for approval or a password entry when the OS requires human authority.
6. Never read, print, paste, log, or expose ADC JSON, OAuth tokens,
   service-account keys, or other credential contents. You may confirm that a
   credential file exists without displaying it.
7. For browser-based OAuth, first tell me that a Google page will open and that
   my only task is to choose an account and approve access. Then execute
   gcloud auth application-default login yourself through your terminal tool,
   keep the process running, and wait for the browser consent to complete. Do
   not print the command as an action for me. If the browser does not open,
   first use available browser or operating-system open capabilities; never
   default to sending me to Terminal.
8. After OAuth succeeds, verify only that ADC exists without reading it, then
   continue automatically. Do not wait for me to copy terminal success text.
9. Obtain a real GCP project ID by running gcloud auth login and then
   gcloud projects list yourself — do not ask me for an ID before you have run
   that command, and never execute a command containing the literal
   placeholder PROJECT_ID. Set it as the ADC quota project, then enable the API
   on that same project with
   gcloud services enable searchconsole.googleapis.com --project=PROJECT_ID.
   A newly created project never has it enabled, and skipping this returns a
   403 whose details contain "reason": "SERVICE_DISABLED". If the list is empty
   I have no project: send me to the project creation page. If it has several,
   ask me to choose from the ids you read.
10. After I complete a human checkpoint, resume from that checkpoint. Do not
   repeat steps that already passed their verification.
11. Perform read-only GSC verification only. Do not submit or delete sitemaps
    and do not modify Search Console data.
12. Continue through any issue you can safely diagnose and repair. Do not stop
    after merely printing general instructions.

Completion evidence required:

- The gsc-mcp binary is installed and executable.
- gcloud and ADC are ready, and the ADC quota project is set.
- gsc-mcp doctor reports list_sites OK and a visible property count.
- hermes mcp list reports gsc as enabled.
- hermes mcp test gsc reports Connected and Tools discovered: 6.
- The final response tells me to start a new Hermes session so it loads the
  newly added MCP server.

If human action is required, respond only in this format:

[WAITING_FOR_USER]
Stage:
Why execution stopped:
The one action I must take:
What successful completion looks like:
What I should reply when done:

The requested user action must be a browser/UI click, a choice, a value, or an
approval. It must never be a shell or PowerShell command.

When every completion check passes, report only the evidence listed above. Do
not print property names or credential contents.
```

After installation succeeds, start a new Hermes session in the cloned
repository and ask it to call `list_sites`.
