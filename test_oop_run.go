package main

import (
"fmt"
"os"

"github.com/mgantlett/nomos-os/src/nomos/modules/verify"
)

func main() {
	cwd, _ := os.Getwd()
	fmt.Printf("Analyzing: %s\n", cwd)
	findings, err := verify.AnalyzeASTInterfaces(cwd, false)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Found %d OOP violations.\n", len(findings))
	for _, f := range findings {
		fmt.Printf("- %s:%d %s\n", f.File, f.Line, f.Message)
	}
}
