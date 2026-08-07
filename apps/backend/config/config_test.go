package config

import "testing"

// TestIsProductionGatesSwaggerExposure 锁定「仅非生产环境暴露 Swagger UI」的判定依据。
//
// router.Setup 用 !cfg.App.IsProduction() 决定是否注册 /swagger 路由。
// 这份文档完整列出全部接口、参数结构与字段含义，在生产暴露等于主动交出一张
// 攻击面地图；接口本身有鉴权，但没必要额外奉送。
//
// 之所以测这个看似显然的方法：它是那道开关的唯一依据，
// 一旦 Env 的取值约定变化（如加入 "prod" 简写而忘了归一），
// 生产环境就会静默地把文档暴露出去——不报错、无日志，只能靠人工发现。
func TestIsProductionGatesSwaggerExposure(t *testing.T) {
	cases := []struct {
		env          string
		isProduction bool
	}{
		{EnvProduction, true},
		{EnvDevelopment, false},
		{EnvTesting, false},
		// ResolveEnv 会把简写归一成全称，故到达这里的不该是简写；
		// 但真出现了也必须判为「非生产」而非误判成生产，
		// 否则开发环境反而打不开文档。
		{"prod", false},
		{"", false},
	}

	for _, tc := range cases {
		t.Run(tc.env, func(t *testing.T) {
			cfg := AppConfig{Env: tc.env}
			if got := cfg.IsProduction(); got != tc.isProduction {
				t.Errorf("env=%q：期望 IsProduction=%v，实际=%v（Swagger 暴露判定将出错）",
					tc.env, tc.isProduction, got)
			}
		})
	}
}

// TestResolveEnvNormalizesShorthand 确认简写被归一，避免配置目录与判定不一致。
func TestResolveEnvNormalizesShorthand(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	if got := ResolveEnv(); got != EnvProduction {
		t.Errorf("APP_ENV=prod 应归一为 %q，实际 %q", EnvProduction, got)
	}

	t.Setenv("APP_ENV", "dev")
	if got := ResolveEnv(); got != EnvDevelopment {
		t.Errorf("APP_ENV=dev 应归一为 %q，实际 %q", EnvDevelopment, got)
	}

	t.Setenv("APP_ENV", "test")
	if got := ResolveEnv(); got != EnvTesting {
		t.Errorf("APP_ENV=test 应归一为 %q，实际 %q", EnvTesting, got)
	}

	// 缺省应为 development，而不是空串或生产。
	t.Setenv("APP_ENV", "")
	if got := ResolveEnv(); got != EnvDevelopment {
		t.Errorf("APP_ENV 为空应缺省 %q，实际 %q", EnvDevelopment, got)
	}
}
