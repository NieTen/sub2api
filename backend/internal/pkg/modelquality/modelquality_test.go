package modelquality

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestGeneratedAnswersAgainstKnownProblems(t *testing.T) {
	// 固定选择每个范围的下界，手工验算十道题，避免测试照抄判分实现。
	challenge := generate(func(int) int { return 0 })
	want := map[string]string{"q1": "360", "q2": "6", "q3": "95", "q4": "72", "q5": "是", "q6": "grape-ember-cedar", "q7": "2/3", "q8": "48", "q9": "34", "q10": "1/5"}
	if !reflect.DeepEqual(challenge.Expected, want) {
		t.Fatalf("标准答案错误: %+v", challenge.Expected)
	}
	for _, phrase := range []string{"(17 × 23) − 31", "1000 除以 7", "首项为 2", "5 名不同的人", "列表：birch, cedar, delta, ember, frost, grape", "1/3 + 1/3", "s = 3", "从 1 到 80", "3 个红球和 3 个蓝球"} {
		if !strings.Contains(challenge.Prompt, phrase) {
			t.Errorf("题目与标准答案不对应: %s", phrase)
		}
	}
}

func TestScoringAndItems(t *testing.T) {
	challenge := generate(func(int) int { return 0 })
	answers := map[string]string{}
	for key, value := range challenge.Expected {
		answers[key] = value
	}
	answers["q2"] = "5"
	answers["q5"] = "否"
	data, _ := json.Marshal(answers)
	result, err := Evaluate(challenge, string(data))
	if err != nil {
		t.Fatal(err)
	}
	if result.Score != 80 || result.Passed != 8 || result.Total != 10 || result.Version != Version || len(result.Items) != 10 {
		t.Fatalf("判分错误: %+v", result)
	}
	if result.Items[1].ID != "q2" || result.Items[1].Actual != "5" || result.Items[1].Expected != "6" || result.Items[1].Correct || result.Items[9].ID != "q10" {
		t.Fatalf("题目诊断或排序错误: %+v", result.Items)
	}
	data, _ = json.Marshal(challenge.Expected)
	result, err = Evaluate(challenge, "```json\n"+string(data)+"\n```")
	if err != nil || result.Score != 100 {
		t.Fatalf("完整代码块答卷判分失败: %+v %v", result, err)
	}
}

func TestMalformedAnswersAreErrors(t *testing.T) {
	challenge := generate(func(int) int { return 0 })
	data, _ := json.Marshal(challenge.Expected)
	valid := string(data)
	tests := []string{"", "不是JSON", "null", "[]", "{}", valid[:len(valid)-1], valid + " 解释", valid + valid, strings.Replace(valid, `"q1":"360"`, `"q1":null`, 1), strings.Replace(valid, `"q1":"360"`, `"q1":[]`, 1), strings.Replace(valid, `"q1":"360"`, `"q1":"360","q1":"0"`, 1), strings.Replace(valid, `"q1":"360"`, `"unknown":"360"`, 1), strings.Replace(valid, `"q1":"360",`, "", 1)}
	for _, text := range tests {
		result, err := Evaluate(challenge, text)
		if err == nil {
			t.Errorf("不完整答卷不应产生分数: %q => %+v", text, result)
		}
		if result.Total != 0 || result.Items != nil {
			t.Errorf("错误答卷产生了部分分数: %+v", result)
		}
	}
	challenge.Version = "other"
	if _, err := Evaluate(challenge, valid); err == nil {
		t.Fatal("应拒绝未知题库版本")
	}
}

func TestDynamicQuestions(t *testing.T) {
	first := Generate()
	different := false
	for i := 0; i < 20; i++ {
		challenge := Generate()
		if len(challenge.Expected) != 10 || challenge.Version != Version {
			t.Fatal("题库必须保持十题及版本")
		}
		different = different || challenge.Prompt != first.Prompt
		data, _ := json.Marshal(challenge.Expected)
		result, err := Evaluate(challenge, string(data))
		if err != nil || result.Score != 100 {
			t.Fatalf("生成的题组无法评估: %v", err)
		}
	}
	if !different {
		t.Fatal("题目参数未随机变化")
	}
}
