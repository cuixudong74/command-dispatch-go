package command

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ParseOptions 一次解析的输入配置。每次解析独立，不共享可变状态。
type ParseOptions struct {
	Env    map[string]string // 环境变量快照（nil 时取 os.Environ）
	Config map[string]string // 配置文件内容（参数名 -> 值）
}

// Result 解析结果。
type Result struct {
	Command *Command
	Ctx     *Context
}

// Parse 从根命令出发解析 args，返回绑定结果。
// 解析与校验全部在局部状态上进行，失败时不留下部分绑定（Result 为 nil）。
// 来源优先级固定：命令行 > 环境变量 > 配置文件 > 内置默认值。
func (c *Command) Parse(args []string, opts *ParseOptions) (*Result, error) {
	if opts == nil {
		opts = &ParseOptions{}
	}
	// 1. 定位目标命令：沿命令树下钻。
	cmd := c
	i := 0
	for i < len(args) {
		tok := args[i]
		if strings.HasPrefix(tok, "-") {
			break
		}
		cmd.mu.RLock()
		hasChildren := len(cmd.children) > 0
		cmd.mu.RUnlock()
		if !hasChildren {
			break
		}
		next, err := cmd.Lookup(tok)
		if err != nil {
			return nil, err
		}
		cmd = next
		i++
	}
	rest := args[i:]

	// 2. 解析标志与位置参数到局部状态。
	params := cmd.effectiveParams()
	cliValues := map[string][]string{} // 参数名 -> 命令行出现的原始值
	var positionals []string
	j := 0
	for j < len(rest) {
		tok := rest[j]
		if tok == "--" {
			positionals = append(positionals, rest[j+1:]...)
			break
		}
		if strings.HasPrefix(tok, "--") {
			name := strings.TrimPrefix(tok, "--")
			var inlineVal string
			hasInline := false
			if k := strings.IndexByte(name, '='); k >= 0 {
				name, inlineVal, hasInline = name[:k], name[k+1:], true
			}
			p := findParam(params, name)
			if p == nil {
				return nil, parseErr("未知参数 --%s（命令 %q）", name, cmd.Path())
			}
			if p.Type == Bool && !hasInline {
				cliValues[p.Name] = append(cliValues[p.Name], "true")
				j++
				continue
			}
			if !hasInline {
				if j+1 >= len(rest) {
					return nil, validErr(p.Name, "缺少取值")
				}
				inlineVal = rest[j+1]
				j++
			}
			cliValues[p.Name] = append(cliValues[p.Name], inlineVal)
			j++
			continue
		}
		if strings.HasPrefix(tok, "-") && tok != "-" {
			return nil, parseErr("无法识别的参数 %q（仅支持 --长参数）", tok)
		}
		positionals = append(positionals, tok)
		j++
	}

	// 位置参数按声明顺序匹配，Variadic 吸收剩余。
	posValues := map[string][]string{}
	pi := 0
	for _, p := range params {
		if !p.Positional && !p.Variadic {
			continue
		}
		if p.Variadic {
			if pi < len(positionals) {
				posValues[p.Name] = append(posValues[p.Name], positionals[pi:]...)
				pi = len(positionals)
			}
			continue
		}
		if pi < len(positionals) {
			posValues[p.Name] = append(posValues[p.Name], positionals[pi])
			pi++
		}
	}
	if pi < len(positionals) {
		return nil, parseErr("多余的位置参数: %s", strings.Join(positionals[pi:], " "))
	}

	// 3. 多来源合并 + 校验，全部写入局部 bound。
	bound := map[string]*BoundParam{}
	for _, p := range params {
		bp, err := resolveParam(p, cliValues[p.Name], posValues[p.Name], opts)
		if err != nil {
			return nil, err
		}
		if bp != nil {
			bound[p.Name] = bp
		}
	}
	if err := validateGroups(cmd.effectiveGroups(), bound); err != nil {
		return nil, err
	}

	return &Result{
		Command: cmd,
		Ctx: &Context{
			Command: cmd,
			Bound:   bound,
			Args:    nil,
		},
	}, nil
}

// resolveParam 按固定优先级合并来源并校验单个参数。
// 优先级：命令行 > 环境变量 > 配置文件 > 内置默认值。
func resolveParam(p *Param, cli, pos []string, opts *ParseOptions) (*BoundParam, error) {
	var raw []string
	src := SourceNone
	switch {
	case len(cli) > 0:
		raw, src = cli, SourceCLI
	case len(pos) > 0:
		raw, src = pos, SourceCLI
	case p.EnvVar != "" && envValue(opts.Env, p.EnvVar) != "":
		raw, src = []string{envValue(opts.Env, p.EnvVar)}, SourceEnv
	case opts.Config != nil && opts.Config[p.Name] != "":
		raw, src = []string{opts.Config[p.Name]}, SourceConfig
	case p.Default != "":
		raw, src = []string{p.Default}, SourceDefault
	}

	if src == SourceNone {
		if p.Required {
			return nil, validErr(p.Name, "缺少必填参数（可通过 --%s、环境变量 %s 或配置文件提供）", p.Name, p.EnvVar)
		}
		return nil, nil
	}

	bp := &BoundParam{Param: p, Source: src}
	if p.Type == StringSlice || p.Variadic {
		var vals []string
		for _, r := range raw {
			vals = append(vals, strings.Split(r, ",")...)
		}
		bp.Values = vals
		bp.Value = strings.Join(vals, ",")
	} else {
		bp.Value = raw[len(raw)-1] // 重复出现时取最后一次
		bp.Values = []string{bp.Value}
	}

	// 类型校验。
	if err := checkType(p, bp.Value); err != nil {
		return nil, err
	}
	// 枚举校验。
	if len(p.Enum) > 0 {
		for _, v := range bp.Values {
			ok := false
			for _, e := range p.Enum {
				if v == e {
					ok = true
					break
				}
			}
			if !ok {
				return nil, validErr(p.Name, "取值 %q 不在允许范围 %v 内", v, p.Enum)
			}
		}
	}
	// 自定义校验。
	if p.Validator != nil {
		if err := p.Validator(bp.Value); err != nil {
			return nil, validErr(p.Name, "自定义校验失败: %v", err)
		}
	}
	return bp, nil
}

// envValue 从环境快照（nil 时取进程环境）读取变量。
func envValue(env map[string]string, key string) string {
	if env == nil {
		return os.Getenv(key)
	}
	return env[key]
}

// checkType 校验取值能否转换为目标类型。
func checkType(p *Param, v string) error {
	switch p.Type {
	case Int:
		if _, err := strconv.Atoi(v); err != nil {
			return validErr(p.Name, "取值 %q 不是合法整数", v)
		}
	case Float:
		if _, err := strconv.ParseFloat(v, 64); err != nil {
			return validErr(p.Name, "取值 %q 不是合法浮点数", v)
		}
	case Bool:
		if _, err := strconv.ParseBool(v); err != nil {
			return validErr(p.Name, "取值 %q 不是合法布尔值", v)
		}
	}
	return nil
}

// validateGroups 校验参数组约束。
// “已提供”指取值来自命令行 / 环境变量 / 配置文件（默认值不计入）。
func validateGroups(groups []*Group, bound map[string]*BoundParam) error {
	for _, g := range groups {
		var provided []string
		for _, name := range g.Params {
			if bp, ok := bound[name]; ok && bp.Source >= SourceConfig {
				provided = append(provided, name)
			}
		}
		switch g.Mode {
		case GroupMutex:
			if len(provided) > 1 {
				return &Error{
					Kind:   ErrKindValidation,
					Param:  provided[1],
					Msg:    fmt.Sprintf("参数组 %q 内参数互斥，不能同时提供", g.Name),
					Causes: provided,
				}
			}
		case GroupAtLeastOne:
			if len(provided) == 0 {
				return &Error{
					Kind:   ErrKindValidation,
					Param:  g.Params[0],
					Msg:    fmt.Sprintf("参数组 %q 要求至少提供一个参数", g.Name),
					Causes: g.Params,
				}
			}
		}
	}
	return nil
}

// Execute 解析并执行业务函数，返回应使用的退出码。
// 解析错误、校验错误与业务错误分别映射到 codes 中配置的退出码。
func (c *Command) Execute(args []string, opts *ParseOptions, codes ExitCodes) int {
	res, err := c.Parse(args, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		return ExitCode(err, codes)
	}
	if res.Command.Run == nil {
		fmt.Fprint(os.Stderr, res.Command.Help())
		return 0
	}
	if err := res.Command.Run(res.Ctx); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		return ExitCode(err, codes)
	}
	return 0
}
