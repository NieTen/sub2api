// Package modeltrace 将 ModelTrace 的闭集数字指纹算法移植为标准库 Go 实现。
// 来源及许可见本目录的 来源说明.md 和 LICENSE.ModelTrace。
package modeltrace

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"unicode"
)

const (
	BankVersion = "df3a0f9d3e054c0dc02d6d586686db8daf8fa7c8"
	BankSHA256  = "1c2cb74d372f9f0f30d0dabbb7b7a838660d2f769a88d0c8489e4c662e088c21"
	Limitation  = "结果是候选库内的模型指纹相似度，不能证明实际模型身份或能力下降；未收录模型、系统提示和采样参数可能影响结果。"
	dimension   = 355
	alpha       = 0.5
)

//go:embed unified_bank.json
var bankJSON []byte

type Model struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Family      string `json:"family"`
	FamilyName  string `json:"family_name"`
}

type Output struct {
	Text          string `json:"text"`
	ExpectedCount int    `json:"expected_count"`
}

type Diagnostic struct {
	Index          int  `json:"index"`
	ParsedNumbers  int  `json:"parsed_numbers"`
	MinimumNumbers int  `json:"minimum_numbers"`
	Accepted       bool `json:"accepted"`
}

type ModelResult struct {
	Model                  string  `json:"model"`
	DisplayName            string  `json:"display_name"`
	Probability            float64 `json:"probability"`
	ProfileSimilarity      float64 `json:"profile_similarity"`
	Score                  float64 `json:"score"`
	NuisanceScore          float64 `json:"nuisance_score"`
	Family                 string  `json:"family"`
	FamilyName             string  `json:"family_name"`
	ConditionalProbability float64 `json:"conditional_probability"`
}

type FamilyResult struct {
	Family      string  `json:"family"`
	DisplayName string  `json:"display_name"`
	Probability float64 `json:"probability"`
}

type Calibration struct {
	Queries    string  `json:"queries"`
	Beta       float64 `json:"beta"`
	CVAccuracy float64 `json:"cv_accuracy"`
}

type Analysis struct {
	Prediction           string         `json:"prediction"`
	PredictionName       string         `json:"prediction_name"`
	Probability          float64        `json:"probability"`
	Margin               float64        `json:"margin"`
	ValidResponses       int            `json:"valid_responses"`
	Results              []ModelResult  `json:"results"`
	Diagnostics          []Diagnostic   `json:"diagnostics"`
	FamilyPrediction     string         `json:"family_prediction"`
	FamilyPredictionName string         `json:"family_prediction_name"`
	FamilyProbability    float64        `json:"family_probability"`
	FamilyProbabilities  []FamilyResult `json:"family_probabilities"`
	Calibration          Calibration    `json:"calibration"`
	Method               string         `json:"method"`
	BankVersion          string         `json:"bank_version"`
	Limitation           string         `json:"limitation"`
}

type bankModel struct {
	Model
	Counts []float64 `json:"counts"`
}

type featureArtifact struct {
	FeatureMean          []float64     `json:"feature_mean"`
	FeatureScale         []float64     `json:"feature_scale"`
	NuisanceBasis        [][]float64   `json:"nuisance_basis"`
	Centroids            [][]float64   `json:"centroids"`
	EnvironmentCentroids [][][]float64 `json:"environment_centroids"`
	Weight               float64       `json:"weight"`
}

type robustArtifact struct {
	ModelOrder    []string        `json:"model_order"`
	Hellinger     featureArtifact `json:"hellinger"`
	OrderedBlocks featureArtifact `json:"ordered_blocks"`
}

type fingerprintBank struct {
	Models      []bankModel            `json:"models"`
	Robust      robustArtifact         `json:"robust"`
	Calibration map[string]Calibration `json:"calibration"`
}

var (
	loadOnce     sync.Once
	builtinBank  fingerprintBank
	loadError    error
	digitPattern = regexp.MustCompile(`[0-9]+`)
)

func loadBank() (*fingerprintBank, error) {
	loadOnce.Do(func() {
		if err := json.Unmarshal(bankJSON, &builtinBank); err != nil {
			loadError = fmt.Errorf("读取模型指纹库失败: %w", err)
			return
		}
		loadError = validateBank(&builtinBank)
	})
	return &builtinBank, loadError
}

func validateBank(bank *fingerprintBank) error {
	if len(bank.Models) < 2 || len(bank.Robust.ModelOrder) != len(bank.Models) {
		return errors.New("模型指纹库模型数量无效")
	}
	for i, model := range bank.Models {
		if model.ID == "" || model.ID != bank.Robust.ModelOrder[i] || len(model.Counts) != dimension {
			return errors.New("模型指纹库模型顺序或频数无效")
		}
	}
	for _, entry := range []struct {
		artifact   featureArtifact
		dimensions int
	}{{bank.Robust.Hellinger, dimension}, {bank.Robust.OrderedBlocks, 74}} {
		a := entry.artifact
		if len(a.FeatureMean) != entry.dimensions || len(a.FeatureScale) != entry.dimensions || len(a.Centroids) != len(bank.Models) {
			return errors.New("模型指纹库特征维数无效")
		}
		for _, scale := range a.FeatureScale {
			if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
				return errors.New("模型指纹库特征尺度无效")
			}
		}
		vectors := append(append([][]float64{}, a.Centroids...), a.NuisanceBasis...)
		for _, environment := range a.EnvironmentCentroids {
			if len(environment) != len(bank.Models) {
				return errors.New("模型指纹库环境模型数量无效")
			}
			vectors = append(vectors, environment...)
		}
		for _, vector := range vectors {
			if len(vector) != entry.dimensions {
				return errors.New("模型指纹库向量维数无效")
			}
			for _, value := range vector {
				if math.IsNaN(value) || math.IsInf(value, 0) {
					return errors.New("模型指纹库包含无效数字")
				}
			}
		}
	}
	if len(bank.Robust.OrderedBlocks.EnvironmentCentroids) == 0 || bank.Robust.OrderedBlocks.Weight < 0 || bank.Robust.OrderedBlocks.Weight > 1 {
		return errors.New("模型指纹库有序特征无效")
	}
	for _, key := range []string{"1", "2", "3"} {
		calibration, ok := bank.Calibration[key]
		if !ok || calibration.Beta <= 0 {
			return errors.New("模型指纹库校准参数无效")
		}
	}
	return nil
}

// Models 返回独立副本，调用者不能修改内置指纹库。
func Models() []Model {
	bank, err := loadBank()
	if err != nil {
		return nil
	}
	models := make([]Model, len(bank.Models))
	for i, model := range bank.Models {
		models[i] = model.Model
	}
	return models
}

// ParseNumbers 保持上游的最长数字段规则；字母及汉字会分割数字段。
func ParseNumbers(text string) []int {
	var best, current []int
	previousEnd := 0
	for _, match := range digitPattern.FindAllStringIndex(text, -1) {
		for _, character := range text[previousEnd:match[0]] {
			if len(current) > 0 && unicode.IsLetter(character) {
				if len(current) > len(best) {
					best = current
				}
				current = nil
				break
			}
		}
		value, err := strconv.Atoi(text[match[0]:match[1]])
		if err == nil && value >= 1 && value <= dimension {
			current = append(current, value)
		}
		previousEnd = match[1]
	}
	if len(current) > len(best) {
		best = current
	}
	return best
}

func countNumbers(numbers []int) []float64 {
	counts := make([]float64, dimension)
	for _, n := range numbers {
		counts[n-1]++
	}
	return counts
}

func standardize(values []float64) []float64 {
	center := 0.0
	for _, value := range values {
		center += value
	}
	center /= float64(len(values))
	variance := 0.0
	for _, value := range values {
		variance += (value - center) * (value - center)
	}
	variance /= float64(len(values))
	scale := math.Max(math.Sqrt(variance), 1e-12)
	out := make([]float64, len(values))
	for i, value := range values {
		out[i] = (value - center) / scale
	}
	return out
}

func dot(left, right []float64) float64 {
	total := 0.0
	for i, value := range left {
		total += value * right[i]
	}
	return total
}

func normalize(values []float64) []float64 {
	scale := math.Max(math.Sqrt(dot(values, values)), 1e-12)
	out := make([]float64, len(values))
	for i, value := range values {
		out[i] = value / scale
	}
	return out
}

func subtractBasis(values []float64, basis [][]float64) []float64 {
	out := append([]float64(nil), values...)
	// 使用原始向量同时计算各投影，和 Python 矩阵表达式完全一致。
	for _, vector := range basis {
		projection := dot(values, vector)
		for i := range out {
			out[i] -= projection * vector[i]
		}
	}
	return out
}

func featureStandardize(values []float64, artifact featureArtifact) []float64 {
	out := make([]float64, len(values))
	for i, value := range values {
		out[i] = (value - artifact.FeatureMean[i]) / artifact.FeatureScale[i]
	}
	return out
}

func sqrtDistribution(counts []float64) []float64 {
	total := 0.0
	for _, value := range counts {
		total += value + alpha
	}
	out := make([]float64, len(counts))
	for i, value := range counts {
		out[i] = math.Sqrt((value + alpha) / total)
	}
	return out
}

func centerScores(feature []float64, centers [][]float64) []float64 {
	out := make([]float64, len(centers))
	for i, center := range centers {
		out[i] = dot(feature, center)
	}
	return out
}

func orderedFeature(numbers []int) []float64 {
	out := make([]float64, 0, 74)
	start := 0
	for part := 0; part < 4; part++ {
		size := len(numbers) / 4
		if part < len(numbers)%4 {
			size++
		}
		counts := make([]float64, 16)
		for _, number := range numbers[start : start+size] {
			index := int(math.Floor(float64(number-1) / 355 * 16))
			if index > 15 {
				index = 15
			}
			counts[index]++
		}
		out = append(out, sqrtDistribution(counts)...)
		start += size
	}
	lastDigits := make([]float64, 10)
	for _, number := range numbers {
		lastDigits[number%10]++
	}
	return append(out, sqrtDistribution(lastDigits)...)
}

func scoreNumbers(numbers []int, bank *fingerprintBank) ([]float64, []float64) {
	artifact := bank.Robust.Hellinger
	feature := featureStandardize(sqrtDistribution(countNumbers(numbers)), artifact)
	marginal := standardize(centerScores(normalize(subtractBasis(feature, artifact.NuisanceBasis)), artifact.Centroids))
	// 上游边际分数会再次标准化，保留该步骤用于数值一致性。
	fused := standardize(marginal)
	ordered := bank.Robust.OrderedBlocks
	if ordered.Weight == 0 {
		return fused, marginal
	}
	feature = featureStandardize(orderedFeature(numbers), ordered)
	unit := normalize(feature)
	template := make([]float64, len(bank.Models))
	for i := range template {
		template[i] = math.Inf(-1)
	}
	for _, environment := range ordered.EnvironmentCentroids {
		for i, value := range centerScores(unit, environment) {
			template[i] = math.Max(template[i], value)
		}
	}
	template = standardize(template)
	nuisance := standardize(centerScores(normalize(subtractBasis(feature, ordered.NuisanceBasis)), ordered.Centroids))
	for i := range template {
		template[i] = 0.5*template[i] + 0.5*nuisance[i]
	}
	template = standardize(template)
	for i := range fused {
		fused[i] = (1-ordered.Weight)*fused[i] + ordered.Weight*template[i]
	}
	return fused, marginal
}

func softmax(values []float64) []float64 {
	maximum := values[0]
	for _, value := range values {
		maximum = math.Max(maximum, value)
	}
	out := make([]float64, len(values))
	total := 0.0
	for i, value := range values {
		out[i] = math.Exp(value - maximum)
		total += out[i]
	}
	for i := range out {
		out[i] /= total
	}
	return out
}

func profileSimilarity(left, right []float64) float64 {
	leftTotal, rightTotal := 0.0, alpha*dimension
	for i := range left {
		leftTotal += left[i]
		rightTotal += right[i]
	}
	divergence := 0.0
	for i := range left {
		p := left[i] / leftTotal
		q := (right[i] + alpha) / rightTotal
		midpoint := (p + q) / 2
		if p > 0 {
			divergence += p * math.Log(p/midpoint) / 2
		}
		divergence += q * math.Log(q/midpoint) / 2
	}
	return 1 - math.Sqrt(math.Max(0, divergence)/math.Log(2))
}

// Analyze 完整执行闭集归因。无有效回答时返回错误，不生成模型不一致结论。
func Analyze(outputs []Output) (*Analysis, error) {
	bank, err := loadBank()
	if err != nil {
		return nil, err
	}
	analysis := &Analysis{Diagnostics: make([]Diagnostic, 0, len(outputs)), Method: "统一全局稳健数字指纹", BankVersion: BankVersion, Limitation: Limitation}
	combined := make([]float64, len(bank.Models))
	nuisance := make([]float64, len(bank.Models))
	pooled := make([]float64, dimension)
	for index, output := range outputs {
		numbers := ParseNumbers(output.Text)
		minimum := 80
		if output.ExpectedCount > 0 {
			minimum = max(minimum, int(math.Ceil(float64(output.ExpectedCount)*0.55)))
		}
		accepted := len(numbers) >= minimum
		analysis.Diagnostics = append(analysis.Diagnostics, Diagnostic{index, len(numbers), minimum, accepted})
		if !accepted {
			continue
		}
		scores, marginal := scoreNumbers(numbers, bank)
		for i := range combined {
			combined[i] += scores[i]
			nuisance[i] += marginal[i]
		}
		for i, value := range countNumbers(numbers) {
			pooled[i] += value
		}
		analysis.ValidResponses++
	}
	if analysis.ValidResponses == 0 {
		return nil, errors.New("没有可用回答：拒答、非数字或严重截断的回答不会计入模型指纹")
	}
	key := strconv.Itoa(min(analysis.ValidResponses, 3))
	analysis.Calibration = bank.Calibration[key]
	analysis.Calibration.Queries = key
	scaled := make([]float64, len(combined))
	for i := range combined {
		combined[i] /= float64(analysis.ValidResponses)
		nuisance[i] /= float64(analysis.ValidResponses)
		scaled[i] = combined[i] * analysis.Calibration.Beta
	}
	probabilities := softmax(scaled)
	familyIndexes := make(map[string]int)
	for i, model := range bank.Models {
		family := model.Family
		if family == "" {
			family = "models"
		}
		familyName := model.FamilyName
		if familyName == "" {
			familyName = family
		}
		familyIndex, ok := familyIndexes[family]
		if !ok {
			familyIndex = len(analysis.FamilyProbabilities)
			familyIndexes[family] = familyIndex
			analysis.FamilyProbabilities = append(analysis.FamilyProbabilities, FamilyResult{Family: family, DisplayName: familyName})
		}
		analysis.FamilyProbabilities[familyIndex].Probability += probabilities[i]
		analysis.Results = append(analysis.Results, ModelResult{Model: model.ID, DisplayName: model.DisplayName, Probability: probabilities[i], ProfileSimilarity: profileSimilarity(pooled, model.Counts), Score: combined[i], NuisanceScore: nuisance[i], Family: family, FamilyName: familyName})
	}
	for i := range analysis.Results {
		result := &analysis.Results[i]
		total := analysis.FamilyProbabilities[familyIndexes[result.Family]].Probability
		if total > 0 {
			result.ConditionalProbability = result.Probability / total
		}
	}
	sort.SliceStable(analysis.Results, func(i, j int) bool { return analysis.Results[i].Probability > analysis.Results[j].Probability })
	top := analysis.Results[0]
	analysis.Prediction = top.Model
	analysis.PredictionName = top.DisplayName
	analysis.Probability = top.Probability
	analysis.Margin = top.Probability - analysis.Results[1].Probability
	for i, family := range analysis.FamilyProbabilities {
		if i == 0 || family.Probability > analysis.FamilyProbability {
			analysis.FamilyPrediction = family.Family
			analysis.FamilyPredictionName = family.DisplayName
			analysis.FamilyProbability = family.Probability
		}
	}
	return analysis, nil
}
