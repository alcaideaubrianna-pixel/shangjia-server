package sys

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestTelegramAntiScanCacheKeyRemainsStableAfterMarking(t *testing.T) {
	media := &telegramMediaItem{Id: 12, Purpose: "display", AssetHash: "source-hash", AntiScanSeed: 34}
	key := telegramAntiScanCacheKey(media)
	media.AssetHash = "anti-scan:" + key
	if got := telegramAntiScanCacheKey(media); got != key {
		t.Fatalf("cache key changed after marking: got %q want %q", got, key)
	}
}

func TestRenderTelegramAntiScanFileDeterministicBySeed(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.png")
	writeAntiScanTestImage(t, sourcePath)

	firstPath := filepath.Join(dir, "first.jpg")
	secondPath := filepath.Join(dir, "second.jpg")
	thirdPath := filepath.Join(dir, "third.jpg")
	if err := renderTelegramAntiScanFile(sourcePath, firstPath, 101); err != nil {
		t.Fatal(err)
	}
	if err := renderTelegramAntiScanFile(sourcePath, secondPath, 101); err != nil {
		t.Fatal(err)
	}
	if err := renderTelegramAntiScanFile(sourcePath, thirdPath, 202); err != nil {
		t.Fatal(err)
	}
	first := mustReadFile(t, firstPath)
	second := mustReadFile(t, secondPath)
	third := mustReadFile(t, thirdPath)
	if !bytes.Equal(first, second) {
		t.Fatal("same seed should generate identical retry output")
	}
	if bytes.Equal(first, third) {
		t.Fatal("different push seeds should generate different image output")
	}
}

func TestRenderTelegramAntiScanFileChangesPerceptualHashBySeed(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.png")
	writeAntiScanTestImage(t, sourcePath)
	sourceHash, err := telegramAntiScanFileHash(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	hashes := make(map[telegramAntiScanHash]struct{})
	for seed := int64(1); seed <= 5; seed++ {
		targetPath := filepath.Join(dir, fmt.Sprintf("seed-%d.jpg", seed))
		if err := renderTelegramAntiScanFile(sourcePath, targetPath, seed); err != nil {
			t.Fatal(err)
		}
		finalPath, cleanup, err := prepareTelegramPhotoUploadFile(context.Background(), targetPath, nil)
		if cleanup != nil {
			defer cleanup()
		}
		if err != nil {
			t.Fatal(err)
		}
		hash, err := telegramAntiScanFileHash(finalPath)
		if err != nil {
			t.Fatal(err)
		}
		if _, passed := telegramAntiScanCandidateScore(sourceHash, hash, nil); !passed {
			t.Fatalf("seed %d failed perceptual distance after final JPEG encoding", seed)
		}
		hashes[hash] = struct{}{}
	}
	if len(hashes) < 4 {
		t.Fatalf("different seeds produced too few perceptual variants: %d", len(hashes))
	}
}

func TestTelegramAntiScanCandidateRejectsRecentCollision(t *testing.T) {
	source := telegramAntiScanHash{PHash: 0, DHash: 0}
	candidate := telegramAntiScanHash{PHash: 0xff, DHash: 0xff}
	if _, passed := telegramAntiScanCandidateScore(source, candidate, nil); !passed {
		t.Fatal("candidate far from source should pass")
	}
	history := []telegramAntiScanHash{{PHash: candidate.PHash ^ 1, DHash: candidate.DHash ^ 1}}
	if _, passed := telegramAntiScanCandidateScore(source, candidate, history); passed {
		t.Fatal("candidate close to recent history should be rejected")
	}
}

func TestMergeTelegramAntiScanHashHistoryDeduplicatesAndTrims(t *testing.T) {
	current := telegramAntiScanHash{PHash: 9, DHash: 9}
	history := []telegramAntiScanHash{
		current,
		{PHash: 8, DHash: 8},
		{PHash: 7, DHash: 7},
		{PHash: 6, DHash: 6},
	}
	merged := mergeTelegramAntiScanHashHistory(history, current, 3)
	if len(merged) != 3 {
		t.Fatalf("unexpected history length: %d", len(merged))
	}
	if merged[0] != current || merged[1] != history[1] || merged[2] != history[2] {
		t.Fatalf("unexpected history order: %#v", merged)
	}
}

func TestObfuscateTelegramCaptionKeepsHTMLAndLimitsNumberScope(t *testing.T) {
	caption := "<b>广州资料</b>\n📏身高：174.5\n⚖️体重：51kg\n电话：13800138000\n编号：G35535"
	first := obfuscateTelegramCaption(caption, 88)
	second := obfuscateTelegramCaption(caption, 88)
	if first != second {
		t.Fatal("same job seed should generate identical retry caption")
	}
	if !strings.Contains(first, "<b>广州资料</b>") {
		t.Fatalf("telegram HTML markup was not preserved: %s", first)
	}
	if !strings.Contains(first, "13800138000") || !strings.Contains(first, "G35535") {
		t.Fatalf("unrelated numbers were modified: %s", first)
	}
	if strings.ContainsAny(first, "★☆✦✧▲▼●○◆◇■□➢➤◈▣✿❀") {
		t.Fatalf("visible noise symbols were added: %s", first)
	}
	if strings.Contains(first, "身高：174.5") || strings.Contains(first, "体重：51kg") {
		t.Fatalf("height or weight was not perturbed: %s", first)
	}
	if regexp.MustCompile(`身高[^0-9]{0,8}[\x{200B}\x{200C}\x{2060}\x{2063}]`).MatchString(first) ||
		regexp.MustCompile(`体重[^0-9]{0,8}[\x{200B}\x{200C}\x{2060}\x{2063}]`).MatchString(first) {
		t.Fatalf("invisible characters were inserted before height or weight: %s", first)
	}
	if !regexp.MustCompile(`身高[^0-9]{0,8}(175|176|177|178)([^0-9]|$)`).MatchString(first) {
		t.Fatalf("height perturbation exceeded expected range: %s", first)
	}
	if !regexp.MustCompile(`体重[^0-9]{0,8}(49|53)kg`).MatchString(first) {
		t.Fatalf("weight perturbation exceeded expected range: %s", first)
	}
}

func TestObfuscateTelegramHeightOnlyIncreases(t *testing.T) {
	seen := make(map[string]bool)
	for seed := int64(1); seed <= 20; seed++ {
		result := obfuscateTelegramText("身高：180 体重：50kg", rand.New(rand.NewSource(seed)), make(map[string]string))
		matched := regexp.MustCompile(`身高：(180|181|182|183)`).FindStringSubmatch(result)
		if len(matched) != 2 {
			t.Fatalf("height must only increase, seed:%d result:%s", seed, result)
		}
		seen[matched[1]] = true
		if !strings.Contains(result, "体重：48kg") && !strings.Contains(result, "体重：52kg") {
			t.Fatalf("weight must support both directions, seed:%d result:%s", seed, result)
		}
	}
	if len(seen) < 3 {
		t.Fatalf("height random increment produced too few variants: %#v", seen)
	}
}

func TestObfuscateTelegramSynonymsUsesConsistentSafeReplacements(t *testing.T) {
	random := rand.New(rand.NewSource(2026))
	replacements := make(map[string]string)
	value := "安排：可，确认：可。态度：不同意；复核：不同意。\n描述：可爱 可靠 银行 行业 行情 是否 好看 行程 可预约"
	result := obfuscateTelegramText(value, random, replacements)

	positive := replacements["可"]
	if positive == "" || positive == "可" || !containsString(telegramObfuscationSynonymAlternatives["可"], positive) {
		t.Fatalf("unexpected positive replacement: %q map=%#v", positive, replacements)
	}
	if !strings.Contains(result, "安排："+positive+"，确认："+positive+"。") {
		t.Fatalf("all standalone 可 tokens should share one replacement %q: %s", positive, result)
	}
	negative := replacements["不同意"]
	if negative == "" || negative == "不同意" || !containsString(telegramObfuscationSynonymAlternatives["不同意"], negative) {
		t.Fatalf("unexpected negative replacement: %q map=%#v", negative, replacements)
	}
	if !strings.Contains(result, "态度："+negative+"；复核："+negative+"。") {
		t.Fatalf("all standalone 不同意 tokens should share one replacement %q: %s", negative, result)
	}
	protected := "描述：可爱 可靠 银行 行业 行情 是否 好看 行程 可预约"
	if !strings.Contains(result, protected) {
		t.Fatalf("embedded words must not be replaced: %s", result)
	}
}

func TestObfuscateTelegramSynonymsDoesNotGenerateAwkwardNegativeTerms(t *testing.T) {
	for seed := int64(1); seed <= 100; seed++ {
		result := obfuscateTelegramText("结果：不行，规则：不可以，状态：不能。", rand.New(rand.NewSource(seed)), make(map[string]string))
		if strings.Contains(result, "并非") || strings.Contains(result, "否") {
			t.Fatalf("awkward negative synonym generated, seed:%d result:%s", seed, result)
		}
	}
}

func TestObfuscateTelegramCaptionSharesSynonymMappingAcrossHTMLNodes(t *testing.T) {
	result := obfuscateTelegramCaption("<b>可</b>\n可", 77)
	matchedReplacement := ""
	for _, alternative := range telegramObfuscationSynonymAlternatives["可"] {
		if strings.Count(result, alternative) == 2 {
			matchedReplacement = alternative
			break
		}
	}
	if matchedReplacement == "" {
		t.Fatalf("same token across HTML nodes did not share one replacement: %s", result)
	}
	if !strings.Contains(result, "<b>"+matchedReplacement+"</b>") {
		t.Fatalf("telegram HTML markup was not preserved: %s", result)
	}
}

func TestObfuscateTelegramNumberRejectsLongUnrelatedNumbers(t *testing.T) {
	random := rand.New(rand.NewSource(8))
	result := obfuscateTelegramText("身高：1740 电话：13800138000 编号：G35535", random, make(map[string]string))
	if result != "身高：1740 电话：13800138000 编号：G35535" {
		t.Fatalf("long unrelated numbers must remain unchanged: %s", result)
	}
}

func writeAntiScanTestImage(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 96, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 96; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 2), G: uint8(y * 2), B: uint8((x + y) % 255), A: 255})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(file, img); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
