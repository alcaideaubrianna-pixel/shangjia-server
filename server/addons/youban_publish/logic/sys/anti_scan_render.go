package sys

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/skip2/go-qrcode"
	xdraw "golang.org/x/image/draw"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type antiScanFaceBox struct {
	X int
	Y int
	W int
	H int
}

// renderAntiScanPreview 按当前配置生成真实预览图，后续发送链路可复用同一处理入口。
func renderAntiScanPreview(ctx context.Context, src []byte, in *sysin.AntiScanPreviewInp, detect *antiScanDetectResult) ([]byte, []string, error) {
	if isAntiScanNoop(in) {
		return src, nil, nil
	}
	base, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, nil, gerror.New("图片格式不支持，请上传 JPG、PNG、GIF 或 WEBP")
	}
	faces := parseTencentFaceBoxes(detect.FaceRaw)
	warnings := []string{}
	canvas := imageToRGBA(base)
	if in.BackgroundReplaceEnabled == 1 || in.PortraitBackgroundEnabled == 1 || in.BackgroundTextureEnabled == 1 || in.BackgroundBlurEnabled == 1 {
		next, usedSegment := applyAntiScanBackground(canvas, detect.SegmentRaw, in)
		canvas = next
		if !usedSegment && (in.BackgroundReplaceEnabled == 1 || in.PortraitBackgroundEnabled == 1) {
			warnings = append(warnings, "未获取到可用人像分割结果，背景替换已降级为纹理叠加")
		}
	}
	if in.ColorJitterEnabled == 1 {
		applyColorJitter(canvas, in.ColorJitterStrength)
	}
	if in.NoiseEnabled == 1 {
		applyNoise(canvas, in.NoiseStrength)
	}
	if in.SharpenBlurEnabled == 1 {
		canvas = applySharpenBlur(canvas, in.SharpenBlurMode, in.SharpenBlurStrength)
	}
	if in.MaskEnabled == 1 && in.MaskCount > 0 {
		sticker := loadAntiScanSticker(ctx, in.StickerImage)
		applyAntiScanMasks(canvas, faces, in, sticker)
	}
	applyAntiScanWatermarks(canvas, in)
	canvas = applyCropResize(canvas, in)
	buf := bytes.NewBuffer(nil)
	if err = jpeg.Encode(buf, canvas, &jpeg.Options{Quality: normalizePreviewQuality(in.CompressionQuality)}); err != nil {
		return nil, nil, gerror.Wrap(err, "编码预览图失败")
	}
	return buf.Bytes(), warnings, nil
}

func isAntiScanNoop(in *sysin.AntiScanPreviewInp) bool {
	return in.MetadataStripEnabled == 0 &&
		in.ResizeEnabled == 0 &&
		in.CropEnabled == 0 &&
		in.PortraitBackgroundEnabled == 0 &&
		in.BackgroundReplaceEnabled == 0 &&
		in.BackgroundBlurEnabled == 0 &&
		in.BackgroundTextureEnabled == 0 &&
		in.MaskEnabled == 0 &&
		in.WatermarkEnabled == 0 &&
		in.ProfileNoWatermarkEnabled == 0 &&
		in.NoiseEnabled == 0 &&
		in.CompressionEnabled == 0 &&
		in.JpegQualityControlEnabled == 0 &&
		in.ColorJitterEnabled == 0 &&
		in.SharpenBlurEnabled == 0
}

func imageToRGBA(src image.Image) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

func applyAntiScanBackground(src *image.RGBA, segmentRaw string, in *sysin.AntiScanPreviewInp) (*image.RGBA, bool) {
	bounds := src.Bounds()
	bg := patternedBackground(bounds.Dx(), bounds.Dy(), in)
	if in.BackgroundBlurEnabled == 1 {
		bg = boxBlur(src, 5+in.StickerOpacity/10)
		if in.BackgroundTextureEnabled == 1 || in.PortraitBackgroundEnabled == 1 {
			overlayTexture(bg, in.StickerOpacity)
		}
	}
	if in.BackgroundReplaceEnabled != 1 && in.PortraitBackgroundEnabled != 1 {
		overlayTexture(src, in.StickerOpacity)
		return src, false
	}
	portrait, ok := decodeTencentSegmentPortrait(segmentRaw)
	if !ok {
		overlayTexture(src, in.StickerOpacity)
		return src, false
	}
	if portrait.Bounds().Dx() != bounds.Dx() || portrait.Bounds().Dy() != bounds.Dy() {
		resized := image.NewRGBA(bounds)
		xdraw.ApproxBiLinear.Scale(resized, bounds, portrait, portrait.Bounds(), draw.Over, nil)
		portrait = resized
	}
	draw.Draw(bg, bounds, portrait, image.Point{}, draw.Over)
	return bg, true
}

func patternedBackground(width int, height int, in *sysin.AntiScanPreviewInp) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	c1 := color.RGBA{R: 238, G: 232, B: 218, A: 255}
	c2 := color.RGBA{R: 204, G: 214, B: 204, A: 255}
	c3 := color.RGBA{R: 178, G: 164, B: 146, A: 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			tile := (x/42 + y/42) % 2
			c := c1
			if tile == 1 {
				c = c2
			}
			if (x+y)%96 < 4 || int(math.Abs(float64(x-y)))%120 < 3 {
				c = c3
			}
			dst.SetRGBA(x, y, c)
		}
	}
	overlayTexture(dst, in.StickerOpacity)
	return dst
}

func overlayTexture(dst *image.RGBA, opacity int) {
	alpha := clampInt(opacity, 10, 90)
	b := dst.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			if (x*7+y*11)%53 > 2 {
				continue
			}
			c := dst.RGBAAt(x, y)
			c.R = uint8(blendInt(int(c.R), 64, alpha/3))
			c.G = uint8(blendInt(int(c.G), 74, alpha/3))
			c.B = uint8(blendInt(int(c.B), 86, alpha/3))
			dst.SetRGBA(x, y, c)
		}
	}
}

func decodeTencentSegmentPortrait(raw string) (image.Image, bool) {
	if strings.TrimSpace(raw) == "" {
		return nil, false
	}
	var parsed struct {
		Response struct {
			ResultImage string `json:"ResultImage"`
			ResultMask  string `json:"ResultMask"`
		} `json:"Response"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, false
	}
	value := parsed.Response.ResultImage
	if value == "" {
		value = parsed.Response.ResultMask
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, false
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err == nil
}

func parseTencentFaceBoxes(raw string) []antiScanFaceBox {
	var parsed struct {
		Response struct {
			FaceInfos []struct {
				X      int `json:"X"`
				Y      int `json:"Y"`
				Width  int `json:"Width"`
				Height int `json:"Height"`
			} `json:"FaceInfos"`
		} `json:"Response"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil
	}
	boxes := make([]antiScanFaceBox, 0, len(parsed.Response.FaceInfos))
	for _, item := range parsed.Response.FaceInfos {
		if item.Width > 0 && item.Height > 0 {
			boxes = append(boxes, antiScanFaceBox{X: item.X, Y: item.Y, W: item.Width, H: item.Height})
		}
	}
	return boxes
}

func applyAntiScanMasks(dst *image.RGBA, faces []antiScanFaceBox, in *sysin.AntiScanPreviewInp, sticker image.Image) {
	count := clampInt(in.MaskCount, 1, 3)
	for i := 0; i < count; i++ {
		rect := pickMaskRect(dst.Bounds(), faces, i)
		if in.MaskMode == "sticker" {
			drawStickerMask(dst, rect, in.StickerOpacity, sticker)
			continue
		}
		drawQRCodeMask(dst, rect, in.QrText, in.StickerOpacity)
	}
}

func pickMaskRect(bounds image.Rectangle, faces []antiScanFaceBox, index int) image.Rectangle {
	w := bounds.Dx()
	h := bounds.Dy()
	size := clampInt(minInt(w, h)/5, 72, 180)
	margin := clampInt(size/3, 18, 64)
	candidates := []image.Rectangle{
		image.Rect(w-margin-size, h-margin-size, w-margin, h-margin),
		image.Rect(margin, h-margin-size, margin+size, h-margin),
		image.Rect(w-margin-size, margin, w-margin, margin+size),
		image.Rect(margin, margin, margin+size, margin+size),
		image.Rect(w/2-size/2, h-margin-size, w/2+size/2, h-margin),
	}
	for offset := 0; offset < len(candidates); offset++ {
		rect := candidates[(index+offset)%len(candidates)]
		if !overlapsAnyFace(expandRect(rect, size/8, bounds), faces, bounds) {
			return rect
		}
	}
	return candidates[index%len(candidates)]
}

func overlapsAnyFace(rect image.Rectangle, faces []antiScanFaceBox, bounds image.Rectangle) bool {
	for _, face := range faces {
		faceRect := image.Rect(face.X, face.Y, face.X+face.W, face.Y+face.H)
		faceRect = expandRect(faceRect, maxInt(face.W, face.H)/2, bounds)
		if rect.Overlaps(faceRect) {
			return true
		}
	}
	return false
}

func drawQRCodeMask(dst *image.RGBA, rect image.Rectangle, text string, opacity int) {
	if strings.TrimSpace(text) == "" {
		text = "youban-preview"
	}
	pngBytes, err := qrcode.Encode(text, qrcode.Medium, rect.Dx())
	if err != nil {
		drawStickerMask(dst, rect, opacity, nil)
		return
	}
	qr, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		drawStickerMask(dst, rect, opacity, nil)
		return
	}
	drawTranslucent(dst, rect, color.RGBA{R: 255, G: 255, B: 255, A: uint8(clampInt(opacity+80, 120, 220))})
	draw.Draw(dst, rect, qr, image.Point{}, draw.Over)
}

func drawStickerMask(dst *image.RGBA, rect image.Rectangle, opacity int, sticker image.Image) {
	drawTranslucent(dst, rect, color.RGBA{R: 20, G: 24, B: 32, A: uint8(clampInt(opacity+80, 110, 220))})
	if sticker != nil {
		xdraw.ApproxBiLinear.Scale(dst, rect, sticker, sticker.Bounds(), draw.Over, nil)
	}
}

func loadAntiScanSticker(ctx context.Context, value string) image.Image {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if strings.HasPrefix(value, "data:image/") {
		idx := strings.Index(value, ",")
		if idx > 0 {
			if data, err := base64.StdEncoding.DecodeString(value[idx+1:]); err == nil {
				img, _, _ := image.Decode(bytes.NewReader(data))
				return img
			}
		}
	}
	if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, value, nil)
	if err != nil {
		return nil
	}
	client := http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil
	}
	img, _, _ := image.Decode(bytes.NewReader(data))
	return img
}

func applyColorJitter(dst *image.RGBA, strength int) {
	delta := clampInt(strength, 1, 40)
	b := dst.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := dst.RGBAAt(x, y)
			c.R = uint8(clampInt(int(c.R)+delta/2, 0, 255))
			c.G = uint8(clampInt(int(c.G)-delta/3, 0, 255))
			c.B = uint8(clampInt(int(c.B)+delta/4, 0, 255))
			dst.SetRGBA(x, y, c)
		}
	}
}

func applyNoise(dst *image.RGBA, strength int) {
	r := rand.New(rand.NewSource(37))
	level := clampInt(strength, 1, 60)
	b := dst.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			if (x+y)%2 == 1 {
				continue
			}
			c := dst.RGBAAt(x, y)
			n := r.Intn(level*2+1) - level
			c.R = uint8(clampInt(int(c.R)+n, 0, 255))
			c.G = uint8(clampInt(int(c.G)+n, 0, 255))
			c.B = uint8(clampInt(int(c.B)+n, 0, 255))
			dst.SetRGBA(x, y, c)
		}
	}
}

func applySharpenBlur(src *image.RGBA, mode string, strength int) *image.RGBA {
	if mode == "sharpen" {
		return sharpen(src, clampInt(strength, 1, 30))
	}
	return boxBlur(src, 1+clampInt(strength, 1, 30)/8)
}

func applyCropResize(src *image.RGBA, in *sysin.AntiScanPreviewInp) *image.RGBA {
	out := src
	if in.CropEnabled == 1 {
		p := clampInt(in.CropPercent, 1, 8)
		cropX := out.Bounds().Dx() * p / 200
		cropY := out.Bounds().Dy() * p / 200
		rect := image.Rect(cropX, cropY, out.Bounds().Dx()-cropX, out.Bounds().Dy()-cropY)
		next := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
		draw.Draw(next, next.Bounds(), out, rect.Min, draw.Src)
		out = next
	}
	if in.ResizeEnabled == 1 {
		scale := clampInt(in.ResizeScale, 80, 100)
		w := maxInt(1, out.Bounds().Dx()*scale/100)
		h := maxInt(1, out.Bounds().Dy()*scale/100)
		next := image.NewRGBA(image.Rect(0, 0, w, h))
		xdraw.ApproxBiLinear.Scale(next, next.Bounds(), out, out.Bounds(), draw.Over, nil)
		out = next
	}
	return out
}

func drawTranslucent(dst *image.RGBA, rect image.Rectangle, c color.RGBA) {
	draw.Draw(dst, rect, &image.Uniform{C: c}, image.Point{}, draw.Over)
}

func boxBlur(src *image.RGBA, radius int) *image.RGBA {
	radius = clampInt(radius, 1, 12)
	b := src.Bounds()
	dst := image.NewRGBA(b)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			var rr, gg, bb, aa, count int
			for yy := maxInt(0, y-radius); yy <= minInt(b.Dy()-1, y+radius); yy++ {
				for xx := maxInt(0, x-radius); xx <= minInt(b.Dx()-1, x+radius); xx++ {
					c := src.RGBAAt(xx, yy)
					rr += int(c.R)
					gg += int(c.G)
					bb += int(c.B)
					aa += int(c.A)
					count++
				}
			}
			dst.SetRGBA(x, y, color.RGBA{R: uint8(rr / count), G: uint8(gg / count), B: uint8(bb / count), A: uint8(aa / count)})
		}
	}
	return dst
}

func sharpen(src *image.RGBA, strength int) *image.RGBA {
	blurred := boxBlur(src, 1)
	b := src.Bounds()
	dst := image.NewRGBA(b)
	amount := clampInt(strength, 1, 30)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			o := src.RGBAAt(x, y)
			bl := blurred.RGBAAt(x, y)
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8(clampInt(int(o.R)+(int(o.R)-int(bl.R))*amount/20, 0, 255)),
				G: uint8(clampInt(int(o.G)+(int(o.G)-int(bl.G))*amount/20, 0, 255)),
				B: uint8(clampInt(int(o.B)+(int(o.B)-int(bl.B))*amount/20, 0, 255)),
				A: o.A,
			})
		}
	}
	return dst
}

func expandRect(rect image.Rectangle, pad int, bounds image.Rectangle) image.Rectangle {
	return image.Rect(maxInt(bounds.Min.X, rect.Min.X-pad), maxInt(bounds.Min.Y, rect.Min.Y-pad), minInt(bounds.Max.X, rect.Max.X+pad), minInt(bounds.Max.Y, rect.Max.Y+pad))
}

func normalizePreviewQuality(value int) int {
	if value <= 0 {
		return 82
	}
	return clampInt(value, 60, 95)
}

func blendInt(base int, over int, alpha int) int {
	return (base*(100-alpha) + over*alpha) / 100
}

func clampInt(value int, min int, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
