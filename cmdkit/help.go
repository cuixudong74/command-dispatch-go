package cmdkit

import (
	"fmt"
	"sort"
	"strings"
)

// Help 生成与声明的命令树一致的帮助信息（含别名、默认值、枚举与来源说明）。
func Help(cmd *Command) string {
	var sb strings.Builder

	// 用法行：包含完整命令路径。
	var path []string
	for c := cmd; c != nil; c = c.parent {
		path = append([]string{c.Name}, path...)
	}
	fmt.Fprintf(&sb, "Usage: %s", strings.Join(path, " "))
	if len(cmd.Children()) > 0 {
		sb.WriteString(" <command>")
	}
	if len(effectiveFlags(cmd)) > 0 {
		sb.WriteString(" [flags]")
	}
	for _, pos := range cmd.Positionals {
		fmt.Fprintf(&sb, " <%s...>", pos.Name)
	}
	sb.WriteString("\n")

	if len(cmd.Aliases) > 0 {
		fmt.Fprintf(&sb, "Aliases: %s\n", strings.Join(cmd.Aliases, ", "))
	}
	if cmd.Description != "" {
		fmt.Fprintf(&sb, "\n%s\n", cmd.Description)
	} else if cmd.Usage != "" {
		fmt.Fprintf(&sb, "\n%s\n", cmd.Usage)
	}

	// 子命令列表。
	if children := cmd.Children(); len(children) > 0 {
		sb.WriteString("\nCommands:\n")
		names := make([]string, 0, len(children))
		width := 0
		for _, ch := range children {
			n := ch.Name
			if len(ch.Aliases) > 0 {
				n += " (" + strings.Join(ch.Aliases, ", ") + ")"
			}
			names = append(names, n)
			if len(n) > width {
				width = len(n)
			}
		}
		for i, ch := range children {
			fmt.Fprintf(&sb, "  %-*s  %s\n", width, names[i], ch.Usage)
		}
	}

	// 参数列表（含继承参数）。
	if flags := effectiveFlags(cmd); len(flags) > 0 {
		sb.WriteString("\nFlags:\n")
		for _, f := range flags {
			var left strings.Builder
			if f.Shorthand != "" {
				fmt.Fprintf(&left, "-%s, ", f.Shorthand)
			}
			fmt.Fprintf(&left, "--%s", f.Name)
			for _, a := range f.Aliases {
				fmt.Fprintf(&left, ", --%s", a)
			}

			var notes []string
			if f.Required {
				notes = append(notes, "required")
			}
			if f.Default != "" {
				notes = append(notes, fmt.Sprintf("default %q", f.Default))
			}
			if len(f.Choices) > 0 {
				notes = append(notes, "one of: "+strings.Join(f.Choices, ", "))
			}
			if len(f.EnvVars) > 0 {
				notes = append(notes, "env: "+strings.Join(f.EnvVars, ", "))
			}
			if f.ConfigKey != "" {
				notes = append(notes, "config: "+f.ConfigKey)
			}
			if f.Persistent {
				notes = append(notes, "inherited by subcommands")
			}
			note := ""
			if len(notes) > 0 {
				note = " (" + strings.Join(notes, "; ") + ")"
			}
			fmt.Fprintf(&sb, "  %-24s  %s%s\n", left.String(), f.Usage, note)
		}
	}

	// 位置参数。
	if len(cmd.Positionals) > 0 {
		sb.WriteString("\nArguments:\n")
		for _, pos := range cmd.Positionals {
			max := "unlimited"
			if pos.Max >= 0 {
				max = fmt.Sprintf("%d", pos.Max)
			}
			fmt.Fprintf(&sb, "  %-24s  %s (min %d, max %s)\n", pos.Name, pos.Usage, pos.Min, max)
		}
	}
	return sb.String()
}

// sortedChildNames 供补全脚本使用：返回排序后的子命令名与别名。
func sortedChildNames(cmd *Command) []string {
	var out []string
	for _, ch := range cmd.Children() {
		out = append(out, ch.names()...)
	}
	sort.Strings(out)
	return out
}
