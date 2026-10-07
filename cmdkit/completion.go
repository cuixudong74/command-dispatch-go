package cmdkit

import (
	"fmt"
	"sort"
	"strings"
)

// completionData 从命令树收集补全数据：每条命令路径的子命令、参数名与枚举取值。
type completionData struct {
	subs    map[string][]string // path -> 子命令名与别名
	flags   map[string][]string // path -> 参数名（--long / -s）
	choices map[string][]string // path+" "+--flag -> 枚举取值
}

func collectCompletion(root *Command) *completionData {
	d := &completionData{
		subs:    map[string][]string{},
		flags:   map[string][]string{},
		choices: map[string][]string{},
	}
	var walk func(cmd *Command, path string)
	walk = func(cmd *Command, path string) {
		d.subs[path] = sortedChildNames(cmd)
		var flags []string
		for _, f := range effectiveFlags(cmd) {
			flags = append(flags, "--"+f.Name)
			for _, a := range f.Aliases {
				flags = append(flags, "--"+a)
			}
			if f.Shorthand != "" {
				flags = append(flags, "-"+f.Shorthand)
			}
			if len(f.Choices) > 0 {
				d.choices[path+" --"+f.Name] = f.Choices
			}
		}
		sort.Strings(flags)
		d.flags[path] = flags
		for _, ch := range cmd.Children() {
			walk(ch, path+" "+ch.Name)
		}
	}
	walk(root, "")
	return d
}

// BashCompletion 生成 bash 补全脚本，覆盖子命令、参数名与枚举取值。
func BashCompletion(root *Command, prog string) string {
	d := collectCompletion(root)
	var sb strings.Builder

	fmt.Fprintf(&sb, "# bash completion for %s\n", prog)
	fmt.Fprintf(&sb, "_%s_complete() {\n", strings.ReplaceAll(prog, "-", "_"))
	sb.WriteString("  local cur prev\n")
	sb.WriteString("  cur=\"${COMP_WORDS[COMP_CWORD]}\"\n")
	sb.WriteString("  prev=\"${COMP_WORDS[COMP_CWORD-1]}\"\n")

	// 逐路径生成枚举取值的 prev 匹配。
	sb.WriteString("  case \"${COMP_WORDS[*]:1:COMP_CWORD-1}|$prev\" in\n")
	for key, choices := range d.choices {
		path, flag, _ := strings.Cut(key, " --")
		_ = path // 路径匹配由下方通用逻辑处理，这里按参数名全局匹配枚举
		fmt.Fprintf(&sb, "    *--%s) COMPREPLY=($(compgen -W %q -- \"$cur\")); return ;;\n",
			flag, strings.Join(choices, " "))
	}
	sb.WriteString("  esac\n")

	// 计算当前命令路径：跳过参数及其值。
	sb.WriteString(`  local path="" i w
  for ((i=1; i<COMP_CWORD; i++)); do
    w="${COMP_WORDS[i]}"
    case "$w" in
      -*) ;;
      *) path="$path $w" ;;
    esac
  done
`)
	// 按路径派发候选。
	sb.WriteString("  case \"$path\" in\n")
	paths := make([]string, 0, len(d.subs))
	for p := range d.subs {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		words := append(append([]string{}, d.subs[p]...), d.flags[p]...)
		fmt.Fprintf(&sb, "    %q) COMPREPLY=($(compgen -W %q -- \"$cur\")) ;;\n",
			strings.TrimSpace(p), strings.Join(words, " "))
	}
	sb.WriteString("  esac\n}\n")
	fmt.Fprintf(&sb, "complete -F _%s_complete %s\n", strings.ReplaceAll(prog, "-", "_"), prog)
	return sb.String()
}

// ZshCompletion 生成 zsh 补全脚本，覆盖子命令、参数名与枚举取值。
func ZshCompletion(root *Command, prog string) string {
	d := collectCompletion(root)
	var sb strings.Builder

	fmt.Fprintf(&sb, "#compdef %s\n", prog)
	fmt.Fprintf(&sb, "# zsh completion for %s\n", prog)
	fmt.Fprintf(&sb, "_%s() {\n", strings.ReplaceAll(prog, "-", "_"))
	sb.WriteString("  local -a path\n  local i\n")
	sb.WriteString(`  local cmdpath=""
  for ((i=2; i<CURRENT; i++)); do
    case "${words[i]}" in
      -*) ;;
      *) cmdpath="$cmdpath ${words[i]}" ;;
    esac
  done
`)
	// 枚举取值：根据上一个词补全。
	sb.WriteString("  case \"${words[CURRENT-1]}\" in\n")
	for key, choices := range d.choices {
		_, flag, _ := strings.Cut(key, " --")
		fmt.Fprintf(&sb, "    --%s) compadd -- %s; return ;;\n", flag, strings.Join(choices, " "))
	}
	sb.WriteString("  esac\n")

	sb.WriteString("  case \"$cmdpath\" in\n")
	paths := make([]string, 0, len(d.subs))
	for p := range d.subs {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		words := append(append([]string{}, d.subs[p]...), d.flags[p]...)
		fmt.Fprintf(&sb, "    %q) compadd -- %s ;;\n", strings.TrimSpace(p), strings.Join(words, " "))
	}
	sb.WriteString("  esac\n}\n")
	fmt.Fprintf(&sb, "_%s \"$@\"\n", strings.ReplaceAll(prog, "-", "_"))
	return sb.String()
}
