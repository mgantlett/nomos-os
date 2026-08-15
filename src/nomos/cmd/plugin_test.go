package cmd

import (
	"bytes"
	"testing"
)

func TestPluginSubcommandsRegistered(t *testing.T) {
	pluginCmdFound := false
	for _, c := range RootCmd.Commands() {
		if c.Name() == "plugin" {
			pluginCmdFound = true

			// Verify list and call are registered under plugin
			listFound := false
			callFound := false
			for _, sub := range c.Commands() {
				if sub.Name() == "list" {
					listFound = true
				}
				if sub.Name() == "call" {
					callFound = true
				}
			}

			if !listFound {
				t.Errorf("expected 'plugin list' subcommand to be registered")
			}
			if !callFound {
				t.Errorf("expected 'plugin call' subcommand to be registered")
			}
		}
	}

	if !pluginCmdFound {
		t.Errorf("expected 'plugin' subcommand to be registered on RootCmd")
	}
}

func TestPluginCallArgsValidation(t *testing.T) {
	// Call without args should fail
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)

	RootCmd.SetArgs([]string{"plugin", "call"})
	err := RootCmd.Execute()
	if err == nil {
		t.Errorf("expected error when running 'plugin call' without arguments")
	}
}
