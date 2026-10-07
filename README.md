# command-dispatch-go

可复用的命令解析与分发框架。定义命令的人只需声明命令与参数的结构和约束，
解析、校验、绑定、帮助与补全全部由框架处理。

```go
root := command.New("app", "我的应用", nil)
root.Params = []*command.Param{
    {Name: "token", Usage: "访问令牌", EnvVar: "APP_TOKEN", Default: "dev"},
}
remote := command.New("remote", "管理远端", nil)
remote.Aliases = []string{"r"}
add := command.New("add", "添加远端", func(ctx *command.Context) error {
    fmt.Println(ctx.Get("url").String(), ctx.Get("token").String())
    return nil
})
add.Params = []*command.Param{{Name: "url", Required: true}}
remote.AddCommand(add)
root.AddCommand(remote)

os.Exit(root.Execute(os.Args[1:], nil, command.DefaultExitCodes()))
```

## 解析语义

### 命令树与前缀缩写

- 支持任意多级子命令；每个命令可声明 `Aliases` 别名。
- 输入按 **精确主名 > 精确别名 > 唯一前缀** 的顺序匹配。
- 前缀命中多个候选时报解析错误，并在错误中列出全部候选命令名。
- 完全不匹配时报解析错误，并按编辑距离给出最接近的建议（"你是想要 …"）。

### 参数来源与优先级

同一参数的取值可来自四个来源，优先级固定且可预期：

```
命令行 (--name)  >  环境变量 (EnvVar)  >  配置文件 (ParseOptions.Config)  >  内置默认值 (Default)
```

命令行永远覆盖其余来源。每个绑定结果通过 `BoundParam.Source`
（`SourceCLI` / `SourceEnv` / `SourceConfig` / `SourceDefault`）可追溯最终取值的来源。

- 环境变量默认取 `os.Environ`，也可用 `ParseOptions.Env` 注入快照（便于测试与隔离）。
- 配置文件以 `map[string]string`（参数名 → 值）形式通过 `ParseOptions.Config` 传入，
  文件格式（JSON/YAML/TOML）由使用方自行解析。
- 同一参数在命令行重复出现时取最后一次；`StringSlice` 类型则累积全部出现并按逗号拆分。

### 参数约束与校验

- `Required`：四个来源都未提供时校验失败。
- `Variadic` + `Positional`：位置参数按声明顺序匹配，可变参数吸收剩余全部输入。
- `Group{Mode: GroupMutex}`：组内参数互斥，至多提供一个。
- `Group{Mode: GroupAtLeastOne}`：组内至少提供一个。
- `Enum`：取值必须在枚举集合内。
- `Validator`：自定义校验函数，在类型与枚举校验之后执行。

**校验失败约定**：错误信息中一定带出错参数名（`Error.Param`）；
解析或校验失败时 `Parse` 返回的 `Result` 为 `nil`，**不会留下部分绑定**。
参数组判定中"已提供"指取值来自命令行 / 环境变量 / 配置文件，内置默认值不计入。

### 参数继承与覆盖

- 父级命令声明的参数对全部后代命令可见（含参数组）。
- 子命令声明同名参数时**整体覆盖**父级声明（默认值、校验规则、环境变量等随之替换），
  覆盖关系为"就近优先"，重复定义不视为冲突、不会报错。
- 帮助信息展示的永远是覆盖后的生效声明。

### 错误分类与退出码

| 类别 | 触发 | 默认退出码 |
|---|---|---|
| `ErrKindParse` | 未知命令、歧义前缀、缺参数值等 | 2 |
| `ErrKindValidation` | 必填缺失、互斥冲突、枚举越界、自定义校验失败 | 3 |
| `ErrKindBusiness` | `Run` 返回的错误 | 1 |

退出码可通过 `ExitCodes` 自定义；`ExitCode` 是纯函数，相同输入下结果稳定。
`KindOf(err)` 可从任意 error 中提取类别（非框架错误一律视为业务错误）。

### 并发与状态约定

- `AddCommand` 与 `Lookup`/`Parse` 可并发调用（内部读写锁保护）。
- 每次 `Parse` 使用独立的局部状态，多次解析之间不共享可变状态；
  同一进程内并发发起多个解析互不串扰（见 `TestConcurrentRegisterAndParse`，`-race` 通过）。
- 命令树在解析期间只读，绑定结果全部存放在当次返回的 `Context` 中。

### 帮助与补全

- `Command.Help()` 由声明的命令树自动生成，包含子命令、别名、参数、
  默认值、环境变量、枚举与参数组约束，与声明始终一致。
- `Command.CompletionScript(command.Bash)` / `(command.Zsh)` 生成补全脚本，
  覆盖子命令（含别名）、参数名与枚举取值。

## 测试

```sh
go test -race ./...
```

覆盖场景：歧义前缀、唯一前缀与别名、未知命令建议、来源优先级（CLI/env/config/default）、
参数组互斥与至少一个、必填与无部分绑定、可变参数与枚举、继承覆盖、
退出码分类与稳定性、并发注册与并发解析、帮助一致性、补全脚本内容。
