package cmdkit

import (
	"errors"
	"strings"
	"testing"
)

func newTestTree() *Command {
	root := &Command{Name: "app"}
	serve := &Command{
		Name:    "serve",
		Aliases: []string{"srv"},
		Usage:   "start server",
		Flags: []Flag{
			{Name: "port", Shorthand: "p", Default: "8080", EnvVars: []string{"APP_PORT"}, ConfigKey: "server.port"},
			{Name: "host", Default: "localhost", Persistent: true},
		},
	}
	_ = serve
	status := &Command{Name: "status", Usage: "show status"}
	stats := &Command{Name: "stats", Usage: "show stats"}
	root.AddCommand(serve, status, stats)
	return root
}

func TestUniquePrefixAndAlias(t *testing.T) {
	root := newTestTree()
	p := NewParser()

	// 唯一前缀 "ser" -> serve
	ctx, err := p.Parse(root, ParseOptions{Args: []string{"ser"}})
	if err != nil {
		t.Fatalf("prefix parse failed: %v", err)
	}
	if ctx.Command.Name != "serve" {
		t.Fatalf("got %q, want serve", ctx.Command.Name)
	}

	// 别名 srv -> serve
	ctx, err = p.Parse(root, ParseOptions{Args: []string{"srv"}})
	if err != nil {
		t.Fatalf("alias parse failed: %v", err)
	}
	if ctx.Command.Name != "serve" {
		t.Fatalf("got %q, want serve", ctx.Command.Name)
	}
}

func TestAmbiguousPrefix(t *testing.T) {
	root := newTestTree()
	p := NewParser()
	// "st" 同时是 status 与 stats 的前缀
	_, err := p.Parse(root, ParseOptions{Args: []string{"st"}})
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
	var pe *Error
	if !errors.As(err, &pe) || pe.Kind != KindParse {
		t.Fatalf("want parse error, got %v", err)
	}
	if !strings.Contains(err.Error(), "status") || !strings.Contains(err.Error(), "stats") {
		t.Fatalf("error should list all candidates, got: %v", err)
	}
}

func TestUnknownCommandSuggestion(t *testing.T) {
	root := newTestTree()
	p := NewParser()
	_, err := p.Parse(root, ParseOptions{Args: []string{"statuz"}})
	if err == nil || !strings.Contains(err.Error(), `did you mean "status"`) {
		t.Fatalf("want suggestion for status, got: %v", err)
	}
}

func TestSourcePriority(t *testing.T) {
	root := newTestTree()
	p := NewParser()

	cases := []struct {
		name       string
		opts       ParseOptions
		wantValue  string
		wantSource Source
	}{
		{"default only", ParseOptions{Args: []string{"serve"}, Env: map[string]string{}}, "8080", SourceDefault},
		{"config beats default", ParseOptions{Args: []string{"serve"}, Env: map[string]string{}, Config: map[string]string{"server.port": "1000"}}, "1000", SourceConfig},
		{"env beats config", ParseOptions{Args: []string{"serve"}, Env: map[string]string{"APP_PORT": "2000"}, Config: map[string]string{"server.port": "1000"}}, "2000", SourceEnv},
		{"flag beats all", ParseOptions{Args: []string{"serve", "--port", "3000"}, Env: map[string]string{"APP_PORT": "2000"}, Config: map[string]string{"server.port": "1000"}}, "3000", SourceFlag},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, err := p.Parse(root, tc.opts)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			v, src := ctx.Value("port")
			if v != tc.wantValue || src != tc.wantSource {
				t.Fatalf("got (%q, %s), want (%q, %s)", v, src, tc.wantValue, tc.wantSource)
			}
		})
	}
}

func TestFlagForms(t *testing.T) {
	root := newTestTree()
	p := NewParser()
	for _, args := range [][]string{
		{"serve", "--port", "9000"},
		{"serve", "--port=9000"},
		{"serve", "-p", "9000"},
		{"serve", "-p=9000"},
	} {
		ctx, err := p.Parse(root, ParseOptions{Args: args, Env: map[string]string{}})
		if err != nil {
			t.Fatalf("args %v: %v", args, err)
		}
		if v, _ := ctx.Value("port"); v != "9000" {
			t.Fatalf("args %v: got %q", args, v)
		}
	}
}

func TestUnknownFlag(t *testing.T) {
	root := newTestTree()
	p := NewParser()
	_, err := p.Parse(root, ParseOptions{Args: []string{"serve", "--nope", "1"}, Env: map[string]string{}})
	var pe *Error
	if !errors.As(err, &pe) || pe.Kind != KindParse {
		t.Fatalf("want parse error, got %v", err)
	}
}
