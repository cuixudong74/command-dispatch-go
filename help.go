package command

import (
	"fmt"
	"sort"
	"strings"
)

// Help 生成与声明的命令树一致的帮助信息（含别名与默认值）。
func (c *Command) Help() string {
	var b strings.Builder
	fmt.Fprintf(&b, "用法: %s", c.Path())
	c.mu.RLock()
	hasChildren := len(c.children) > 0
	c.mu.RUnlock()
	if hasChildren {
		b.WriteString(" <命令>")
	}
	for _, p := range c.effectiveParams() {
		if p.Positional || p.Variadic {
			if p.Variadic {
				fmt.Fprintf(&b, " [%s...]", p.Name)
			} else {
				fmt.Fprintf(&b, " <%s>", p.Name)
			}
			continue
		}
		if p.Required {
			fmt.Fprintf(&b, " --%s=<%s>", p.Name, typeName(p.Type))
		} else {
			fmt.Fprintf(&b, " [--%s=<%s>]", p.Name, typeName(p.Type))
		}
	}
	b.WriteString("\n")
	if c.Usage != "" {
		fmt.Fprintf(&b, "\n%s\n", c.Usage)
	}

	if hasChildren {
		b.WriteString("\n子命令:\n")
		c.mu.RLock()
		subs := make([]*Command, 0, len(c.children))
		for _, s := range c.children {
			subs = append(subs, s)
		}
		c.mu.RUnlock()
		sort.Slice(subs, func(i, j int) bool { return subs[i].Name < subs[j].Name })
		for _, s := range subs {
			name := s.Name
			if len(s.Aliases) > 0 {
				name += "（别名: " + strings.Join(s.Aliases, ", ") + "）"
			}
			fmt.Fprintf(&b, "  %-24s %s\n", name, s.Usage)
		}
	}

	if params := c.effectiveParams(); len(params) > 0 {
		b.WriteString("\n参数:\n")
		for _, p := range params {
			var names []string
			if p.Positional || p.Variadic {
				names = append(names, p.Name)
			} else {
				names = append(names, "--"+p.Name)
				for _, a := range p.Aliases {
					names = append(names, "--"+a)
				}
			}
			var notes []string
			if p.Required {
				notes = append(notes, "必填")
			}
			if p.Default != "" {
				notes = append(notes, "默认: "+p.Default)
			}
			if p.EnvVar != "" {
				notes = append(notes, "环境变量: "+p.EnvVar)
			}
			if len(p.Enum) > 0 {
				notes = append(notes, "可选: "+strings.Join(p.Enum, "|"))
			}
			line := fmt.Sprintf("  %-28s %s", strings.Join(names, ", "), p.Usage)
			if len(notes) > 0 {
				line += "（" + strings.Join(notes, "；") + "）"
			}
			b.WriteString(line + "\n")
		}
	}

	if groups := c.effectiveGroups(); len(groups) > 0 {
		b.WriteString("\n参数组:\n")
		for _, g := range groups {
			mode := "互斥（至多一个）"
			if g.Mode == GroupAtLeastOne {
				mode = "至少一个"
			}
			fmt.Fprintf(&b, "  %s: %s [%s]\n", g.Name, mode, strings.Join(g.Params, ", "))
		}
	}
	return b.String()
}

func typeName(t ValueType) string {
	switch t {
	case Int:
		return "int"
	case Float:
		return "float"
	case Bool:
		return "bool"
	case StringSlice:
		return "list"
	default:
		return "string"
	}
}
