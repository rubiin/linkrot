package main

import "testing"

func TestRootCommandExists(t *testing.T) {
	if rootCmd.Use == "" {
		t.Error("root command has no Use string")
	}
}
