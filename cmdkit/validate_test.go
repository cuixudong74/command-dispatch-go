package cmdkit

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func validationTree() *Command {
	root := &Command{Name: "app"}
	cmd := &Command{
		Name: "deploy",
		Flags: []Flag{
			{Name: "env", Required: true, Choices: []string{"dev", "staging", "prod"}},
			{Name: "token", Required: true},
			{Name: "json", MutexGroup: "format"},
			{Name: "yaml", MutexGroup: "format"},
			{Name: "user", OneOfGroup: "auth"},
			{Name: "password", OneOfGroup: "auth"},
			{Name: "replicas", Validator: func(v string) error {
				if v == "0" {
					return fmt.Errorf("must be positive")
				}
				return nil
			}},
		},
		Positionals: []Positional{{Name: "file", Min: 1, Max: 2}},
	}
	root.AddCommand(cmd)
	return root
}

func baseArgs() []string {
	return []string{"deploy", "--env", "dev", "--token", "t", "--user", "u", "f1"}
}

func TestRequiredMissing(t *testing.T) {
	p := NewParser()
	_, err := p.Parse(validationTree(), ParseOptions{Args: []string{"deploy", "--token", "t", "--user", "u", "f1"}, Env: map[string]string{}})
	var ve *Error
	if !errors.As(err, &ve) || ve.Kind != KindValidation {
		t.Fatalf("want validation error, got %v", err)
	}
	if !strings.Contains(err.Error(), "--env") {
		t.Fatalf("error should mention --env, got: %v", err)
	}
}

func TestMutexGroupConflict(t *testing.T) {
	p := NewParser()
	args := append(baseArgs(), "--json", "1", "--yaml", "2")
	_, err := p.Parse(validationTree(), ParseOptions{Args: args, Env: map[string]string{}})
	var ve *Error
	if !errors.As(err, &ve) || ve.Kind != KindValidation {
		t.Fatalf("want validation error, got %v", err)
	}
	if !strings.Contains(err.Error(), "--json") || !strings.Contains(err.Error(), "--yaml") {
		t.Fatalf("error should mention both flags, got: %v", err)
	}
}

func TestOneOfGroupMissing(t *testing.T) {
	p := NewParser()
	args := []string{"deploy", "--env", "dev", "--token", "t", "f1"}
	_, err := p.Parse(validationTree(), ParseOptions{Args: args, Env: map[string]string{}})
	var ve *Error
	if !errors.As(err, &ve) || ve.Kind != KindValidation {
		t.Fatalf("want validation error, got %v", err)
	}
	if !strings.Contains(err.Error(), "at least one of") {
		t.Fatalf("got: %v", err)
	}
}

func TestChoicesConstraint(t *testing.T) {
	p := NewParser()
	args := []string{"deploy", "--env", "qa", "--token", "t", "--user", "u", "f1"}
	_, err := p.Parse(validationTree(), ParseOptions{Args: args, Env: map[string]string{}})
	if err == nil || !strings.Contains(err.Error(), "allowed: dev, staging, prod") {
		t.Fatalf("want choices error, got: %v", err)
	}
}

func TestCustomValidator(t *testing.T) {
	p := NewParser()
	args := append(baseArgs(), "--replicas", "0")
	_, err := p.Parse(validationTree(), ParseOptions{Args: args, Env: map[string]string{}})
	if err == nil || !strings.Contains(err.Error(), "--replicas") || !strings.Contains(err.Error(), "must be positive") {
		t.Fatalf("want custom validator error, got: %v", err)
	}
}

func TestPositionalCount(t *testing.T) {
	p := NewParser()
	// 少于 Min
	args := []string{"deploy", "--env", "dev", "--token", "t", "--user", "u"}
	if _, err := p.Parse(validationTree(), ParseOptions{Args: args, Env: map[string]string{}}); err == nil {
		t.Fatal("want min count error")
	}
	// 多于 Max
	args = append(baseArgs(), "f2", "f3")
	if _, err := p.Parse(validationTree(), ParseOptions{Args: args, Env: map[string]string{}}); err == nil {
		t.Fatal("want max count error")
	}
	// 合法范围
	ctx, err := p.Parse(validationTree(), ParseOptions{Args: append(baseArgs(), "f2"), Env: map[string]string{}})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got := ctx.Args(); len(got) != 2 || got[0] != "f1" || got[1] != "f2" {
		t.Fatalf("got args %v", got)
	}
}

func TestNoPartialBinding(t *testing.T) {
	p := NewParser()
	ran := false
	root := validationTree()
	root.Children()[0].Run = func(ctx *Context) error { ran = true; return nil }
	// 校验失败时：ctx 必须为 nil 且 Run 不得执行。
	ctx, err := p.Parse(root, ParseOptions{Args: []string{"deploy", "--env", "bad", "--token", "t", "--user", "u", "f1"}, Env: map[string]string{}})
	if err == nil {
		t.Fatal("want error")
	}
	if ctx != nil {
		t.Fatal("context must be nil on validation failure")
	}
	if ran {
		t.Fatal("Run must not execute on validation failure")
	}
}

func TestBusinessErrorKind(t *testing.T) {
	p := NewParser()
	root := &Command{Name: "app"}
	root.AddCommand(&Command{Name: "boom", Run: func(ctx *Context) error {
		return fmt.Errorf("kaput")
	}})
	_, err := p.Parse(root, ParseOptions{Args: []string{"boom"}, Env: map[string]string{}})
	var be *Error
	if !errors.As(err, &be) || be.Kind != KindBusiness {
		t.Fatalf("want business error, got %v", err)
	}
	if be.ExitCode != p.ExitCodes.Business {
		t.Fatalf("exit code %d, want %d", be.ExitCode, p.ExitCodes.Business)
	}
}

func TestConfigurableExitCodes(t *testing.T) {
	p := &Parser{ExitCodes: ExitCodes{Parse: 10, Validation: 20, Business: 30}}
	root := validationTree()

	_, err := p.Parse(root, ParseOptions{Args: []string{"nope-cmd"}, Env: map[string]string{}})
	var e *Error
	if !errors.As(err, &e) || e.ExitCode != 10 {
		t.Fatalf("parse exit code: %v", err)
	}
	_, err = p.Parse(root, ParseOptions{Args: []string{"deploy"}, Env: map[string]string{}})
	if !errors.As(err, &e) || e.ExitCode != 20 {
		t.Fatalf("validation exit code: %v", err)
	}
	// 相同输入退出码稳定
	_, err2 := p.Parse(root, ParseOptions{Args: []string{"deploy"}, Env: map[string]string{}})
	var e2 *Error
	if !errors.As(err2, &e2) || e2.ExitCode != e.ExitCode {
		t.Fatalf("exit code unstable: %v vs %v", err, err2)
	}
}
