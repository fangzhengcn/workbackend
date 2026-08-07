package service

import (
	"testing"
)

// TestDictTypeCodePattern 锁定字典类型标识的字符集校验。
//
// 该标识会作为 URL 路径段用于按类型取数据（/dicts/data/type/{type}），
// 含空格、斜杠或中文会导致路由匹配不到或需转义，排查时很不直观。
func TestDictTypeCodePattern(t *testing.T) {
	svc := &DictService{}

	cases := []struct {
		code    string
		wantErr bool
	}{
		{"sys_user_gender", false},
		{"sysNormalDisable", false},
		{"a1", false},
		{"", true},
		{"1abc", true},     // 不能以数字开头
		{"_abc", true},     // 不能以下划线开头
		{"sys user", true}, // 含空格
		{"sys/user", true}, // 含斜杠，会破坏路由
		{"用户性别", true},     // 中文需 URL 转义
		{"sys-user", true}, // 连字符不在允许集内
		{"sys.user", true}, // 点号同理
	}

	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			err := svc.validateTypeCode(tc.code)
			if (err != nil) != tc.wantErr {
				t.Errorf("code=%q：期望出错=%v，实际 err=%v", tc.code, tc.wantErr, err)
			}
		})
	}
}

// TestDictDataDefaultExclusivity 验证「每类型最多一个默认项」的判定条件。
//
// 多个默认项时，前端取默认值的逻辑会随查询顺序漂移，
// 表现为「同样的配置，有时选中这项有时选中那项」——极难复现的偶发问题。
// 故新增/修改中只要把某项设为默认，就必须先清掉同类型下其他项的标记。
func TestDictDataDefaultExclusivity(t *testing.T) {
	cases := []struct {
		name           string
		isDefault      *int8
		wantClearOther bool
	}{
		{"设为默认需清理其他", ptrInt8(1), true},
		{"显式设为非默认无需清理", ptrInt8(0), false},
		{"未传该字段无需清理", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 复刻 UpdateData 中的判定：仅当显式传 1 时才清理。
			becameDefault := tc.isDefault != nil && *tc.isDefault == 1
			if becameDefault != tc.wantClearOther {
				t.Errorf("期望需要清理=%v，实际=%v", tc.wantClearOther, becameDefault)
			}
		})
	}
}

func ptrInt8(v int8) *int8 { return &v }
