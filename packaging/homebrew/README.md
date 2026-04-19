# Homebrew tap for `giv`

This folder mirrors what goes into a **separate Git tap repository** (Homebrew does not install from a subfolder of the app repo).

## Why two repos?

- **`cjp2600/giv`** — application source and releases.
- **`cjp2600/homebrew-giv`** (example name) — only `Formula/giv.rb`. Users run `brew tap cjp2600/giv`.

## Quick publish (personal tap)

1. On GitHub, create a **new public repository** named **`homebrew-giv`** (the `homebrew-` prefix lets people run `brew tap username/giv`).
2. Add **`Formula/giv.rb`** in that repo — copy from [`Formula/giv.rb`](./Formula/giv.rb) in this directory.
3. Commit and push **`main`** (any default branch is fine).
4. Users install with:
   ```bash
   brew tap cjp2600/giv
   brew install giv
   ```

## Update after each release

1. Bump **`url`** to `.../archive/refs/tags/vNEW.tar.gz`.
2. Set **`sha256`**:  
   `curl -sL "https://github.com/cjp2600/giv/archive/refs/tags/vNEW.tar.gz" | shasum -a 256`
3. Optionally set explicit **`version "NEW"`** if Homebrew mis-detects the version.

## Optional: upstream to Homebrew/core

Requirements are stricter ([acceptance criteria](https://docs.brew.sh/Acceptable-Formulae)): notability, tests, `brew audit`, etc. Many small CLIs stay on a personal tap first.
