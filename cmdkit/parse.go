package cmdkit

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// ErrHelp 在解析遇到 -h/--help 时返回，调用方可据此打印 Help。
var ErrHelp = fmt.Errorf("help requested")

// Parser 是无状态的解析器，可并发使用；所有可变输入都来自 ParseOptions。
type Parser struct {
	ExitCodes ExitCodes
}

// NewParser 创建使用默认退出码的解析器。
func NewParser() *Parser {
	return &Parser{ExitCodes: DefaultExitCodes()}
}

// ParseOptions 为单次解析的全部输入，解析之间不共享可变状态。
type ParseOptions struct {
	Args   []string          // 待解析参数（不含程序名）
	Env    map[string]string // 环境变量；nil 时使用 os.Environ
	Config map[string]string // 配置文件键值
}

// Parse 解析并分发：定位目标命令、绑定参数、校验、执行 Run。
// 任何解析/校验失败都返回 *Error 且不产生部分绑定的 Context。
func (p *Parser) Parse(root *Command, opts ParseOptions) (*Context, error) {
	env := opts.Env
	if env == nil {
		env = map[string]string{}
		for _, kv := range os.Environ() {
			if k, v, ok := strings.Cut(kv, "="); ok {
				env[k] = v
			}
		}
	}
	cfg := opts.Config
	if cfg == nil {
		cfg = map[string]string{}
	}

	// 1. 沿命令树下行，直到 token 不再匹配任何子命令。
	cmd := root
	rest := opts.Args
	for len(rest) > 0 {
		tok := rest[0]
		if strings.HasPrefix(tok, "-") {
			break // 进入参数区
		}
		child, err := cmd.findChild(tok)
		if err != nil {
			if len(cmd.Children()) == 0 {
				break // 叶子命令：剩余 token 按位置参数处理
			}
			return nil, newError(KindParse, p.ExitCodes, "%s", err.Error())
		}
		cmd = child
		rest = rest[1:]
	}

	// 2. 解析命名参数与位置参数。
	flags := effectiveFlags(cmd)
	byLong := map[string]*Flag{}
	byShort := map[string]*Flag{}
	for i := range flags {
		f := &flags[i]
		byLong[f.Name] = f
		for _, a := range f.Aliases {
			byLong[a] = f
		}
		if f.Shorthand != "" {
			byShort[f.Shorthand] = f
		}
	}

	cliValues := map[string]string{}   // 命令行显式提供的值
	provided := map[string]bool{}      // 显式提供的参数（用于互斥/至少一组判断）
	var positionals []string
	helpRequested := false

	for i := 0; i < len(rest); i++ {
		tok := rest[i]
		switch {
		case tok == "--":
			positionals = append(positionals, rest[i+1:]...)
			i = len(rest)
		case tok == "-h" || tok == "--help":
			helpRequested = true
		case strings.HasPrefix(tok, "--"):
			name, val, hasVal := strings.Cut(tok[2:], "=")
			f, ok := byLong[name]
			if !ok {
				return nil, newError(KindParse, p.ExitCodes, "unknown flag --%s for command %q", name, cmd.Name)
			}
			if !hasVal {
				if i+1 >= len(rest) {
					return nil, newError(KindParse, p.ExitCodes, "flag --%s requires a value", f.Name)
				}
				i++
				val = rest[i]
			}
			cliValues[f.Name] = val
			provided[f.Name] = true
		case strings.HasPrefix(tok, "-") && len(tok) > 1:
			name, val, hasVal := strings.Cut(tok[1:], "=")
			f, ok := byShort[name]
			if !ok {
				return nil, newError(KindParse, p.ExitCodes, "unknown flag -%s for command %q", name, cmd.Name)
			}
			if !hasVal {
				if i+1 >= len(rest) {
					return nil, newError(KindParse, p.ExitCodes, "flag -%s requires a value", f.Shorthand)
				}
				i++
				val = rest[i]
			}
			cliValues[f.Name] = val
			provided[f.Name] = true
		default:
			positionals = append(positionals, tok)
		}
	}

	if helpRequested {
		return &Context{Command: cmd}, ErrHelp
	}

	// 3. 多来源取值：命令行 > 环境变量 > 配置文件 > 内置默认值。
	values := map[string]BoundValue{}
	for _, f := range flags {
		bv := BoundValue{Source: SourceNone}
		if v, ok := cliValues[f.Name]; ok {
			bv = BoundValue{Value: v, Source: SourceFlag}
		} else {
			for _, ev := range f.EnvVars {
				if v, ok := env[ev]; ok {
					bv = BoundValue{Value: v, Source: SourceEnv}
					break
				}
			}
			if bv.Source == SourceNone && f.ConfigKey != "" {
				if v, ok := cfg[f.ConfigKey]; ok {
					bv = BoundValue{Value: v, Source: SourceConfig}
				}
			}
			if bv.Source == SourceNone && f.Default != "" {
				bv = BoundValue{Value: f.Default, Source: SourceDefault}
			}
		}
		values[f.Name] = bv
	}

	// 4. 校验：全部通过才产生绑定结果，失败时不留下部分绑定。
	if err := p.validate(cmd, flags, values, provided, positionals); err != nil {
		return nil, err
	}

	ctx := &Context{Command: cmd, values: values, args: positionals}

	// 5. 分发：执行业务处理函数。
	if cmd.Run != nil {
		if err := cmd.Run(ctx); err != nil {
			return nil, newError(KindBusiness, p.ExitCodes, "command %q failed: %v", cmd.Name, err)
		}
	}
	return ctx, nil
}

// validate 执行全部约束校验，任一失败即返回带参数信息的错误。
func (p *Parser) validate(cmd *Command, flags []Flag, values map[string]BoundValue, provided map[string]bool, positionals []string) error {
	for _, f := range flags {
		bv := values[f.Name]

		// 必填。
		if f.Required && bv.Source == SourceNone {
			return newError(KindValidation, p.ExitCodes, "required flag --%s is missing", f.Name)
		}
		if bv.Source == SourceNone {
			continue
		}
		// 枚举约束。
		if len(f.Choices) > 0 {
			ok := false
			for _, c := range f.Choices {
				if bv.Value == c {
					ok = true
					break
				}
			}
			if !ok {
				return newError(KindValidation, p.ExitCodes,
					"invalid value %q for --%s (from %s), allowed: %s",
					bv.Value, f.Name, bv.Source, strings.Join(f.Choices, ", "))
			}
		}
		// 自定义校验。
		if f.Validator != nil {
			if err := f.Validator(bv.Value); err != nil {
				return newError(KindValidation, p.ExitCodes,
					"invalid value for --%s (from %s): %v", f.Name, bv.Source, err)
			}
		}
	}

	// 互斥参数组：同组最多显式出现一个。
	mutexSeen := map[string]string{}
	for _, f := range flags {
		if f.MutexGroup == "" || !provided[f.Name] {
			continue
		}
		if prev, clash := mutexSeen[f.MutexGroup]; clash {
			return newError(KindValidation, p.ExitCodes,
				"flags --%s and --%s are mutually exclusive (group %q)", prev, f.Name, f.MutexGroup)
		}
		mutexSeen[f.MutexGroup] = f.Name
	}

	// 至少选一个参数组。
	oneOf := map[string][]string{}
	for _, f := range flags {
		if f.OneOfGroup != "" {
			oneOf[f.OneOfGroup] = append(oneOf[f.OneOfGroup], f.Name)
		}
	}
	for group, members := range oneOf {
		hit := false
		for _, m := range members {
			if provided[m] {
				hit = true
				break
			}
		}
		if !hit {
			sort.Strings(members)
			return newError(KindValidation, p.ExitCodes,
				"at least one of --%s is required (group %q)", strings.Join(members, ", --"), group)
		}
	}

	// 位置参数数量与校验。
	for _, pos := range cmd.Positionals {
		if len(positionals) < pos.Min {
			return newError(KindValidation, p.ExitCodes,
				"positional argument %q requires at least %d value(s), got %d", pos.Name, pos.Min, len(positionals))
		}
		if pos.Max >= 0 && len(positionals) > pos.Max {
			return newError(KindValidation, p.ExitCodes,
				"positional argument %q accepts at most %d value(s), got %d", pos.Name, pos.Max, len(positionals))
		}
		if pos.Validator != nil {
			for _, v := range positionals {
				if err := pos.Validator(v); err != nil {
					return newError(KindValidation, p.ExitCodes,
						"invalid positional argument %q value %q: %v", pos.Name, v, err)
				}
			}
		}
	}
	return nil
}
