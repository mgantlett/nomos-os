{ pkgs ? import (fetchTarball "https://github.com/NixOS/nixpkgs/archive/ad6fe71504ff652bd8b52839de83575d15a02c29.tar.gz") {} }:

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
    typescript
    pm2
    psmisc
    opencode
    python3Packages.pymupdf
    ripgrep
    cudaPackages.cudatoolkit
  ];

  shellHook = ''
    export TYPST_FONT_PATHS="${pkgs.liberation_ttf}/share/fonts/truetype"
    export PATH="$PWD/bin:/home/markg/Projects/sophialabs/private/nomos-sovereign/bin:$PATH"
    echo "⚡ Welcome to the Nomos development shell! ⚡"
    echo "Loaded dependencies: shellcheck, sqlite, jq, git, curl, nodejs, bc, parallel, google-cloud-sdk, datasette, pm2, psmisc"

    # Dynamically compile the local nomos engine if it's missing or stale, provided we are in a fully hydrated worktree
    if [ -f src/nomos/main.go ]; then
      if [ ! -f bin/nomos ] || [ src/nomos/main.go -nt bin/nomos ]; then
        echo "🔨 Compiling contextual nomos binary..."
        go build -o bin/nomos ./src/nomos/main.go || true
        

      fi
    else
      echo "🐚 Running in Hollow Shell mode (src/ hidden). Binary compilation skipped."
    fi
  '';
}
