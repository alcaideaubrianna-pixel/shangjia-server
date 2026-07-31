package sys

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	xdraw "golang.org/x/image/draw"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type telegramChannelSendPolicy struct {
	AntiScanEnabled        bool
	TextObfuscationEnabled bool
}

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
	assetHash := strings.TrimSpace(media.AssetHash)
	if strings.HasPrefix(assetHash, "anti-scan:") {
		return strings.TrimPrefix(assetHash, "anti-scan:")
	}
	sourceHash := assetHash
	sum := sha256.Sum256([]byte(fmt.Sprintf("anti-scan-v1|%d|%d|%s|%s", media.AntiScanSeed, media.Id, media.Purpose, sourceHash)))
	return hex.EncodeToString(sum[:])
}

func prepareTelegramAntiScanFile(ctx context.Context, media *telegramMediaItem, sourcePath string) (string, error) {
	key := telegramAntiScanCacheKey(media)
	return cachedGeneratedMediaFile(ctx, key, sourcePath, ".jpg", func(target string) error {
		return renderTelegramAntiScanFile(sourcePath, target, media.AntiScanSeed)
	})
}

func prepareTelegramAntiScanThumbnailFile(ctx context.Context, media *telegramMediaItem, sourcePath string) (string, error) {
	assetHash := strings.TrimSpace(media.AssetHash)
	sum := sha256.Sum256([]byte(fmt.Sprintf("anti-scan-thumbnail-v1|%d|%d|%s|%s", media.AntiScanSeed, media.Id, media.Purpose, assetHash)))
	key := hex.EncodeToString(sum[:])
	return cachedGeneratedMediaFile(ctx, key, sourcePath, ".jpg", func(target string) error {
		return renderTelegramAntiScanFile(sourcePath, target, media.AntiScanSeed)
	})
}

func renderTelegramAntiScanFile(sourcePath string, targetPath string, seed int64) error {
	file, err := os.Open(sourcePath)
	if err != nil {
		return gerror.Wrap(err, "打开防扫图源文件失败")
	}
	source, _, err := image.Decode(file)
	_ = file.Close()
	if err != nil {
		return gerror.Wrap(err, "解析防扫图源文件失败")
	}
	random := rand.New(rand.NewSource(seed))
	bounds := source.Bounds()
	left := random.Intn(4)
	top := random.Intn(4)
	right := random.Intn(4)
	bottom := random.Intn(4)
	if bounds.Dx()-left-right < 2 || bounds.Dy()-top-bottom < 2 {
		left, top, right, bottom = 0, 0, 0, 0
	}
	crop := image.Rect(bounds.Min.X+left, bounds.Min.Y+top, bounds.Max.X-right, bounds.Max.Y-bottom)
	width := crop.Dx() * (992 + random.Intn(7)) / 1000
	height := crop.Dy() * (992 + random.Intn(7)) / 1000
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	destination := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(destination, destination.Bounds(), source, crop, draw.Src, nil)
	brightness := random.Intn(7) - 3
	warmth := random.Intn(5) - 2
	saturation := 0.985 + random.Float64()*0.03
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pixel := destination.RGBAAt(x, y)
			average := (float64(pixel.R) + float64(pixel.G) + float64(pixel.B)) / 3
			pixel.R = uint8(clampInt(int(average+(float64(pixel.R)-average)*saturation)+brightness+warmth, 0, 255))
			pixel.G = uint8(clampInt(int(average+(float64(pixel.G)-average)*saturation)+brightness, 0, 255))
			pixel.B = uint8(clampInt(int(average+(float64(pixel.B)-average)*saturation)+brightness-warmth, 0, 255))
			pixel.A = 255
			destination.SetRGBA(x, y, pixel)
		}
	}
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

var telegramObfuscationNumberPattern = regexp.MustCompile(`(身高|体重)([^0-9]{0,6})([0-9]{3})(?:\.[0-9]+)?`)

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
	for _, node := range nodes {
		obfuscateTelegramTextNodes(node, random)
	}
	var builder bytes.Buffer
	for _, node := range nodes {
		if err = xhtml.Render(&builder, node); err != nil {
			return caption
		}
	}
	zeroWidth := []string{"\u200B", "\u200C", "\u200D", "\uFEFF"}
	for index := 0; index < 3+random.Intn(6); index++ {
		builder.WriteString(zeroWidth[random.Intn(len(zeroWidth))])
	}
	return builder.String()
}

func obfuscateTelegramTextNodes(node *xhtml.Node, random *rand.Rand) {
	if node == nil {
		return
	}
	if node.Type == xhtml.TextNode {
		node.Data = obfuscateTelegramText(node.Data, random)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		obfuscateTelegramTextNodes(child, random)
	}
}

func obfuscateTelegramText(value string, random *rand.Rand) string {
	value = telegramObfuscationNumberPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := telegramObfuscationNumberPattern.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		number, err := strconv.ParseFloat(parts[3], 64)
		if err != nil {
			return match
		}
		delta := 0.1 + random.Float64()*0.9
		if random.Intn(2) == 0 {
			delta = -delta
		}
		return parts[1] + parts[2] + strconv.FormatFloat(number+delta, 'f', 1, 64)
	})
	symbols := []rune("★☆✦✧▲▼●○◆◇■□➢➤◈▣✿❀")
	var builder strings.Builder
	for _, char := range value {
		builder.WriteRune(char)
		if strings.ContainsRune("：:,，。！!", char) && random.Float64() < 0.28 {
			builder.WriteRune(symbols[random.Intn(len(symbols))])
		}
	}
	return builder.String()
}
