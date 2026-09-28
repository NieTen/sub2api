// Package modelquality 提供可复现、可自动判分的轻量能力基准。
// 分数只代表本题组表现，不能用作智商、模型身份或综合能力认证。
package modelquality

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
)

const Version = "modelquality-v1"

type Challenge struct {
	Prompt   string            `json:"prompt"`
	Expected map[string]string `json:"expected"`
	Version  string            `json:"version"`
}

type Item struct {
	ID       string `json:"id"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Correct  bool   `json:"correct"`
}

type Result struct {
	Score   float64 `json:"score"`
	Passed  int     `json:"passed"`
	Total   int     `json:"total"`
	Items   []Item  `json:"items"`
	Version string  `json:"version"`
}

// Generate 保持十种题型和难度范围不变，每次随机更换操作数及文本。
func Generate() Challenge { return generate(rand.IntN) }

func generate(draw func(int) int) Challenge {
	challenge := Challenge{Version: Version, Expected: make(map[string]string, 10)}
	questions := make([]string, 0, 10)
	add := func(id, question, answer string) {
		questions = append(questions, id+". "+question)
		challenge.Expected[id] = answer
	}
	integer := strconv.Itoa
	a, b, c := 17+draw(63), 23+draw(58), 31+draw(180)
	add("q1", fmt.Sprintf("计算 (%d × %d) − %d，只回答整数。", a, b, c), integer(a*b-c))
	n, divisor := 1000+draw(8000), 7+draw(17)
	add("q2", fmt.Sprintf("%d 除以 %d 的非负余数是多少？只回答整数。", n, divisor), integer(n%divisor))
	first, multiplier, increment := 2+draw(8), 2+draw(3), 1+draw(7)
	sequence := []int{first}
	for len(sequence) < 6 {
		sequence = append(sequence, sequence[len(sequence)-1]*multiplier+increment)
	}
	add("q3", fmt.Sprintf("数列首项为 %d，此后每一项等于前一项乘 %d 再加 %d。求第 6 项，只回答整数。", first, multiplier, increment), integer(sequence[5]))
	total := 5 + draw(3)
	arrangements := factorial(total) - 2*factorial(total-1)
	add("q4", fmt.Sprintf("%d 名不同的人排成一行，其中指定的甲、乙两人不能相邻，共有多少种排列？只回答整数。", total), integer(arrangements))
	logicQuestions := []string{
		"所有赤类物体都属于圆类；没有圆类物体属于蓝类；有些木类物体属于赤类。能否必然推出：有些木类物体不属于蓝类？只回答“是”或“否”。",
		"所有赤类物体都属于圆类；有些圆类物体属于木类。能否必然推出：有些赤类物体属于木类？只回答“是”或“否”。",
		"如果开关 A 打开，则灯 B 亮；如果灯 B 亮，则灯 C 灭。已知灯 C 亮。能否必然推出：开关 A 没有打开？只回答“是”或“否”。",
		"如果开关 A 打开，则灯 B 亮；已知灯 B 亮。能否必然推出：开关 A 打开了？只回答“是”或“否”。",
	}
	logic := draw(len(logicQuestions))
	answer := "否"
	if logic%2 == 0 {
		answer = "是"
	}
	add("q5", logicQuestions[logic], answer)
	words := []string{"amber", "birch", "cedar", "delta", "ember", "frost", "grape", "haven", "ivory", "jolly", "koala", "lemon"}
	for i := len(words) - 1; i > 0; i-- {
		j := draw(i + 1)
		words[i], words[j] = words[j], words[i]
	}
	words = words[:6]
	add("q6", "对下面列表执行指令：按从 1 开始的位置编号，仅保留偶数位置的单词，然后将保留单词的顺序反转，最后用短横线连接，不加空格。列表："+strings.Join(words, ", ")+"。", strings.Join([]string{words[5], words[3], words[1]}, "-"))
	n1, d1, n2, d2 := 1+draw(9), 3+draw(10), 1+draw(9), 3+draw(10)
	add("q7", fmt.Sprintf("计算 %d/%d + %d/%d，以最简分数 n/d 输出（即使分母为 1 也保留 /1）。", n1, d1, n2, d2), fraction(n1*d2+n2*d1, d1*d2))
	limit, initial := 8+draw(8), 3+draw(15)
	value := initial
	for i := 1; i <= limit; i++ {
		if i%3 == 0 {
			value -= i
		} else {
			value += 2 * i
		}
	}
	add("q8", fmt.Sprintf("伪代码：s = %d；对 i 从 1 到 %d（含端点）依次执行：若 i 能被 3 整除则 s = s − i，否则 s = s + 2 × i。循环结束的 s 是多少？只回答整数。", initial, limit), integer(value))
	upper, multipleA, multipleB := 80+draw(120), 3+draw(4), 7+draw(5)
	count := upper/multipleA + upper/multipleB - upper/(multipleA/gcd(multipleA, multipleB)*multipleB)
	add("q9", fmt.Sprintf("从 1 到 %d（含端点）的整数中，能被 %d 或 %d 整除的整数共有多少个？同时满足两项只计一次。只回答整数。", upper, multipleA, multipleB), integer(count))
	red, blue := 3+draw(6), 3+draw(6)
	add("q10", fmt.Sprintf("袋中有 %d 个红球和 %d 个蓝球，每球被抽到机会相同。不放回地连续抽两个球，两个球都是红球的概率是多少？以最简分数 n/d 输出。", red, blue), fraction(red*(red-1), (red+blue)*(red+blue-1)))
	challenge.Prompt = "请独立完成以下 10 道能力基准题，不使用外部工具。只输出一个合法 JSON 对象，键严格为 q1 到 q10，所有值均为字符串，不附解释或 Markdown。每题答案遵守该题格式。\n" + strings.Join(questions, "\n")
	return challenge
}

func factorial(n int) int {
	value := 1
	for i := 2; i <= n; i++ {
		value *= i
	}
	return value
}
func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
func fraction(n, d int) string {
	divisor := gcd(n, d)
	return strconv.Itoa(n/divisor) + "/" + strconv.Itoa(d/divisor)
}

// Evaluate 严格校验完整答卷；传输残缺及格式无效返回错误，不混入能力低分。
func Evaluate(challenge Challenge, text string) (Result, error) {
	if challenge.Version != Version || len(challenge.Expected) != 10 {
		return Result{}, errors.New("能力基准版本或题目数量无效")
	}
	for i := 1; i <= 10; i++ {
		if _, ok := challenge.Expected[fmt.Sprintf("q%d", i)]; !ok {
			return Result{}, errors.New("能力基准缺少标准答案")
		}
	}
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```json\n") && strings.HasSuffix(text, "```") {
		text = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(text, "```json\n"), "```"))
	}
	if strings.HasPrefix(text, "```\n") && strings.HasSuffix(text, "```") {
		text = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(text, "```\n"), "```"))
	}
	decoder := json.NewDecoder(bytes.NewBufferString(text))
	decoder.UseNumber()
	// 手工读取对象以拒绝重复题号；encoding/json 默认会静默覆盖重复键。
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return Result{}, errors.New("能力答卷不是完整 JSON 对象")
	}
	answers := make(map[string]string, 10)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return Result{}, errors.New("能力答卷 JSON 格式无效")
		}
		key, ok := keyToken.(string)
		if !ok {
			return Result{}, errors.New("能力答卷题号格式无效")
		}
		if _, ok := challenge.Expected[key]; !ok {
			return Result{}, fmt.Errorf("能力答卷包含未知题号 %s", key)
		}
		if _, ok := answers[key]; ok {
			return Result{}, fmt.Errorf("能力答卷包含重复题号 %s", key)
		}
		var answer any
		if err := decoder.Decode(&answer); err != nil {
			return Result{}, errors.New("能力答卷 JSON 格式无效")
		}
		switch value := answer.(type) {
		case string:
			answers[key] = strings.TrimSpace(value)
		case json.Number:
			answers[key] = value.String()
		default:
			return Result{}, fmt.Errorf("能力答卷 %s 的答案必须是字符串或数字", key)
		}
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return Result{}, errors.New("能力答卷 JSON 不完整")
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return Result{}, errors.New("能力答卷包含额外内容")
	}
	if len(answers) != len(challenge.Expected) {
		return Result{}, errors.New("能力答卷缺少题目答案，无法计算完整基准得分")
	}
	result := Result{Version: Version, Total: len(challenge.Expected), Items: make([]Item, 0, len(challenge.Expected))}
	keys := make([]string, 0, len(challenge.Expected))
	for key := range challenge.Expected {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, _ := strconv.Atoi(strings.TrimPrefix(keys[i], "q"))
		right, _ := strconv.Atoi(strings.TrimPrefix(keys[j], "q"))
		return left < right
	})
	for _, key := range keys {
		expected := challenge.Expected[key]
		actual := answers[key]
		correct := actual == expected
		if correct {
			result.Passed++
		}
		result.Items = append(result.Items, Item{ID: key, Expected: expected, Actual: actual, Correct: correct})
	}
	result.Score = float64(result.Passed) * 100 / float64(result.Total)
	return result, nil
}
