package cmdkit

import (
	"strings"
	"testing"
)

func helpTree() *Command {
	root := &Command{Name: "app", Description: "demo app"}
	serve := &Command{
		Name:    "serve",
		Aliases: []string{"srv"},
		Usage:   "start server",
		Flags: []Flag{
			{Name: "port", Shorthand: "p", Default: "8080", Usage: "listen port", EnvVars: []string{"APP_PORT"}},
			{Name: "mode", Choices: []string{"fast", "safe"}, Usage: "run mode"},
		},
	}
	root.AddCommand(serve, &Command{Name: "stop", Usage: "stop server"})
	return root
}

func TestHelpContent(t *testing.T) {
	root := helpTree()
	h := Help(root)
	for _, want := range []string{"Usage: app <command>", "serve (srv)", "stop", "start server"} {
		if !strings.Contains(h, want) {
			t.Errorf("root help missing %q:\n%s", want, h)
		}
	}

	sh := Help(root.Children()[0])
	for _, want := range []string{"Usage: app serve [flags]", "Aliases: srv", `--port`, `default "8080"`, "one of: fast, safe", "env: APP_PORT"} {
		if !strings.Contains(sh, want) {
			t.Errorf("serve help missing %q:\n%s", want, sh)
		}
	}
}

func TestHelpFlag(t *testing.T) {
	p := NewParser()
	ctx, err := p.Parse(helpTree(), ParseOptions{Args: []string{"serve", "--help"}, Env: map[string]string{}})
	if err != ErrHelp {
		t.Fatalf("want ErrHelp, got %v", err)
	}
	if ctx.Command.Name != "serve" {
		t.Fatalf("want serve, got %q", ctx.Command.Name)
	}
}

func TestCompletionScripts(t *testing.T) {
	root := helpTree()
	bash := BashCompletion(root, "app")
	zsh := ZshCompletion(root, "app")
	for _, script := range []string{bash, zsh} {
		for _, want := range []string{"serve", "srv", "stop", "--port", "--mode", "fast safe"} {
			if !strings.Contains(script, want) {
				t.Errorf("completion script missing %q", want)
			}
		}
	}
	if !strings.Contains(bash, "complete -F") {
		t.Error("bash script missing complete builtin")
	}
	if !strings.Contains(zsh, "#compdef app") {
		t.Error("zsh script missing compdef header")
	}
}
