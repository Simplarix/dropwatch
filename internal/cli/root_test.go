package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitAndDomainListJSON(t *testing.T) {
	t.Parallel()
	temp := t.TempDir()
	database := filepath.Join(temp, "dropwatch.db")
	configPath := filepath.Join(temp, "missing.yaml")
	initCommand := NewRootCommand()
	initOutput := new(bytes.Buffer)
	initCommand.SetOut(initOutput)
	initCommand.SetArgs([]string{"--database", database, "--config", configPath, "init", "--json"})
	if err := initCommand.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(initOutput.String(), database) {
		t.Fatalf("init output = %s", initOutput.String())
	}
	addCommand := NewRootCommand()
	addCommand.SetOut(new(bytes.Buffer))
	addCommand.SetArgs([]string{"--database", database, "--config", configPath, "domain", "add", "Example.COM", "--no-check"})
	if err := addCommand.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	listCommand := NewRootCommand()
	listOutput := new(bytes.Buffer)
	listCommand.SetOut(listOutput)
	listCommand.SetArgs([]string{"--database", database, "--config", configPath, "domain", "list", "--json"})
	if err := listCommand.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listOutput.String(), `"ascii_domain": "example.com"`) {
		t.Fatalf("list output = %s", listOutput.String())
	}
}
