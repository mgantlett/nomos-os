{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  buildInputs = with pkgs; [
    shellcheck
    sqlite
    jq
    git
    curl
    nodejs
    bc
    parallel
    go
    gopls
    pandoc
    typst
    liberation_ttf
    python3
    google-cloud-sdk
    aider-chat
    typescript
    pm2
    psmisc
    opencode
    python3Packages.pymupdf
    ripgrep
  ];

  shellHook = ''
    export TYPST_FONT_PATHS="${pkgs.liberation_ttf}/share/fonts/truetype"
    echo "⚡ Welcome to the Nomos development shell! ⚡"
    echo "Loaded dependencies: shellcheck, sqlite, jq, git, curl, nodejs, bc, parallel, google-cloud-sdk, aider-chat, datasette, pm2, psmisc, opencode"

    # Dynamically compile the local nomos engine if it's missing or stale, provided we are in a fully hydrated worktree
    if [ -f src/nomos/main.go ]; then
      if [ ! -f bin/nomos ] || [ src/nomos/main.go -nt bin/nomos ]; then
        echo "🔨 Compiling contextual nomos binary..."
        go build -o bin/nomos ./src/nomos/main.go || true
        
        COMMON_DIR=$(git rev-parse --git-common-dir 2>/dev/null)
        if [ -n "$COMMON_DIR" ]; then
          if [[ "$COMMON_DIR" == /* ]]; then
            ROOT_DIR=$(dirname "$COMMON_DIR")
          else
            ROOT_DIR=$(cd $(dirname "$COMMON_DIR") && pwd)
          fi
          
          SOV_BIN="$ROOT_DIR/../../private/nomos-sovereign/bin/nomos"
          
          if [ -d "$ROOT_DIR/../../private/nomos-sovereign/bin" ]; then
             echo "🔄 Re-compiling contextual binary for nomos-sovereign edition natively..."
             go build -o "$SOV_BIN" ./src/nomos/main.go || true
          fi
        fi
      fi
    else
      echo "🐚 Running in Hollow Shell mode (src/ hidden). Binary compilation skipped."
    fi
  '';
}
