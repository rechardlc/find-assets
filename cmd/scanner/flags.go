package main

import "strings"

// flagAliases 将短选项映射到完整 flag 名。
// 规则：优先取首字母；冲突时适度加长（如 period→p，pattern→pt；serve→s，source→so）。
var flagAliases = map[string]string{
	"p":  "period",
	"pt": "pattern",
	"w":  "workers",
	"b":  "bars",
	"r":  "range",
	"v":  "volume",
	"e":  "export",
	"o":  "out",
	"s":  "serve",
	"a":  "addr",
	"so": "source",
	"h":  "help",
}

// knownLongFlags 已是完整名的 flag，展开时原样保留。
var knownLongFlags = map[string]bool{
	"period": true, "pattern": true, "workers": true, "bars": true,
	"range": true, "volume": true, "export": true, "out": true,
	"serve": true, "addr": true, "source": true, "help": true,
}

// expandShortFlags 把 argv 中的短选项展开为完整 flag 名，供 flag.Parse 使用。
// 支持 -p=day、-p day、-pt pierce、布尔开关 -s 等形式；长选项与位置参数不变。
func expandShortFlags(argv []string) []string {
	if len(argv) <= 1 {
		return argv
	}
	out := make([]string, 0, len(argv))
	out = append(out, argv[0])

	for i := 1; i < len(argv); i++ {
		arg := argv[i]
		if !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") {
			out = append(out, arg)
			continue
		}

		body := strings.TrimPrefix(arg, "-")
		name, val, hasEq := strings.Cut(body, "=")

		long := resolveFlagName(name)
		if long == "" {
			out = append(out, arg)
			continue
		}

		if hasEq {
			out = append(out, "-"+long+"="+val)
			continue
		}

		// 布尔 flag 无值；其余尝试消费下一个 argv 作为值。
		if isBoolFlag(long) {
			out = append(out, "-"+long)
			continue
		}
		if i+1 < len(argv) && !strings.HasPrefix(argv[i+1], "-") {
			out = append(out, "-"+long, argv[i+1])
			i++
			continue
		}
		out = append(out, "-"+long)
	}
	return out
}

func resolveFlagName(name string) string {
	if knownLongFlags[name] {
		return name
	}
	if long, ok := flagAliases[name]; ok {
		return long
	}
	return ""
}

func isBoolFlag(name string) bool {
	return name == "serve" || name == "help"
}
