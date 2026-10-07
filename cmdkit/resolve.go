package cmdkit

// effectiveFlags 计算命令实际可用的参数集：
// 祖先声明的 Persistent 参数沿命令链向下继承；
// 子命令本地同名参数覆盖继承参数的默认值与校验规则（其余字段以本地为准，
// 本地未设置的字段回落到继承值），不会因重复定义而冲突。
func effectiveFlags(cmd *Command) []Flag {
	// 自根向下收集继承参数，保证覆盖顺序：越靠近叶子的本地定义越优先。
	var chain []*Command
	for c := cmd; c != nil; c = c.parent {
		chain = append(chain, c)
	}

	merged := map[string]*Flag{}
	var order []string

	put := func(f Flag, inherited bool) {
		existing, ok := merged[f.Name]
		if !ok {
			cp := f
			merged[f.Name] = &cp
			order = append(order, f.Name)
			return
		}
		// 同名覆盖：本地定义优先，空字段回落到已有（继承）值。
		overlay(existing, f)
	}

	// 先放祖先的 Persistent 参数（根在前，靠近叶子的祖先后覆盖）。
	for i := len(chain) - 1; i >= 0; i-- {
		ancestor := chain[i]
		if ancestor == cmd {
			continue
		}
		for _, f := range ancestor.Flags {
			if f.Persistent {
				put(f, true)
			}
		}
	}
	// 再放本命令的全部本地参数（覆盖同名继承参数）。
	for _, f := range cmd.Flags {
		put(f, false)
	}

	out := make([]Flag, 0, len(order))
	for _, name := range order {
		out = append(out, *merged[name])
	}
	return out
}

// overlay 用 src 覆盖 dst：非零字段覆盖，零字段保留 dst 原值。
// Validator 与 Choices 显式设置为非零时才覆盖，实现"子命令可覆盖默认值或校验规则"。
func overlay(dst *Flag, src Flag) {
	if src.Shorthand != "" {
		dst.Shorthand = src.Shorthand
	}
	if len(src.Aliases) > 0 {
		dst.Aliases = src.Aliases
	}
	if src.Usage != "" {
		dst.Usage = src.Usage
	}
	if src.Required {
		dst.Required = true
	}
	if src.Default != "" {
		dst.Default = src.Default
	}
	if len(src.EnvVars) > 0 {
		dst.EnvVars = src.EnvVars
	}
	if src.ConfigKey != "" {
		dst.ConfigKey = src.ConfigKey
	}
	if len(src.Choices) > 0 {
		dst.Choices = src.Choices
	}
	if src.Validator != nil {
		dst.Validator = src.Validator
	}
	if src.MutexGroup != "" {
		dst.MutexGroup = src.MutexGroup
	}
	if src.OneOfGroup != "" {
		dst.OneOfGroup = src.OneOfGroup
	}
	// Persistent 属性以继承来源为准，不被本地覆盖改变。
}
