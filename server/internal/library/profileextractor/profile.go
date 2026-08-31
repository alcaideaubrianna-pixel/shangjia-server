package profileextractor

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Fields contains values extracted from a source profile. Zero values mean unknown.
type Fields struct {
	Age    int
	Height int
	Weight int
	Cup    string
}

type Analysis struct {
	Fields
	HeightMentioned     bool
	WeightMentioned     bool
	CupMentioned        bool
	HeightSourceEmpty   bool
	WeightSourceEmpty   bool
	CupSourceEmpty      bool
	HeightSourceInvalid bool
	WeightSourceInvalid bool
	CupSourceInvalid    bool
}

// Merge fills only missing source values and keeps the source as authoritative when present.
func Merge(text string, height int, weight int, cup string) Fields {
	parsed := Parse(text)
	if height > 0 {
		parsed.Height = height
	}
	if weight > 0 {
		parsed.Weight = weight
	}
	if normalized := NormalizeCup(cup); normalized != "" {
		parsed.Cup = normalized
	}
	return parsed
}

// Refresh replaces fields explicitly present in new text and preserves known
// values when the new text does not mention that field.
func Refresh(text string, current Fields) Fields {
	parsed := Parse(text)
	if parsed.Age == 0 {
		parsed.Age = current.Age
	}
	if parsed.Height == 0 {
		parsed.Height = current.Height
	}
	if parsed.Weight == 0 {
		parsed.Weight = current.Weight
	}
	if parsed.Cup == "" {
		parsed.Cup = NormalizeCup(current.Cup)
	}
	return parsed
}

// NormalizeCup accepts a structured cup value without allowing generic tags
// to leak into the cup_size column.
func NormalizeCup(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.TrimSuffix(value, "罩杯")
	value = strings.TrimSuffix(value, "CUP")
	value = strings.TrimSpace(value)
	if len(value) != 1 || value[0] < 'A' || value[0] > 'H' {
		return ""
	}
	return value
}

var (
	agePattern            = regexp.MustCompile(`(?i)(?:年龄|age)(?:\s*:)+\s*\[?\s*(1[8-9]|[2-7][0-9]|80)(?:\s*(?:岁|周岁|虚岁))?`)
	birthYearAgePattern   = regexp.MustCompile(`(?i)(?:年龄|age)(?:\s*:)+\s*\[?\s*(0[0-9])(?:\s*年)?\s*\]?`)
	standaloneAge         = regexp.MustCompile(`(?:^|[^0-9])(1[8-9]|[2-7][0-9]|80)\s*(?:岁|周岁|虚岁)(?:[^0-9]|$)`)
	heightPattern         = regexp.MustCompile(`(?i)(?:身高|净身高|升高|身长|高度|height|高)(?:\s*\([^)]*\))?(?:\s*:)*\s*\[?\s*(?:姐姐|妹妹|大姐|小妹)?\s*(?:净身|净|裸|裸足|光脚|脱鞋|不穿鞋|穿高跟|穿鞋|快|真实|q)?\s*\.?\s*([0-9]+(?:\.[0-9]+)?\s*(?:cm|厘米|米|m)?)`)
	weightPattern         = regexp.MustCompile(`(?i)(?:体重|净体重|重量|斤数|weight|重)(?:\s*\([^)]*\))?(?:\s*:)*\s*[,]?\s*\[?\s*(?:姐姐|妹妹|大姐|小妹)?\s*(?:bbw\s*[,，]?\s*|现(?:在)?\s*|目前\s*:*\s*|不到\s*|快\s*|无虚假\s*["“”']*\s*)?([0-9]+(?:\.[0-9]+)?\s*(?:kg|公斤|千克|斤)?)`)
	reversedHeight        = regexp.MustCompile(`(?i)(1[4-9][0-9]|20[0-9])\s*(?:cm|厘米)?\s*(?:净身高|净高|身高|高)`)
	weightPrefixedIndex   = regexp.MustCompile(`(?i)(?:体重|weight)\s*[0-9]\s+([5-9][0-9]|1[0-9]{2})`)
	chineseHeight         = regexp.MustCompile(`(?:身高|净身高)\s*:\s*([一二])米([零一二三四五六七八九])([零一二三四五六七八九])`)
	chineseHeightShort    = regexp.MustCompile(`(?:身高|净身高)\s*:\s*([一二])米([四五六七八九])(?:\s|$)`)
	chineseWeight         = regexp.MustCompile(`(?:体重|净体重)\s*:\s*([五六七八九])十几斤`)
	slashHeight           = regexp.MustCompile(`(?i)(?:身高|height)\s*:*\s*/\s*(1[4-9][0-9]|20[0-9])`)
	slashWeight           = regexp.MustCompile(`(?i)(?:体重|weight)\s*:*\s*/\s*([5-9][0-9]|1[0-9]{2})`)
	slashCup              = regexp.MustCompile(`(?i)(?:罩杯|胸围|bust)\s*:*\s*/\s*([a-h])\+?`)
	heightAfterYear       = regexp.MustCompile(`(?:高|身高)\s*[0-9]{2}\s*(1[4-9][0-9]|20[0-9])`)
	standaloneWeightKg    = regexp.MustCompile(`(?:^|\s)([3-9][0-9](?:\.[0-9]+)?)\s*(?:kg|公斤|千克)(?:\s|$|[^a-z])`)
	heightInvalidText     = regexp.MustCompile(`(?i)(?:身高|净身高|height)\s*:\s*兔子(?:\s|$)`)
	cupPattern            = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|罩|杯|cup|胸围|胸部|上围|胸|🐻)(?:\s*:)*\s*[^a-z\n;]{0,12}?([a-h])\+?`)
	crossCup              = regexp.MustCompile(`(?i)(?:^|[^0-9])([0-9]{2,3})\s*([a-h])\+?([^a-z]|$)`)
	compactFields         = regexp.MustCompile(`(?:^|[^0-9])(1[8-9]|[2-7][0-9]|80)\s*(?:岁|周岁|虚岁)?\s*[/,\s-]+(1[4-9][0-9]|20[0-9])\s*[/,\s-]+([0-9]{2,3})(?:[^0-9]|$)`)
	compactHeightCup      = regexp.MustCompile(`(?i)(?:^|[^0-9])(1[4-9][0-9]|20[0-9])\s*([a-h])\+?(?:[^a-z]|$)`)
	heightLabel           = regexp.MustCompile(`(?i)(?:身高|净身高|升高|身长|高度|height)\s*(?:\([^)]*\))?\s*:`)
	weightLabel           = regexp.MustCompile(`(?i)(?:体重|净体重|重量|斤数|weight)\s*(?:\([^)]*\))?\s*:`)
	cupLabel              = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围|🐻)\s*:`)
	heightEmpty           = regexp.MustCompile(`(?i)(?:身高|净身高|升高|身长|高度|height)\s*(?:\([^)]*\))?\s*:\s*(?:$|\n|体重|罩杯|职业|是否)`)
	weightEmpty           = regexp.MustCompile(`(?i)(?:体重|净体重|重量|斤数|weight)\s*(?:\([^)]*\))?\s*:\s*(?:$|\n|罩杯|职业|是否)`)
	cupEmpty              = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)\s*:\s*(?:$|\n|职业|学历|是否)`)
	fieldShifted          = regexp.MustCompile(`(?i)身高\s*:\s*([a-h])\+?\s+体重\s*:\s*(1[4-9][0-9]|20[0-9])\s+罩杯\s*:\s*([5-9][0-9]|1[0-9]{2})`)
	fieldShiftedReverse   = regexp.MustCompile(`(?i)罩杯\s*:\s*(1[4-9][0-9]|20[0-9])\s+身高\s*:\s*([5-9][0-9]|1[0-9]{2})\s+体重\s*:\s*(?:小\s*)?([a-h])\+?`)
	heightRaw             = regexp.MustCompile(`(?i)(?:身高|净身高|升高|身长|高度|height)\s*(?:\([^)]*\))?\s*:\s*([0-9]+(?:\.[0-9]+)?)`)
	weightRaw             = regexp.MustCompile(`(?i)(?:体重|净体重|重量|斤数|weight)\s*(?:\([^)]*\))?\s*:\s*([0-9]+(?:\.[0-9]+)?)`)
	weightCupValue        = regexp.MustCompile(`(?i)(?:体重|净体重|重量|斤数|weight)\s*(?:\([^)]*\))?\s*:\s*[a-h](?:\s|$)`)
	weightExplicitJin     = regexp.MustCompile(`(?i)(?:体重|净体重|重量|斤数|weight)\s*:*\s*([0-9]+(?:\.[0-9]+)?)\s*斤`)
	weightInvalidText     = regexp.MustCompile(`(?i)(?:体重|净体重|重量|斤数|weight)\s*:\s*图(?:\s|$)`)
	cupUnknown            = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)\s*:*\s*·?\s*(?:(?:具体|这个|我也)?不知道(?:怎么回答|[^\s]{0,24})?|根据体重买的\s*我不知道|不晓得|不太清楚[啊哎唉]?|不清楚(?:[啊哎唉的]|啊老铁)?|不确定|不知|未知|不懂(?:没测过)?|听不懂|忘了|(?:我)?没量过(?:,[^\s]{0,16})?|(?:我)?没量|没有量|没有测量过(?:,[^\s]{0,16})?|(?:我)?没测过|没问|无胸?|否|不是|男|正常|一般的?|反正不小|一只手|大概一个苹果大(?:一|雪生)点|是男娘|伪娘(?:\([^)]*\))?|实习生?|学生|雪生|出|胸\s*[0-9]{1,3}\s*腰\s*[0-9]{1,3}\s*臀\s*[0-9]{1,3}|定制|x|[-+?？]|\.{2,}|·\s*[0-9]{0,3}|[0-9]{1,3}(?:(?:\s*[/.-]\s*)[0-9]{1,3})*(?:\s*(?:\(胸围\)|罩|嘟|\+))?)[.,。]?(?:\s|$)`)
	cupHuge               = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)(?:\s*:)*\s*(?:最大|巨|超大|特大)`)
	cupSmall              = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)(?:\s*:)*\s*(?:最小|小(?:\s|$)|胸小|平|飞机场|贫乳)`)
	cupOutOfRange         = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)(?:\s*:)*\s*[i-z](?:杯|[^a-z]|$)`)
	cupNumericInvalid     = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)(?:\s*:)*\s*[0-9]{1,3}[i-z](?:\s|$)`)
	cupTypoB              = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)(?:\s*:)*\s*[吧把]\+?(?:\s|$)`)
	cupVariable           = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)(?:\s*:)*\s*有浮动(?:\s|$)`)
	cupNegative           = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)(?:\s*:)*\s*不(?:\s|$)`)
	cupMeasurements       = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)(?:\s*:)*\s*(?:\(三围\))?\s*[0-9]{2,3}\s+[0-9]{2,3}\s+[0-9]{2,3}(?:\s|$)`)
	cupTemporarilyUnknown = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)(?:\s*:)*\s*暂时不知(?:\s|$)`)
	cupNumericGuess       = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)(?:\s*:)*\s*[0-9]{1,3}吧(?:\s|$)`)
	cupNormalRange        = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)(?:\s*:)*\s*正常范围(?:\s|$)`)
	cupUncertainText      = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)(?:\s*:)*\s*(?:不确定但是不大|不确定\(自我感觉不小\))(?:\s|$)`)
	cupRelativeSize       = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)(?:\s*:)*\s*(?:不大|不小|不大不小|很大肉眼可见的大|比鸡蛋大出一圈|一个手差不多兜得住|不是飞机\s*不大|没测过反正不是平的|没测过,偏小|不清楚略小)(?:\s|$)`)
	cupUnknownDecorated   = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)(?:\s*:)*\s*(?:·\s*没量过|小\s*\(具体不知道\))(?:\s|$)`)
	cupUnknownTypo        = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)(?:\s*:)*\s*不知豆(?:\s|$)`)
	cupUnfamiliar         = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)(?:\s*:)*\s*不了解(?:\s|$)`)
	cupUnknownVerbose     = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)(?:\s*:)*\s*这个不太清楚啊,微胖(?:\s|$)`)
	cupNumericUnit        = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)(?:\s*:)*\s*[0-9]{1,3}(?:kg|公斤|斤)(?:\s|$)`)
	cupNumericRange       = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围)(?:\s*:)*\s*[0-9]{1,3}\s*[~～]\s*[0-9]{1,3}(?:\s|$)`)
	cupNicknameValue      = regexp.MustCompile(`(?i)(?:罩杯|罩呗|置杯|cup|胸围|胸部|上围|🐻)(?:\s*:)*\s*妮妮(?:\s|💕|$)`)
)

// Parse applies the profile regular expressions used by FeiNiu_bot.
func Parse(text string) Fields {
	return Analyze(text).Fields
}

func Analyze(text string) Analysis {
	text = normalizeText(text)
	result := Analysis{
		HeightMentioned:   heightLabel.MatchString(text),
		WeightMentioned:   weightLabel.MatchString(text),
		CupMentioned:      cupLabel.MatchString(text),
		HeightSourceEmpty: heightEmpty.MatchString(text),
		WeightSourceEmpty: weightEmpty.MatchString(text),
		CupSourceEmpty:    cupEmpty.MatchString(text),
	}
	if match := agePattern.FindStringSubmatch(text); len(match) > 1 {
		result.Age, _ = strconv.Atoi(match[1])
	} else if match := birthYearAgePattern.FindStringSubmatch(text); len(match) > 1 {
		year, _ := strconv.Atoi(match[1])
		age := time.Now().Year() - (2000 + year)
		if age >= 18 && age <= 80 {
			result.Age = age
		}
	} else if match := standaloneAge.FindStringSubmatch(text); len(match) > 1 {
		result.Age, _ = strconv.Atoi(match[1])
	}
	if match := heightPattern.FindStringSubmatch(text); len(match) > 1 {
		result.HeightMentioned = true
		result.Height = parseHeight(match[1])
	}
	if match := weightPattern.FindStringSubmatch(text); len(match) > 1 {
		result.WeightMentioned = true
		result.Weight = parseWeight(match[1])
	}
	if result.Height == 0 {
		if match := reversedHeight.FindStringSubmatch(text); len(match) > 1 {
			result.HeightMentioned = true
			result.Height = parseHeight(match[1])
		}
	}
	if result.Weight == 0 {
		if match := weightPrefixedIndex.FindStringSubmatch(text); len(match) > 1 {
			result.WeightMentioned = true
			result.Weight = parseWeight(match[1] + "斤")
		}
	}
	if result.Height == 0 {
		if match := slashHeight.FindStringSubmatch(text); len(match) > 1 {
			result.HeightMentioned = true
			result.Height = parseHeight(match[1])
		}
	}
	if result.Height == 0 {
		if match := heightAfterYear.FindStringSubmatch(text); len(match) > 1 {
			result.HeightMentioned = true
			result.Height = parseHeight(match[1])
		}
	}
	if result.Weight == 0 {
		if match := slashWeight.FindStringSubmatch(text); len(match) > 1 {
			result.WeightMentioned = true
			result.Weight = parseWeight(match[1] + "斤")
		}
	}
	if result.Weight == 0 {
		if match := standaloneWeightKg.FindStringSubmatch(text); len(match) > 1 {
			result.Weight = parseWeight(match[1] + "kg")
		}
	}
	if result.Height == 0 {
		if match := chineseHeight.FindStringSubmatch(text); len(match) > 3 {
			result.HeightMentioned = true
			result.Height = chineseDigit(match[1])*100 + chineseDigit(match[2])*10 + chineseDigit(match[3])
		}
	}
	if result.Height == 0 {
		if match := chineseHeightShort.FindStringSubmatch(text); len(match) > 2 {
			result.HeightMentioned = true
			result.Height = chineseDigit(match[1])*100 + chineseDigit(match[2])*10
		}
	}
	if result.Weight == 0 {
		if match := chineseWeight.FindStringSubmatch(text); len(match) > 1 {
			result.WeightMentioned = true
			result.Weight = chineseDigit(match[1]) * 10
		}
	}
	if compact := compactFields.FindStringSubmatch(text); len(compact) > 3 {
		if result.Age == 0 {
			result.Age, _ = strconv.Atoi(compact[1])
		}
		result.HeightMentioned = true
		result.WeightMentioned = true
		if result.Height == 0 {
			result.Height = parseHeight(compact[2])
		}
		if result.Weight == 0 {
			result.Weight = parseWeight(compact[3])
		}
	}
	if compact := compactHeightCup.FindStringSubmatch(text); len(compact) > 2 {
		if result.Height == 0 {
			result.Height = parseHeight(compact[1])
		}
		if result.Cup == "" {
			result.Cup = strings.ToUpper(compact[2])
		}
	}
	if match := cupPattern.FindStringSubmatch(text); len(match) > 1 {
		result.CupMentioned = true
		result.Cup = strings.ToUpper(match[1])
	} else if match := crossCup.FindStringSubmatch(text); len(match) > 2 {
		result.CupMentioned = true
		result.Cup = strings.ToUpper(match[2])
	}
	if result.Cup == "" {
		if match := slashCup.FindStringSubmatch(text); len(match) > 1 {
			result.CupMentioned = true
			result.Cup = strings.ToUpper(match[1])
		}
	}
	if result.Cup == "" && cupHuge.MatchString(text) {
		result.CupMentioned = true
		result.Cup = "H"
	}
	if result.Cup == "" && cupTypoB.MatchString(text) {
		result.CupMentioned = true
		result.Cup = "B"
	}
	if result.Cup == "" && cupSmall.MatchString(text) {
		result.CupMentioned = true
		result.Cup = "A"
	}
	if shifted := fieldShifted.FindStringSubmatch(text); len(shifted) > 3 {
		result.Cup = strings.ToUpper(shifted[1])
		result.Height = parseHeight(shifted[2])
		result.Weight = parseWeight(shifted[3] + "斤")
	}
	if shifted := fieldShiftedReverse.FindStringSubmatch(text); len(shifted) > 3 {
		result.Height = parseHeight(shifted[1])
		result.Weight = parseWeight(shifted[2] + "斤")
		result.Cup = strings.ToUpper(shifted[3])
	}
	heightValue, hasHeightValue := matchedDecimal(heightRaw, text)
	weightValue, hasWeightValue := matchedDecimal(weightRaw, text)
	if result.Height == 0 && weightValue >= 140 && weightValue <= 210 && heightValue >= 50 && heightValue < 140 {
		result.Height = int(math.Round(weightValue))
		result.Weight = parseWeight(strconv.FormatFloat(heightValue, 'f', -1, 64) + "斤")
	}
	result.HeightSourceInvalid = result.HeightMentioned && !result.HeightSourceEmpty && result.Height == 0 && (hasHeightValue || heightInvalidText.MatchString(text))
	result.WeightSourceInvalid = result.WeightMentioned && !result.WeightSourceEmpty && result.Weight == 0 && (hasWeightValue || weightCupValue.MatchString(text) || weightExplicitJin.MatchString(text) || weightInvalidText.MatchString(text))
	result.CupSourceInvalid = result.CupMentioned && !result.CupSourceEmpty && result.Cup == "" && (cupUnknown.MatchString(text) || cupOutOfRange.MatchString(text) || cupNumericInvalid.MatchString(text) || cupVariable.MatchString(text) || cupNegative.MatchString(text) || cupMeasurements.MatchString(text) || cupTemporarilyUnknown.MatchString(text) || cupNumericGuess.MatchString(text) || cupNormalRange.MatchString(text) || cupUncertainText.MatchString(text) || cupRelativeSize.MatchString(text) || cupUnknownDecorated.MatchString(text) || cupUnknownTypo.MatchString(text) || cupUnfamiliar.MatchString(text) || cupUnknownVerbose.MatchString(text) || cupNumericUnit.MatchString(text) || cupNumericRange.MatchString(text) || cupNicknameValue.MatchString(text))
	return result
}

func matchedDecimal(pattern *regexp.Regexp, text string) (float64, bool) {
	match := pattern.FindStringSubmatch(text)
	if len(match) < 2 {
		return 0, false
	}
	return firstDecimal(match[1])
}

func normalizeText(text string) string {
	text = norm.NFKC.String(text)
	var builder strings.Builder
	for _, char := range text {
		if unicode.Is(unicode.Cf, char) || unicode.Is(unicode.M, char) {
			continue
		}
		builder.WriteRune(char)
	}
	replacer := strings.NewReplacer(
		"\r", "\n", "\u3000", " ", "\u00a0", " ", "：", ":", "；", ";", "，", ",",
		"（", "(", "）", ")", "－", "-", "—", "-", "➕", "+", "ā", "a", "Hight", "Height", "hight", "height",
	)
	return strings.ToLower(replacer.Replace(builder.String()))
}

func parseHeight(value string) int {
	normalized := strings.ToLower(strings.TrimSpace(value))
	number, ok := firstDecimal(normalized)
	if !ok {
		return 0
	}
	if number > 0 && number < 3 {
		number *= 100
	}
	height := int(math.Round(number))
	if height < 140 || height > 210 {
		return 0
	}
	return height
}

func parseWeight(value string) int {
	normalized := strings.ToLower(strings.TrimSpace(value))
	number, ok := firstDecimal(normalized)
	if !ok {
		return 0
	}
	if number >= 1000 && number < 3000 {
		number = math.Floor(number / 10)
	} else if number >= 900 && number < 1000 {
		compact := int(number) / 10
		if compact >= 50 && compact <= 150 {
			number = float64(compact)
		}
	}
	if strings.Contains(normalized, "kg") || strings.Contains(normalized, "公斤") || strings.Contains(normalized, "千克") {
		number *= 2
	} else if !strings.Contains(normalized, "斤") && number < 70 {
		number *= 2
	}
	weight := int(math.Round(number))
	if weight < 50 || weight > 300 {
		return 0
	}
	return weight
}

func firstDecimal(value string) (float64, bool) {
	match := regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`).FindString(value)
	if match == "" {
		return 0, false
	}
	number, err := strconv.ParseFloat(match, 64)
	return number, err == nil
}

func chineseDigit(value string) int {
	for index, digit := range []string{"零", "一", "二", "三", "四", "五", "六", "七", "八", "九"} {
		if value == digit {
			return index
		}
	}
	return 0
}
