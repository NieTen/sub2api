package modeltrace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type parityFixture struct {
	SourceCommit string       `json:"source_commit"`
	Cases        []parityCase `json:"cases"`
}

type parityCase struct {
	Model    string     `json:"model"`
	Outputs  []Output   `json:"outputs"`
	Expected []Analysis `json:"expected"`
}

func TestUpstreamParity(t *testing.T) {
	data, err := os.ReadFile("testdata/parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture parityFixture
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SourceCommit != BankVersion || len(fixture.Cases) != 16 {
		t.Fatal("离线参考来源或模型数量不正确")
	}
	for _, test := range fixture.Cases {
		for index, expected := range test.Expected {
			t.Run(test.Model+"/"+strconv.Itoa(index+1), func(t *testing.T) {
				actual, err := Analyze(test.Outputs[:index+1])
				if err != nil {
					t.Fatal(err)
				}
				if actual.Prediction != expected.Prediction || actual.PredictionName != expected.PredictionName || actual.ValidResponses != expected.ValidResponses || actual.FamilyPrediction != expected.FamilyPrediction {
					t.Fatalf("归因结论不同: %+v", actual)
				}
				closeFloat(t, "probability", actual.Probability, expected.Probability)
				closeFloat(t, "margin", actual.Margin, expected.Margin)
				closeFloat(t, "family probability", actual.FamilyProbability, expected.FamilyProbability)
				if !reflect.DeepEqual(actual.Diagnostics, expected.Diagnostics) {
					t.Fatalf("有效回答诊断不同: %+v / %+v", actual.Diagnostics, expected.Diagnostics)
				}
				if len(actual.Results) != len(expected.Results) {
					t.Fatal("候选数量不同")
				}
				for i, result := range actual.Results {
					want := expected.Results[i]
					if result.Model != want.Model || result.Family != want.Family || result.DisplayName != want.DisplayName || result.FamilyName != want.FamilyName {
						t.Fatalf("候选顺序或名称不同: %+v / %+v", result, want)
					}
					closeFloat(t, result.Model+" probability", result.Probability, want.Probability)
					closeFloat(t, result.Model+" score", result.Score, want.Score)
					closeFloat(t, result.Model+" profile similarity", result.ProfileSimilarity, want.ProfileSimilarity)
					closeFloat(t, result.Model+" conditional probability", result.ConditionalProbability, want.ConditionalProbability)
				}
				if len(actual.FamilyProbabilities) != len(expected.FamilyProbabilities) {
					t.Fatal("家族数量不同")
				}
				for i, family := range actual.FamilyProbabilities {
					want := expected.FamilyProbabilities[i]
					if family.Family != want.Family || family.DisplayName != want.DisplayName {
						t.Fatal("家族顺序不同")
					}
					closeFloat(t, "family weight", family.Probability, want.Probability)
				}
				if actual.Calibration != expected.Calibration {
					t.Fatalf("校准参数不同: %+v / %+v", actual.Calibration, expected.Calibration)
				}
			})
		}
	}
}

func closeFloat(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got-want) > 1e-10 {
		t.Errorf("%s: 得到 %.16g，期望 %.16g，差值 %.16g", name, got, want, got-want)
	}
}

func TestEmbeddedBankAndModelIsolation(t *testing.T) {
	digest := sha256.Sum256([]byte(strings.ReplaceAll(string(bankJSON), "\r\n", "\n")))
	if hex.EncodeToString(digest[:]) != BankSHA256 {
		t.Fatal("嵌入指纹库与上游数据哈希不同")
	}
	bank, err := loadBank()
	if err != nil {
		t.Fatal(err)
	}
	if len(bank.Models) != 16 {
		t.Fatal("应包含全部 16 个候选")
	}
	models := Models()
	models[0].ID = "changed"
	if Models()[0].ID == "changed" {
		t.Fatal("调用者修改了全局模型库")
	}
}

func TestParseNumbers(t *testing.T) {
	tests := []struct {
		input string
		want  []int
	}{
		{"说明123abc 1,2,3,4 end 7 8", []int{1, 2, 3, 4}},
		{"1,2 中文 3,4", []int{1, 2}},
		{"0,1,355,356,-2,1.5", []int{1, 355, 2, 1, 5}},
		{"1,9999999999999999999999999999999999,2", []int{1, 2}},
		{"拒绝回答", nil},
	}
	for _, test := range tests {
		if got := ParseNumbers(test.input); !reflect.DeepEqual(got, test.want) {
			t.Errorf("%q: 得到 %v，期望 %v", test.input, got, test.want)
		}
	}
}

func TestValidityAndCalibration(t *testing.T) {
	if _, err := Analyze(nil); err == nil {
		t.Fatal("没有回答应返回错误")
	}
	if _, err := Analyze([]Output{{Text: strings.Repeat("1 ", 79)}}); err == nil {
		t.Fatal("不足 80 项应返回错误")
	}
	if _, err := Analyze([]Output{{Text: strings.Repeat("1 ", 164), ExpectedCount: 300}}); err == nil {
		t.Fatal("不足目标 55% 应返回错误")
	}
	outputs := []Output{{Text: "无法完成"}, {Text: strings.Repeat("17 ", 165), ExpectedCount: 300}, {Text: strings.Repeat("23 ", 80)}}
	result, err := Analyze(outputs)
	if err != nil {
		t.Fatal(err)
	}
	if result.ValidResponses != 2 || result.Calibration.Queries != "2" || result.Diagnostics[0].Accepted || !result.Diagnostics[1].Accepted {
		t.Fatalf("有效回答筛选错误: %+v", result)
	}
	total := 0.0
	for _, item := range result.Results {
		total += item.Probability
	}
	closeFloat(t, "probability sum", total, 1)
	more := append(outputs, outputs[1], outputs[2])
	result, err = Analyze(more)
	if err != nil {
		t.Fatal(err)
	}
	if result.ValidResponses != 4 || result.Calibration.Queries != "3" {
		t.Fatal("三份以上应使用三份校准")
	}
}

func TestChallenges(t *testing.T) {
	for round := 0; round < 20; round++ {
		challenges := GenerateChallenges()
		if len(challenges) != 3 {
			t.Fatal("每轮应为三条挑战")
		}
		seen := map[int]bool{}
		for _, challenge := range challenges {
			if challenge.ExpectedCount < 292 || challenge.ExpectedCount > 332 || seen[challenge.ExpectedCount] {
				t.Fatal("挑战长度应位于范围内且不同")
			}
			seen[challenge.ExpectedCount] = true
			for _, piece := range []string{strconv.Itoa(challenge.ExpectedCount), "1 到 355", "禁止调用或借助任何工具", "不要连续递增或递减", "直接从第一个取值开始输出"} {
				if !strings.Contains(challenge.Prompt, piece) {
					t.Fatalf("挑战缺少必要约束 %s", piece)
				}
			}
		}
	}
}
