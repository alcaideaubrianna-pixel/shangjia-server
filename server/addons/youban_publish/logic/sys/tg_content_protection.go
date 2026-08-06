package sys

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"
	"math/bits"
	"math/rand"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/corona10/goimagehash"
	"github.com/corona10/goimagehash/transforms"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	xdraw "golang.org/x/image/draw"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"hotgo/addons/youban_publish/model/input/sysin"
)

// 仅触发推送
type telegramChannelSendPolicy struct {
	AntiScanEnabled        bool
	TextObfuscationEnabled bool
}

const (
	telegramAntiScanHashDistanceTarget = 4
	telegramAntiScanCandidateAttempts  = 8
)

func (s *sSysPublish) telegramChannelSendPolicy(ctx context.Context, job telegramJobRecord) (telegramChannelSendPolicy, error) {
	var policy telegramChannelSendPolicy
	if job.ChannelId <= 0 || job.TenantId <= 0 {
		return policy, nil
	}
	var channel struct {
		AntiScanEnabled        int `orm:"anti_scan_enabled"`
		TextObfuscationEnabled int `orm:"text_obfuscation_enabled"`
	}
	if err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("anti_scan_enabled,text_obfuscation_enabled").
		Where("id", job.ChannelId).Where("tenant_id", job.TenantId).WhereNull("deleted_at").Scan(&channel); err != nil {
		return policy, gerror.Wrap(err, "读取频道内容保护配置失败")
	}
	vip, err := s.tenantVipStatus(ctx, job.TenantId)
	if err != nil {
		return policy, err
	}
	if vip == nil || !vip.IsVip {
		return policy, nil
	}
	policy.AntiScanEnabled = channel.AntiScanEnabled == 1 && containsString(vip.Features, sysin.TenantVipFeatureAntiScan)
	policy.TextObfuscationEnabled = channel.TextObfuscationEnabled == 1 && containsString(vip.Features, sysin.TenantVipFeatureTextObfuscation)
	return policy, nil
}

func (s *sSysPublish) applyTelegramJobContentProtection(ctx context.Context, job telegramJobRecord, caption string, mediaSets ...[]*telegramMediaItem) (string, error) {
	policy, err := s.telegramChannelSendPolicy(ctx, job)
	if err != nil {
		return "", err
	}
	if policy.TextObfuscationEnabled {
		caption = obfuscateTelegramCaption(caption, job.Id)
	}
	if policy.AntiScanEnabled {
		for _, media := range mediaSets {
			for _, item := range media {
				if item == nil || (!isTelegramImageMedia(item.MediaType) && !isTelegramVideoMedia(item.MediaType)) {
					continue
				}
				item.AntiScanEnabled = true
				item.AntiScanSeed = telegramProtectionSeed(job.Id, item.Id, item.Purpose)
				item.TgThumbFileId = ""
				if isTelegramImageMedia(item.MediaType) {
					item.TgFileId = ""
					item.AssetHash = "anti-scan:" + telegramAntiScanCacheKey(item)
				}
			}
		}
	}
	return caption, nil
}

func isTelegramImageMedia(mediaType string) bool {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	return mediaType == "image" || mediaType == "photo"
}

func isTelegramVideoMedia(mediaType string) bool {
	return strings.EqualFold(strings.TrimSpace(mediaType), "video")
}

func telegramProtectionSeed(values ...interface{}) int64 {
	sum := sha256.Sum256([]byte(fmt.Sprint(values...)))
	return int64(binary.LittleEndian.Uint64(sum[:8]))
}

func telegramAntiScanCacheKey(media *telegramMediaItem) string {
	return telegramAntiScanAttemptCacheKey(media, 0)
}

func telegramAntiScanAttemptCacheKey(media *telegramMediaItem, attempt int) string {
	assetHash := strings.TrimSpace(media.AssetHash)
	if attempt == 0 && strings.HasPrefix(assetHash, "anti-scan:") {
		return strings.TrimPrefix(assetHash, "anti-scan:")
	}
	sourceHash := assetHash
	sum := sha256.Sum256([]byte(fmt.Sprintf("anti-scan-v4|%d|%d|%s|%s|%d", media.AntiScanSeed, media.Id, media.Purpose, sourceHash, attempt)))
	return hex.EncodeToString(sum[:])
}

func prepareTelegramAntiScanFile(ctx context.Context, media *telegramMediaItem, sourcePath string, attempt int) (string, error) {
	key := telegramAntiScanAttemptCacheKey(media, attempt)
	return cachedGeneratedMediaFile(ctx, key, sourcePath, ".jpg", func(target string) error {
		return renderTelegramAntiScanCandidate(sourcePath, target, media.AntiScanSeed, attempt)
	})
}

func prepareTelegramAntiScanThumbnailFile(ctx context.Context, media *telegramMediaItem, sourcePath string, attempt int) (string, error) {
	assetHash := strings.TrimSpace(media.AssetHash)
	sum := sha256.Sum256([]byte(fmt.Sprintf("anti-scan-thumbnail-v4|%d|%d|%s|%s|%d", media.AntiScanSeed, media.Id, media.Purpose, assetHash, attempt)))
	key := hex.EncodeToString(sum[:])
	return cachedGeneratedMediaFile(ctx, key, sourcePath, ".jpg", func(target string) error {
		return renderTelegramAntiScanCandidate(sourcePath, target, media.AntiScanSeed, attempt)
	})
}

func renderTelegramAntiScanFile(sourcePath string, targetPath string, seed int64) error {
	return renderTelegramAntiScanCandidate(sourcePath, targetPath, seed, 0)
}

func renderTelegramAntiScanCandidate(sourcePath string, targetPath string, seed int64, attempt int) error {
	file, err := os.Open(sourcePath)
	if err != nil {
		return gerror.Wrap(err, "打开防扫图源文件失败")
	}
	source, _, err := image.Decode(file)
	_ = file.Close()
	if err != nil {
		return gerror.Wrap(err, "解析防扫图源文件失败")
	}
	perturbation := buildTelegramAntiScanPerturbation(source, seed, attempt)
	scale := selectTelegramAntiScanScale(source, perturbation)
	destination := applyTelegramAntiScanPerturbation(source, perturbation, scale)
	if !telegramAntiScanHashDistancePassed(source, destination) {
		for _, fallbackScale := range []float64{1, 1.25, 1.5, 2, 2.5, 3, 4, 5, 6, 8} {
			candidate := applyTelegramAntiScanPerturbation(source, perturbation, fallbackScale)
			if telegramAntiScanHashDistancePassed(source, candidate) {
				scale = fallbackScale
				destination = candidate
				break
			}
		}
	}
	applyTelegramAntiScanSeedVariation(destination, telegramProtectionSeed(seed, attempt))
	random := rand.New(rand.NewSource(telegramProtectionSeed(seed, attempt, scale, "jpeg")))
	out, err := os.Create(targetPath)
	if err != nil {
		return gerror.Wrap(err, "创建防扫图缓存文件失败")
	}
	quality := 91 + random.Intn(6)
	err = jpeg.Encode(out, destination, &jpeg.Options{Quality: quality})
	closeErr := out.Close()
	if err != nil {
		return gerror.Wrap(err, "编码防扫图缓存文件失败")
	}
	return closeErr
}

func telegramAntiScanFileHash(path string) (telegramAntiScanHash, error) {
	file, err := os.Open(path)
	if err != nil {
		return telegramAntiScanHash{}, err
	}
	img, _, err := image.Decode(file)
	_ = file.Close()
	if err != nil {
		return telegramAntiScanHash{}, err
	}
	return telegramAntiScanImageHash(img)
}

func telegramAntiScanImageHash(img image.Image) (telegramAntiScanHash, error) {
	pHash, err := goimagehash.PerceptionHash(img)
	if err != nil {
		return telegramAntiScanHash{}, err
	}
	dHash, err := goimagehash.DifferenceHash(img)
	if err != nil {
		return telegramAntiScanHash{}, err
	}
	return telegramAntiScanHash{PHash: pHash.GetHash(), DHash: dHash.GetHash()}, nil
}

func telegramAntiScanHashDistance(left uint64, right uint64) int {
	return bits.OnesCount64(left ^ right)
}

func telegramAntiScanCandidateScore(source telegramAntiScanHash, candidate telegramAntiScanHash, history []telegramAntiScanHash) (int, bool) {
	pDistance := telegramAntiScanHashDistance(source.PHash, candidate.PHash)
	dDistance := telegramAntiScanHashDistance(source.DHash, candidate.DHash)
	passed := pDistance > telegramAntiScanHashDistanceTarget && dDistance > telegramAntiScanHashDistanceTarget
	score := pDistance + dDistance
	for _, previous := range history {
		previousPDistance := telegramAntiScanHashDistance(previous.PHash, candidate.PHash)
		previousDDistance := telegramAntiScanHashDistance(previous.DHash, candidate.DHash)
		if previousPDistance <= telegramAntiScanHashDistanceTarget && previousDDistance <= telegramAntiScanHashDistanceTarget {
			passed = false
		}

		if distance := previousPDistance + previousDDistance; distance < score {
			score = distance
		}
	}
	return score, passed
}

func selectTelegramAntiScanScale(source image.Image, perturbation [64][64]float64) float64 {
	preview := resizeTelegramAntiScanPreview(source, 256)
	sourcePHash, err := goimagehash.PerceptionHash(preview)
	if err != nil {
		return 4
	}
	sourceDHash, err := goimagehash.DifferenceHash(preview)
	if err != nil {
		return 4
	}
	for _, scale := range []float64{1, 1.25, 1.5, 2, 2.5, 3, 4, 5, 6, 8} {
		candidate := applyTelegramAntiScanPerturbation(preview, perturbation, scale)
		candidatePHash, err := goimagehash.PerceptionHash(candidate)
		if err != nil {
			continue
		}
		candidateDHash, err := goimagehash.DifferenceHash(candidate)
		if err != nil {
			continue
		}
		pHashDistance, err := sourcePHash.Distance(candidatePHash)
		if err != nil {
			continue
		}
		dHashDistance, err := sourceDHash.Distance(candidateDHash)
		if err == nil && pHashDistance > telegramAntiScanHashDistanceTarget && dHashDistance > telegramAntiScanHashDistanceTarget {
			return scale
		}
	}
	return 4
}

func telegramAntiScanHashDistancePassed(source image.Image, candidate image.Image) bool {
	sourcePHash, err := goimagehash.PerceptionHash(source)
	if err != nil {
		return true
	}
	sourceDHash, err := goimagehash.DifferenceHash(source)
	if err != nil {
		return true
	}
	candidatePHash, err := goimagehash.PerceptionHash(candidate)
	if err != nil {
		return true
	}
	candidateDHash, err := goimagehash.DifferenceHash(candidate)
	if err != nil {
		return true
	}
	pHashDistance, err := sourcePHash.Distance(candidatePHash)
	if err != nil {
		return true
	}
	dHashDistance, err := sourceDHash.Distance(candidateDHash)
	return err != nil || (pHashDistance > telegramAntiScanHashDistanceTarget && dHashDistance > telegramAntiScanHashDistanceTarget)
}

func applyTelegramAntiScanSeedVariation(destination *image.RGBA, seed int64) {
	random := rand.New(rand.NewSource(telegramProtectionSeed(seed, "color")))
	brightness := random.Intn(3) - 1
	warmth := random.Intn(3) - 1
	if brightness == 0 && warmth == 0 {
		warmth = 1
	}
	for y := destination.Bounds().Min.Y; y < destination.Bounds().Max.Y; y++ {
		for x := destination.Bounds().Min.X; x < destination.Bounds().Max.X; x++ {
			pixel := destination.RGBAAt(x, y)
			pixel.R = uint8(clampInt(int(pixel.R)+brightness+warmth, 0, 255))
			pixel.G = uint8(clampInt(int(pixel.G)+brightness, 0, 255))
			pixel.B = uint8(clampInt(int(pixel.B)+brightness-warmth, 0, 255))
			destination.SetRGBA(x, y, pixel)
		}
	}
}

func resizeTelegramAntiScanPreview(source image.Image, maximumDimension int) image.Image {
	bounds := source.Bounds()
	if bounds.Dx() <= maximumDimension && bounds.Dy() <= maximumDimension {
		return source
	}
	ratio := math.Min(float64(maximumDimension)/float64(bounds.Dx()), float64(maximumDimension)/float64(bounds.Dy()))
	width := maxTelegramAntiScanInt(1, int(math.Round(float64(bounds.Dx())*ratio)))
	height := maxTelegramAntiScanInt(1, int(math.Round(float64(bounds.Dy())*ratio)))
	return resizeTelegramAntiScanImage(source, width, height)
}

func resizeTelegramAntiScanImage(source image.Image, width int, height int) image.Image {
	destination := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(destination, destination.Bounds(), source, source.Bounds(), draw.Src, nil)
	return destination
}

func buildTelegramAntiScanPerturbation(source image.Image, seed int64, attempt int) [64][64]float64 {
	random := rand.New(rand.NewSource(telegramProtectionSeed(seed, attempt, "perturbation")))
	gray := telegramAntiScanGray(resizeTelegramAntiScanImage(source, 64, 64))
	dct := transforms.DCT2D(cloneTelegramAntiScanMatrix(gray), 64, 64)
	values := make([]float64, 0, 64)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			values = append(values, dct[y][x])
		}
	}
	median := telegramAntiScanMedian(values)
	type coefficient struct {
		x, y     int
		distance float64
		value    float64
	}
	coefficients := make([]coefficient, 0, 63)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if x == 0 && y == 0 {
				continue
			}
			coefficients = append(coefficients, coefficient{x, y, math.Abs(dct[y][x] - median), dct[y][x]})
		}
	}
	sort.Slice(coefficients, func(i, j int) bool { return coefficients[i].distance < coefficients[j].distance })
	coefficientPoolSize := minTelegramAntiScanInt(len(coefficients), 32)
	random.Shuffle(coefficientPoolSize, func(i, j int) {
		coefficients[i], coefficients[j] = coefficients[j], coefficients[i]
	})
	var perturbation [64][64]float64
	for index := 0; index < minTelegramAntiScanInt(14, coefficientPoolSize); index++ {
		item := coefficients[index]
		margin := math.Max(14+float64(random.Intn(7)), item.distance*(0.35+random.Float64()*0.2))
		target := median + margin
		if item.value > median || item.value == median && random.Intn(2) == 0 {
			target = median - margin
		}
		telegramAntiScanAddDCT(&perturbation, item.x, item.y, target-item.value)
	}

	dHashGray := telegramAntiScanGray(resizeTelegramAntiScanImage(source, 9, 8))
	type edge struct {
		x, y       int
		difference float64
	}
	edges := make([]edge, 0, 64)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			edges = append(edges, edge{x, y, dHashGray[y][x+1] - dHashGray[y][x]})
		}
	}
	sort.Slice(edges, func(i, j int) bool { return math.Abs(edges[i].difference) < math.Abs(edges[j].difference) })
	edgePoolSize := minTelegramAntiScanInt(len(edges), 36)
	random.Shuffle(edgePoolSize, func(i, j int) {
		edges[i], edges[j] = edges[j], edges[i]
	})
	var field [8][9]float64
	used := map[[2]int]bool{}
	selected := 0
	for _, item := range edges {
		left := [2]int{item.y, item.x}
		right := [2]int{item.y, item.x + 1}
		if used[left] || used[right] {
			continue
		}
		adjustment := (math.Abs(item.difference) + math.Max(3+random.Float64()*2, math.Abs(item.difference)*0.25)) / 2
		if item.difference > 0 {
			field[item.y][item.x] += adjustment
			field[item.y][item.x+1] -= adjustment
		} else {
			field[item.y][item.x] -= adjustment
			field[item.y][item.x+1] += adjustment
		}
		used[left], used[right] = true, true
		selected++
		if selected == 14 {
			break
		}
	}
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			perturbation[y][x] += telegramAntiScanSampleField(field, (float64(x)+0.5)/64, (float64(y)+0.5)/64)
		}
	}
	return perturbation
}

func telegramAntiScanAddDCT(destination *[64][64]float64, coefficientX int, coefficientY int, delta float64) {
	scaleX := 2.0 / 64
	scaleY := 2.0 / 64
	if coefficientX == 0 {
		scaleX = 1.0 / 64
	}
	if coefficientY == 0 {
		scaleY = 1.0 / 64
	}
	for y := 0; y < 64; y++ {
		cosineY := math.Cos(math.Pi / 64 * (float64(y) + 0.5) * float64(coefficientY))
		for x := 0; x < 64; x++ {
			cosineX := math.Cos(math.Pi / 64 * (float64(x) + 0.5) * float64(coefficientX))
			destination[y][x] += delta * scaleX * scaleY * cosineX * cosineY
		}
	}
}

func applyTelegramAntiScanPerturbation(source image.Image, perturbation [64][64]float64, scale float64) *image.RGBA {
	bounds := source.Bounds()
	destination := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		fy := (float64(y) + 0.5) / float64(bounds.Dy()) * 63
		y0 := int(math.Floor(fy))
		y1 := maxTelegramAntiScanInt(0, minTelegramAntiScanInt(y0+1, 63))
		dy := fy - float64(y0)
		for x := 0; x < bounds.Dx(); x++ {
			fx := (float64(x) + 0.5) / float64(bounds.Dx()) * 63
			x0 := int(math.Floor(fx))
			x1 := maxTelegramAntiScanInt(0, minTelegramAntiScanInt(x0+1, 63))
			dx := fx - float64(x0)
			top := perturbation[y0][x0]*(1-dx) + perturbation[y0][x1]*dx
			bottom := perturbation[y1][x0]*(1-dx) + perturbation[y1][x1]*dx
			shift := (top*(1-dy) + bottom*dy) * scale
			r, g, b, a := source.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			destination.SetRGBA(x, y, color.RGBA{
				R: uint8(clampInt(int(r>>8)+int(math.Round(shift)), 0, 255)),
				G: uint8(clampInt(int(g>>8)+int(math.Round(shift)), 0, 255)),
				B: uint8(clampInt(int(b>>8)+int(math.Round(shift)), 0, 255)),
				A: uint8(a >> 8),
			})
		}
	}
	return destination
}

func telegramAntiScanGray(source image.Image) [][]float64 {
	bounds := source.Bounds()
	gray := make([][]float64, bounds.Dy())
	for y := range gray {
		gray[y] = make([]float64, bounds.Dx())
		for x := range gray[y] {
			r, g, b, _ := source.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			gray[y][x] = 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)
		}
	}
	return gray
}

func cloneTelegramAntiScanMatrix(source [][]float64) [][]float64 {
	result := make([][]float64, len(source))
	for index := range source {
		result[index] = append([]float64(nil), source[index]...)
	}
	return result
}

func telegramAntiScanMedian(values []float64) float64 {
	copyValues := append([]float64(nil), values...)
	sort.Float64s(copyValues)
	middle := len(copyValues) / 2
	return (copyValues[middle-1] + copyValues[middle]) / 2
}

func telegramAntiScanSampleField(field [8][9]float64, normalizedX float64, normalizedY float64) float64 {
	x := math.Max(0, math.Min(normalizedX, 1)) * 8
	y := math.Max(0, math.Min(normalizedY, 1)) * 7
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	x1, y1 := minTelegramAntiScanInt(x0+1, 8), minTelegramAntiScanInt(y0+1, 7)
	dx, dy := x-float64(x0), y-float64(y0)
	top := field[y0][x0]*(1-dx) + field[y0][x1]*dx
	bottom := field[y1][x0]*(1-dx) + field[y1][x1]*dx
	return top*(1-dy) + bottom*dy
}

func minTelegramAntiScanInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxTelegramAntiScanInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

var telegramObfuscationNumberPattern = regexp.MustCompile(`(身高|体重)([^0-9]{0,6})([0-9]{2,3}(?:\.[0-9]+)?)([^0-9]|$)`)

var telegramObfuscationSynonymGroups = [][]string{
	{"没问题", "没毛病", "可以", "OK", "ok", "可", "行", "能"},
	{"同意", "赞同", "认可", "接受", "支持"},
	{"是的", "没错", "确实", "是", "对"},
	{"好的", "不错", "好"},
	{"没办法", "不可以", "不行", "不可", "不能"},
	{"不同意", "不赞同", "不认可", "不接受", "不支持", "拒绝"},
	{"不是", "并非", "否"},
	{"不合适", "不建议", "不好", "不妥"},
}

var telegramObfuscationSynonymTerms = func() []string {
	terms := make([]string, 0)
	for _, group := range telegramObfuscationSynonymGroups {
		terms = append(terms, group...)
	}
	sort.SliceStable(terms, func(left int, right int) bool {
		return utf8.RuneCountInString(terms[left]) > utf8.RuneCountInString(terms[right])
	})
	return terms
}()

var telegramObfuscationSynonymAlternatives = func() map[string][]string {
	result := make(map[string][]string)
	for _, group := range telegramObfuscationSynonymGroups {
		for _, term := range group {
			alternatives := make([]string, 0, len(group)-1)
			for _, candidate := range group {
				if candidate != term {
					alternatives = append(alternatives, candidate)
				}
			}
			result[term] = alternatives
		}
	}
	return result
}()

func obfuscateTelegramCaption(caption string, seed int64) string {
	if strings.TrimSpace(caption) == "" {
		return caption
	}
	contextNode := &xhtml.Node{Type: xhtml.ElementNode, DataAtom: atom.Div, Data: "div"}
	nodes, err := xhtml.ParseFragment(strings.NewReader(caption), contextNode)
	if err != nil {
		return caption
	}
	random := rand.New(rand.NewSource(seed))
	synonymReplacements := make(map[string]string)
	for _, node := range nodes {
		obfuscateTelegramTextNodes(node, random, synonymReplacements)
	}
	var builder bytes.Buffer
	for _, node := range nodes {
		if err = xhtml.Render(&builder, node); err != nil {
			return caption
		}
	}
	zeroWidth := []rune{'\u200B', '\u200C', '\u2060', '\u2063'}
	for index := 0; index < 1+random.Intn(3); index++ {
		builder.WriteRune(zeroWidth[random.Intn(len(zeroWidth))])
	}
	return builder.String()
}

func obfuscateTelegramTextNodes(node *xhtml.Node, random *rand.Rand, synonymReplacements map[string]string) {
	if node == nil {
		return
	}
	if node.Type == xhtml.TextNode {
		node.Data = obfuscateTelegramText(node.Data, random, synonymReplacements)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		obfuscateTelegramTextNodes(child, random, synonymReplacements)
	}
}

func obfuscateTelegramText(value string, random *rand.Rand, synonymReplacements map[string]string) string {
	value = telegramObfuscationNumberPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := telegramObfuscationNumberPattern.FindStringSubmatch(match)
		if len(parts) != 5 {
			return match
		}
		number, err := strconv.ParseFloat(parts[3], 64)
		if err != nil {
			return match
		}
		rounded := int(math.Round(number))
		minimum, maximum := telegramObfuscationNumberBounds(parts[1])
		result := rounded + random.Intn(4)
		if parts[1] != "身高" {
			delta := 2
			if random.Intn(2) == 0 {
				delta = -delta
			}
			result = rounded + delta
			if result < minimum || result > maximum {
				result = rounded - delta
			}
		} else if result > maximum {
			result = maximum
		}
		return parts[1] + parts[2] + strconv.Itoa(result) + parts[4]
	})
	return obfuscateTelegramSynonyms(value, random, synonymReplacements)
}

func telegramObfuscationNumberBounds(label string) (int, int) {
	if label == "身高" {
		return 120, 220
	}
	return 30, 200
}

func obfuscateTelegramSynonyms(value string, random *rand.Rand, replacements map[string]string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	var builder strings.Builder
	builder.Grow(len(value))
	for index := 0; index < len(runes); {
		term, termLength := telegramObfuscationSynonymAt(runes, index)
		if term == "" || !telegramObfuscationTermBounded(runes, index, termLength) {
			builder.WriteRune(runes[index])
			index++
			continue
		}
		replacement := replacements[term]
		if replacement == "" {
			alternatives := telegramObfuscationSynonymAlternatives[term]
			if len(alternatives) == 0 {
				builder.WriteString(term)
				index += termLength
				continue
			}
			replacement = alternatives[random.Intn(len(alternatives))]
			replacements[term] = replacement
		}
		builder.WriteString(replacement)
		index += termLength
	}
	return builder.String()
}

func telegramObfuscationSynonymAt(value []rune, index int) (string, int) {
	for _, term := range telegramObfuscationSynonymTerms {
		termRunes := []rune(term)
		if index+len(termRunes) > len(value) {
			continue
		}
		matched := true
		for offset, termRune := range termRunes {
			if value[index+offset] != termRune {
				matched = false
				break
			}
		}
		if matched {
			return term, len(termRunes)
		}
	}
	return "", 0
}

func telegramObfuscationTermBounded(value []rune, index int, length int) bool {
	leftBounded := index == 0 || telegramObfuscationBoundaryRune(value[index-1])
	rightIndex := index + length
	rightBounded := rightIndex >= len(value) || telegramObfuscationBoundaryRune(value[rightIndex])
	return leftBounded && rightBounded
}

func telegramObfuscationBoundaryRune(value rune) bool {
	return unicode.IsSpace(value) || unicode.IsPunct(value) || unicode.IsSymbol(value)
}
