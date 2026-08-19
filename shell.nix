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
    if [ -f worktrees/.explorer/src/nomos/main.go ]; then
      if [ ! -f bin/nomos ] || [ worktrees/.explorer/src/nomos/main.go -nt bin/nomos ]; then
        echo "🔨 Compiling contextual nomos binary..."
        (
          cd worktrees/.explorer
          mkdir -p ../../bin
          cp ../../go.mod ../../go.sum .
          cp ../../../nomos-commons/go.mod ../../../nomos-commons/go.sum ../../../nomos-commons/worktrees/.explorer/ 2>/dev/null || true
          cat << 'EOF' > go.work
go 1.26.5
use .
replace github.com/mgantlett/nomos-commons => ../../../nomos-commons/worktrees/.explorer
EOF
          go build -o ../../bin/nomos ./src/nomos/main.go || true
          rm -f go.mod go.sum go.work go.work.sum
          rm -f ../../../nomos-commons/worktrees/.explorer/go.mod ../../../nomos-commons/worktrees/.explorer/go.sum
        ) || true
      fi
    else
      echo "🐚 Running in Hollow Shell mode (.explorer missing). Binary compilation skipped."
    fi
  '';
}
