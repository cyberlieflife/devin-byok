package localapi

import (
	"encoding/json"
	"testing"

	"devin-byok/internal/config"
	"devin-byok/internal/paths"
	"devin-byok/internal/pbwire"
)

func TestVerifyCommandModelConfigsForUserConfig(t *testing.T) {
	cfg, err := config.Load(paths.FindConfig())
	if err != nil {
		t.Skipf("no user config: %v", err)
	}
	entries := cfg.ModelList()
	if len(entries) == 0 {
		t.Fatalf("ModelList empty — model entries missing from user config")
	}
	resp := buildGetCommandModelConfigsResponse(cfg)
	fields := pbwire.ParseFields(resp)
	count := 0
	var labels, uids []string
	for _, f := range fields {
		if f.Number != 1 {
			continue
		}
		count++
		sub := pbwire.ParseFields(f.Bytes)
		for _, sf := range sub {
			switch sf.Number {
			case 1:
				labels = append(labels, string(sf.Bytes))
			case 22:
				uids = append(uids, string(sf.Bytes))
			}
		}
	}
	t.Logf("resp bytes=%d entries=%d configModels=%d labels=%v uids=%v", len(resp), count, len(entries), labels, uids)
	if count != len(entries) {
		t.Fatalf("stub returned %d configs, want %d", count, len(entries))
	}
	if len(uids) == 0 {
		t.Fatal("no model_uid field 22 in any entry")
	}
	reg := buildGetAllAcpRegistriesResponse()
	fields = pbwire.ParseFields(reg)
	if len(fields) == 0 || fields[0].Number != 1 || len(fields[0].Bytes) == 0 {
		t.Fatal("acp registry stub has no registry_json payload")
	}
	var registry struct {
		Agents []struct {
			ID           string `json:"id"`
			Distribution struct {
				Binary map[string]struct {
					Archive string `json:"archive"`
					Cmd     string `json:"cmd"`
				} `json:"binary"`
			} `json:"distribution"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(fields[0].Bytes, &registry); err != nil {
		t.Fatalf("registry json invalid: %v", err)
	}
	if len(registry.Agents) != 1 || registry.Agents[0].ID != "devin-cli" {
		t.Fatalf("registry agents wrong: %+v", registry.Agents)
	}
	a := registry.Agents[0]
	if len(a.Distribution.Binary) != 6 || a.Distribution.Binary["darwin-aarch64"].Cmd != "devin" || a.Distribution.Binary["windows-x86_64"].Cmd != "devin.exe" {
		t.Fatalf("binary distribution wrong: %+v", a.Distribution.Binary)
	}
	t.Logf("registry agents=%d ok", len(registry.Agents))
}
