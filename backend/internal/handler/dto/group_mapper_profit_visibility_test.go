package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// profitControlJSONFields 是分组利润控制的三个 JSON 字段。它们与同响应中的
// rate_multiplier 相乘即可反推出运营方的上游采购成本上限，属于内部经营信息，
// 只能出现在管理员 DTO 中。
var profitControlJSONFields = []string{
	"profit_control_enabled",
	"profit_min_margin",
	"profit_safety_buffer",
}

var codexOverdraftJSONFields = []string{
	"codex_overdraft_enabled",
	"codex_overdraft_next_group_id",
}

func profitControlServiceGroup() *service.Group {
	nextGroupID := int64(9)
	return &service.Group{
		ID:                         7,
		Name:                       "profit-gated",
		Platform:                   service.PlatformOpenAI,
		RateMultiplier:             2.0,
		Status:                     service.StatusActive,
		ProfitControlEnabled:       true,
		ProfitMinMargin:            0.3,
		ProfitSafetyBuffer:         0.05,
		CodexOverdraftEnabled:      true,
		CodexOverdraftNextGroupID:  &nextGroupID,
	}
}

func marshalToMap(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

// TestGroupFromServiceOmitsProfitControl 钉死普通用户侧的分组 DTO 不泄露利润控制配置。
func TestGroupFromServiceOmitsProfitControl(t *testing.T) {
	for name, got := range map[string]any{
		"GroupFromService":        GroupFromService(profitControlServiceGroup()),
		"GroupFromServiceShallow": GroupFromServiceShallow(profitControlServiceGroup()),
	} {
		fields := marshalToMap(t, got)
		for _, f := range profitControlJSONFields {
			if _, ok := fields[f]; ok {
				t.Errorf("%s: 普通用户 DTO 不得包含 %q", name, f)
			}
		}
		for _, f := range codexOverdraftJSONFields {
			if _, ok := fields[f]; ok {
				t.Errorf("%s: 普通用户 DTO 不得包含 %q", name, f)
			}
		}
		if _, ok := fields["rate_multiplier"]; !ok {
			t.Errorf("%s: 应仍返回 rate_multiplier", name)
		}
	}
}

// TestGroupFromServiceAdminIncludesProfitControl 钉死管理端仍能读写利润控制配置。
func TestGroupFromServiceAdminIncludesProfitControl(t *testing.T) {
	admin := GroupFromServiceAdmin(profitControlServiceGroup())
	if admin.ProfitControlEnabled != true || admin.ProfitMinMargin != 0.3 || admin.ProfitSafetyBuffer != 0.05 {
		t.Fatalf("管理员 DTO 未透传利润控制配置: %+v", admin)
	}
	fields := marshalToMap(t, admin)
	for _, f := range profitControlJSONFields {
		if _, ok := fields[f]; !ok {
			t.Errorf("管理员 DTO 应包含 %q", f)
		}
	}
}

// TestGroupFromServiceAdminIncludesCodexOverdraft 钉死管理端可读写 OpenAI 分组透支接力配置。
func TestGroupFromServiceAdminIncludesCodexOverdraft(t *testing.T) {
	admin := GroupFromServiceAdmin(profitControlServiceGroup())
	if admin.CodexOverdraftEnabled != true || admin.CodexOverdraftNextGroupID == nil || *admin.CodexOverdraftNextGroupID != 9 {
		t.Fatalf("管理员 DTO 未透传 Codex 透支接力配置: %+v", admin)
	}
	fields := marshalToMap(t, admin)
	for _, f := range codexOverdraftJSONFields {
		if _, ok := fields[f]; !ok {
			t.Errorf("管理员 DTO 应包含 %q", f)
		}
	}
}
