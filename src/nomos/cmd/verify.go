package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/verify"
	"github.com/spf13/cobra"
)

var (
	verifyType string
	listJSON   bool
	verifyCmd  = &cobra.Command{
		Use:   "verify",
		Short: "Run definition of done verification checks",
		Long:  `Run Go-native concurrent Definition of Done (DoD) verification checks including tests, format/lint validation, security audit, and spec-to-code parity.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			repoRoot := findRepoRoot(root)
			if err := enforceWorktreeZone(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), "verify"); err != nil {
				return err
			}

			if listJSON {
				out, err := json.MarshalIndent(verify.DoDStages, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(out))
				return nil
			}

			if verifyType == "security" {
				findings, err := verify.ScanSecurity(root)
				if err != nil {
					return err
				}
				criticalOrHigh := 0
				for _, f := range findings {
					synapse.Info("- [%s] %s:%d: %s\n", f.Severity, f.File, f.Line, f.Message)
					if f.Severity == "CRITICAL" || f.Severity == "HIGH" {
						criticalOrHigh++
					}
				}
				if criticalOrHigh > 0 {
					return fmt.Errorf("security audit failed with %d critical/high finding(s)", criticalOrHigh)
				}
				synapse.Info("Security scan passed with %d low/medium findings.\n", len(findings))
				return nil
			}

			if verifyType == "parity" {
				taskId := verify.GetActiveTaskId(root)
				if taskId == "" {
					return fmt.Errorf("active task ID could not be detected")
				}
				drift, parity, err := verify.CheckSpecParity(root, taskId)
				if err != nil {
					return err
				}
				if drift >= 30.0 {
					return fmt.Errorf("spec parity drift is too high (drift: %.1f%%, parity: %.1f%%)", drift, parity)
				}
				return nil
			}

			if verifyType == "dor" {
				if err := verify.VerifyDoR(root); err != nil {
					return err
				}
				synapse.Info("%s", fmt.Sprint("Definition of Ready (DoR) check passed successfully."))
				return nil
			}

			if verifyType == "drift" {
				if err := verify.CheckSSOTDrift(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(root); return c }()); err != nil {
					return err
				}
				synapse.Info("%s", fmt.Sprint("SSOT Drift check passed successfully."))
				return nil
			}

			return verify.VerifyDoD(root)
		},
	}
)

func init() {
	verifyCmd.Flags().StringVarP(&verifyType, "type", "t", "dod", "Type of verification: dod, security, parity, dor, drift")
	verifyCmd.Flags().BoolVar(&listJSON, "list-json", false, "Output DoD Verification Stages as JSON")
}
