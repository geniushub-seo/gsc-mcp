# Hermes quick start

Hermes stores active MCP configuration in the profile reported by
`hermes config path`; it does not automatically load MCP settings from this
repository directory. This `.hermes/` folder is a safe onboarding bundle: it
contains no credentials and uses the Hermes CLI to merge `gsc` into the active
profile without replacing unrelated settings.

Choose the entry point that matches your situation:

- **Repository already cloned:** run `bash .hermes/setup.sh` as described
  below.
- **Repository not downloaded:** copy the remote bootstrap prompt from
  [ONESHOT.md](ONESHOT.md) into Hermes. Hermes will clone the repository and
  follow its agent instructions.

From the repository root, run:

```bash
bash .hermes/setup.sh
```

The script:

1. Finds Hermes and the installed `gsc-mcp` binary. If the binary is missing,
   it reuses the repository's existing `install.sh`; it does not duplicate the
   installer.
2. Runs `gsc-mcp doctor` and requires a real, non-empty `list_sites` result.
3. Uses `hermes mcp add` only when `gsc` is not already configured and
   automatically enables all six discovered tools, including in a non-TTY
   agent session.
4. Runs `hermes mcp test gsc` and requires all six tools to be discovered.
5. Tells you to start a new Hermes session before calling a tool.

If `doctor` asks for ADC login during an agent-assisted installation, Hermes
must run the gcloud command through its own terminal tool and keep it running.
The user only chooses an account and approves access in Google's browser page;
Hermes then sets the quota project and reruns the same script. It must never
send a novice user to Terminal. The second run resumes from completed
prerequisites and does not duplicate the MCP entry.

If `gsc-mcp` was installed somewhere else, provide its absolute path:

```bash
GSC_MCP_BIN=/absolute/path/to/gsc-mcp bash .hermes/setup.sh
```

After setup succeeds, start a new Hermes session from this repository and ask:

> Use gsc `list_sites` to list my Search Console properties.

Manual verification:

```bash
hermes config path
hermes mcp list
hermes mcp test gsc
```

If an existing `gsc` entry points to the wrong binary, the script stops without
overwriting it. Reconfigure explicitly:

```bash
hermes mcp remove gsc
hermes mcp add gsc --command /absolute/path/to/gsc-mcp --connect-timeout 30
hermes mcp test gsc
```

Never copy ADC JSON, OAuth tokens, or service-account keys into this folder.
