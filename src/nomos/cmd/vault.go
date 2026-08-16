package cmd

import (
	"crypto/md5"
	"fmt"
	"os"
	osExec "os/exec"
	"path/filepath"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	execMod "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/spf13/cobra"
)

// vaultOutDir is the output directory to export vault definitions.
var vaultOutDir string
var vaultSource string
var vaultName string

// vaultCmd represents the root vault command for syncing documents to the tablet.
var vaultCmd = &cobra.Command{
	Use:   "vault",
	Short: "Manage and synchronize markdown documentation vaults with the reMarkable tablet",
	Long:  `Export local code-truth definitions or external Markdown vaults (like VitePress) to PDF and sync them to the reMarkable tablet.`,
}

// vaultSyncCmd triggers the synchronization of code-truth definitions or an external vault to the tablet.
var vaultSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Compile a Markdown vault to PDF and sync to reMarkable",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := os.Getwd()
		if err != nil {
			return err
		}

		if vaultSource != "" {
			// If a custom source (like the GSI Vault VitePress dir) is provided, compile it directly
			vaultOutDir = vaultSource
			synapse.Info("Using external vault source: %s\n", vaultOutDir)
		} else {
			// Otherwise, generate the dynamic Nomos code-truth vault
			if vaultOutDir == "" {
				vaultOutDir = workspace.MustNewContext(root).TmpDir()
				vaultOutDir = filepath.Join(vaultOutDir, "vault-export")
			}
			err = execMod.GenerateWiki(root, vaultOutDir)
			if err != nil {
				return fmt.Errorf("failed to generate vault: %w", err)
			}
			synapse.Info("Vault successfully generated at %s\n", vaultOutDir)
		}

		if vaultName == "" {
			vaultName = "Nomos Vault"
		}

		synapse.Info("📄 Compiling Vault to PDF...\n")

		// Run the pandoc compilation in nix-shell, excluding node_modules to avoid crashing on npm dependency READMEs
		pandocScript := fmt.Sprintf("find %s -type d -name \"node_modules\" -prune -o -name \"*.md\" -print0 | sort -z | xargs -0 pandoc -o /tmp/Nomos-Vault.pdf --toc --toc-depth=2 --metadata title=\"%s\" --metadata author=\"Sophia Labs\" -V mainfont=\"Liberation Serif\" --pdf-engine=typst --highlight-style=tango", vaultOutDir, vaultName)
		compileCmd := osExec.Command("nix-shell", "--run", pandocScript)
		compileCmd.Stdout = os.Stdout
		compileCmd.Stderr = os.Stderr
		if err := compileCmd.Run(); err != nil {
			return fmt.Errorf("failed to compile PDF: %w", err)
		}

		// Generate deterministic UUID from vaultName so we don't overwrite other vaults
		hash := md5.Sum([]byte(vaultName))
		uuid := fmt.Sprintf("%x-%x-%x-%x-%x", hash[0:4], hash[4:6], hash[6:8], hash[8:10], hash[10:16])

		currentTime := fmt.Sprintf("%d000", time.Now().Unix())

		metadata := fmt.Sprintf(`{
    "deleted": false,
    "lastModified": "%s",
    "metadatamodified": false,
    "modified": false,
    "parent": "",
    "pinned": false,
    "synced": false,
    "type": "DocumentType",
    "version": 1,
    "visibleName": "%s"
}`, currentTime, vaultName)

		content := `{
    "extraMetadata": {},
    "fileType": "pdf",
    "fontName": "",
    "lastOpenedPage": 0,
    "lineHeight": -1,
    "margins": 100,
    "pageCount": 1,
    "textScale": 1,
    "transform": {}
}`

		os.WriteFile(fmt.Sprintf("/tmp/%s.metadata", uuid), []byte(metadata), 0644)
		os.WriteFile(fmt.Sprintf("/tmp/%s.content", uuid), []byte(content), 0644)

		// Copy PDF to UUID name
		copyCmd := osExec.Command("cp", "/tmp/Nomos-Vault.pdf", fmt.Sprintf("/tmp/%s.pdf", uuid))
		copyCmd.Run()

		synapse.Info("🚀 Syncing PDF to Remarkable Paper Pro (192.168.1.54:2222)...\n")

		// First, check if the vault already exists on the tablet
		checkCmd := osExec.Command("ssh", "-p", "2222", "root@192.168.1.54", fmt.Sprintf("test -f /home/root/.local/share/remarkable/xochitl/%s.content", uuid))
		exists := checkCmd.Run() == nil

		var scpCmd *osExec.Cmd
		if exists {
			synapse.Info("📌 Existing Vault found! Updating PDF while preserving your handwritten annotations...\n")
			// Only SCP the PDF file to preserve the `.content` page UUID mappings
			scpCmd = osExec.Command("scp", "-P", "2222",
				fmt.Sprintf("/tmp/%s.pdf", uuid),
				"root@192.168.1.54:/home/root/.local/share/remarkable/xochitl/")
		} else {
			synapse.Info("🆕 First time sync! Initializing Vault metadata...\n")
			// SCP the PDF, metadata, and content files
			scpCmd = osExec.Command("scp", "-P", "2222",
				fmt.Sprintf("/tmp/%s.pdf", uuid),
				fmt.Sprintf("/tmp/%s.metadata", uuid),
				fmt.Sprintf("/tmp/%s.content", uuid),
				"root@192.168.1.54:/home/root/.local/share/remarkable/xochitl/")
		}

		scpCmd.Stdout = os.Stdout
		scpCmd.Stderr = os.Stderr
		if err := scpCmd.Run(); err != nil {
			synapse.Info("⚠️  Warning: failed to SCP to remarkable: %v\n", err)
		} else {
			synapse.Info("🔄 Restarting Xochitl to render new file...\n")
			sshCmd := osExec.Command("ssh", "-p", "2222", "root@192.168.1.54", "systemctl restart xochitl")
			sshCmd.Run()
			synapse.Info("✅ Sync Complete! Your Nomos Vault is ready for the beach. 🏖️\n")
		}

		return nil
	},
}

// init registers the vault commands and their flags with the root command.
func init() {
	vaultSyncCmd.Flags().StringVarP(&vaultSource, "source", "s", "", "Path to the markdown directory to compile (e.g. VitePress docs)")
	vaultSyncCmd.Flags().StringVarP(&vaultName, "name", "n", "Nomos Vault", "The display name for the tablet (e.g. 'GSI Vault')")

	vaultCmd.AddCommand(vaultSyncCmd)
	RootCmd.AddCommand(vaultCmd)
}
