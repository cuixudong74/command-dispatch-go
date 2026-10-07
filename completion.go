package command

import (
	"fmt"
	"sort"
	"strings"
)

// Shell 补全脚本目标环境。
type Shell string

const (
	Bash Shell = "bash"
	Zsh  Shell = "zsh"
)

// CompletionScript 生成补全脚本，覆盖子命令、参数名与枚举取值。
// 内容完全由声明的命令树推导，与 Help 保持一致。
func (c *Command) CompletionScript(shell Shell) (string, error) {
	switch shell {
	case Bash:
		return c.bashCompletion(), nil
	case Zsh:
		return c.zshCompletion(), nil
	default:
		return "", fmt.Errorf("不支持的 shell: %q（支持 bash、zsh）", shell)
	}
}

// subNames 返回子命令主名（排序，保证输出稳定）。
func (c *Command) subNames() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	names := make([]string, 0, len(c.children))
	for n := range c.children {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// flagNames 返回生效参数的全部 --长名（含别名）。
func (c *Command) flagNames() []string {
	var names []string
	for _, p := range c.effectiveParams() {
		if p.Positional || p.Variadic {
			continue
		}
		names = append(names, "--"+p.Name)
		for _, a := range p.Aliases {
			names = append(names, "--"+a)
		}
	}
	sort.Strings(names)
	return names
}

// enumParams 返回带枚举取值的参数（用于补全候选值）。
func (c *Command) enumParams() []*Param {
	var out []*Param
	for _, p := range c.effectiveParams() {
		if len(p.Enum) > 0 {
			out = append(out, p)
		}
	}
	return out
}

func (c *Command) bashCompletion() string {
	root := c.root()
	fn := "_" + strings.ReplaceAll(root.Name, "-", "_")
	var b strings.Builder
	fmt.Fprintf(&b, "# bash completion for %s（由声明的命令树自动生成）\n", root.Name)
	fmt.Fprintf(&b, "%s() {\n", fn)
	b.WriteString("  local cur prev words cword\n")
	b.WriteString("  _init_completion 2>/dev/null || { cur=${COMP_WORDS[COMP_CWORD]}; prev=${COMP_WORDS[COMP_CWORD-1]}; }\n")
	// 根据已输入的子命令序列定位当前命令节点。
	b.WriteString("  local node=\"\"\n  local i\n  for ((i=1; i<COMP_CWORD; i++)); do\n")
	b.WriteString("    case \"${words[i]}\" in\n")
	writeBashWalk(&b, root, "      ")
	b.WriteString("    esac\n  done\n")
	// 依据节点输出子命令与参数。
	b.WriteString("  case \"$node\" in\n")
	var walk func(cmd *Command, path string)
	walk = func(cmd *Command, path string) {
		subs := cmd.subNames()
		flags := cmd.flagNames()
		fmt.Fprintf(&b, "    %s)\n", shellQuote(path))
		if len(subs) > 0 || len(flags) > 0 {
			fmt.Fprintf(&b, "      COMPREPLY=( $(compgen -W \"%s\" -- \"$cur\") )\n",
				strings.Join(append(subs, flags...), " "))
		}
		b.WriteString("      ;;\n")
		cmd.mu.RLock()
		children := make([]*Command, 0, len(cmd.children))
		for _, s := range cmd.children {
			children = append(children, s)
		}
		cmd.mu.RUnlock()
		sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
		for _, s := range children {
			walk(s, path+"/"+s.Name)
		}
	}
	walk(root, "")
	// 枚举取值补全：prev 为带枚举的参数名时输出候选值。
	b.WriteString("  esac\n")
	b.WriteString("  case \"$prev\" in\n")
	var enumWalk func(cmd *Command)
	seen := map[string]bool{}
	enumWalk = func(cmd *Command) {
		for _, p := range cmd.enumParams() {
			key := p.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			fmt.Fprintf(&b, "    --%s)\n      COMPREPLY=( $(compgen -W \"%s\" -- \"$cur\") )\n      ;;\n",
				p.Name, strings.Join(p.Enum, " "))
		}
		cmd.mu.RLock()
		children := make([]*Command, 0, len(cmd.children))
		for _, s := range cmd.children {
			children = append(children, s)
		}
		cmd.mu.RUnlock()
		for _, s := range children {
			enumWalk(s)
		}
	}
	enumWalk(root)
	b.WriteString("  esac\n}\n")
	fmt.Fprintf(&b, "complete -F %s %s\n", fn, root.Name)
	return b.String()
}

// writeBashWalk 生成沿子命令路径更新 node 变量的 case 片段。
func writeBashWalk(b *strings.Builder, cmd *Command, indent string) {
	cmd.mu.RLock()
	children := make([]*Command, 0, len(cmd.children))
	for _, s := range cmd.children {
		children = append(children, s)
	}
	cmd.mu.RUnlock()
	sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
	for _, s := range children {
		names := append([]string{s.Name}, s.Aliases...)
		fmt.Fprintf(b, "%s%s) node=\"$node/%s\" ;;\n", indent, strings.Join(names, "|"), s.Name)
	}
}

func (c *Command) zshCompletion() string {
	root := c.root()
	var b strings.Builder
	fmt.Fprintf(&b, "#compdef %s\n# zsh completion for %s（由声明的命令树自动生成）\n\n", root.Name, root.Name)
	fmt.Fprintf(&b, "_%s() {\n", strings.ReplaceAll(root.Name, "-", "_"))
	b.WriteString("  local -a commands\n  local context state line\n  typeset -A opt_args\n")
	b.WriteString("  _arguments -C \\\n")
	for _, p := range root.effectiveParams() {
		if p.Positional || p.Variadic {
			continue
		}
		fmt.Fprintf(&b, "    '--%s[%s]%s' \\\n", p.Name, p.Usage, zshEnumAction(p))
	}
	b.WriteString("    '1: :->cmds' \\\n    '*:: :->args'\n")
	b.WriteString("  case $state in\n    cmds)\n")
	writeZshLevel(&b, root, "      ")
	b.WriteString("      ;;\n  esac\n}\n")
	fmt.Fprintf(&b, "_%s \"$@\"\n", strings.ReplaceAll(root.Name, "-", "_"))
	return b.String()
}

// writeZshLevel 递归输出每层子命令与参数。
func writeZshLevel(b *strings.Builder, cmd *Command, indent string) {
	subs := cmd.subNames()
	if len(subs) == 0 {
		fmt.Fprintf(b, "%s_arguments", indent)
		for _, p := range cmd.effectiveParams() {
			if p.Positional || p.Variadic {
				continue
			}
			fmt.Fprintf(b, " \\\n%s  '--%s[%s]%s'", indent, p.Name, p.Usage, zshEnumAction(p))
		}
		b.WriteString("\n")
		return
	}
	fmt.Fprintf(b, "%scommands=(\n", indent)
	cmd.mu.RLock()
	for _, n := range subs {
		s := cmd.children[n]
		alias := ""
		if len(s.Aliases) > 0 {
			alias = "（别名: " + strings.Join(s.Aliases, ",") + "）"
		}
		fmt.Fprintf(b, "%s  '%s:%s%s'\n", indent, n, s.Usage, alias)
	}
	cmd.mu.RUnlock()
	fmt.Fprintf(b, "%s)\n%s_describe '命令' commands\n", indent, indent)
	fmt.Fprintf(b, "%scase $words[1] in\n", indent)
	cmd.mu.RLock()
	children := make([]*Command, 0, len(cmd.children))
	for _, s := range cmd.children {
		children = append(children, s)
	}
	cmd.mu.RUnlock()
	sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
	for _, s := range children {
		names := append([]string{s.Name}, s.Aliases...)
		fmt.Fprintf(b, "%s  %s)\n", indent, strings.Join(names, "|"))
		writeZshLevel(b, s, indent+"    ")
		fmt.Fprintf(b, "%s    ;;\n", indent)
	}
	fmt.Fprintf(b, "%sesac\n", indent)
}

// zshEnumAction 为枚举参数生成 zsh 取值补全动作。
func zshEnumAction(p *Param) string {
	if len(p.Enum) == 0 {
		return ""
	}
	return ": :(" + strings.Join(p.Enum, " ") + ")"
}

func (c *Command) root() *Command {
	for c.parent != nil {
		c = c.parent
	}
	return c
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + s + "'"
}
