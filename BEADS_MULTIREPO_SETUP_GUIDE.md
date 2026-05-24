# Beads Multi-Repo Setup Guide

This guide explains how to configure your local environment to use the centralized beads issue tracker when working on muster.

## Overview

All beads databases live in `~/repos/beads-central/` — not inside any project repo. The muster project uses the prefix `muster-` (e.g., `muster-a1b2c3`). Your shell automatically routes `bd` commands to the right database based on which directory you are in.

```
~/repos/
├── beads-central/              ← clone this; all databases live here
│   ├── .beads/                 ← other projects (mp- prefix)
│   └── muster/
│       └── .beads/             ← muster database (muster- prefix)
└── muster/                     ← this repo; .beads/ contains only config.yaml
```

---

## Prerequisites

- **bd CLI v1.0.0+** — install via Homebrew:
  ```bash
  brew install gastownhall/tap/bd
  ```
- **beads-central** — ask the project lead for the repo URL and clone it:
  ```bash
  git clone <beads-central-url> ~/repos/beads-central
  ```

---

## Setup

### 1. Add the shell routing hook to `~/.zshrc`

```zsh
# Beads: per-project database routing (switches BEADS_DIR based on cwd)
function _beads_set_dir() {
    case "$PWD" in
        "$HOME/repos/muster"*)
            export BEADS_DIR="$HOME/repos/beads-central/muster/.beads"
            ;;
        *)
            export BEADS_DIR="$HOME/repos/beads-central/.beads"
            ;;
    esac
}
chpwd_functions+=(_beads_set_dir)
_beads_set_dir  # apply at shell startup based on initial cwd
```

Do **not** use a static `export BEADS_DIR=...` — the hook must be dynamic so it switches when you `cd`.

### 2. Activate

```bash
source ~/.zshrc
```

### 3. Verify

```bash
cd ~/repos/muster
bd stats        # should show muster- prefix
bd ready        # shows work available to claim
```

---

## Daily usage

```bash
bd ready                          # find available work
bd show <id>                      # view issue details
bd update <id> --claim            # claim and start work
bd create --title="..." --type=task --priority=2  # file a new issue
bd close <id> --reason="..."      # mark work done
```

Run `bd prime` for full command reference and session close protocol.

---

## Troubleshooting

**`bd` shows wrong issues or wrong prefix**
Run `echo $BEADS_DIR`. If it's not pointing to `~/repos/beads-central/muster/.beads`, re-run `source ~/.zshrc` and make sure the hook block above is present.

**`bd stats` shows 0 issues**
Run `bd dolt status` to confirm the active data directory. If it's wrong, check that `~/repos/beads-central/muster/.beads/` exists and was not accidentally deleted.

**New terminal doesn't auto-switch `BEADS_DIR`**
The `_beads_set_dir` call at the end of the hook block handles shell startup. Confirm it is present and not commented out.
