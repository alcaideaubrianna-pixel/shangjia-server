package sys

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"strings"
	"sync"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"hotgo/addons/youban_publish/model/input/sysin"
)

// applyAntiScanWatermarks 绘制斜向透明水印，防泄漏模式更密集，轻水印模式更稀疏。
func applyAntiScanWatermarks(dst *image.RGBA, in *sysin.AntiScanPreviewInp) {
	lines := make([]string, 0, 3)
	if in.WatermarkEnabled == 1 {
		text := strings.TrimSpace(in.WatermarkText)
		if text == "" {
			text = "youban"
		}
		lines = append(lines, text)
	}
	if in.ProfileNoWatermarkEnabled == 1 {
		lines = append(lines, "profile-preview")
	}
	if len(lines) == 0 {
		return
	}
	fontSize := clampInt(in.WatermarkFontSize, 12, 56)
	alpha := uint8(clampInt(in.WatermarkOpacity*255/100, 13, 204))
	text := strings.TrimSpace(strings.Join(lines, "  "))
	if text == "" {
		text = "youban preview"
	}
	mark := rotateWatermark(renderWatermarkText(text, fontSize, alpha), -24)
	stepX := maxInt(mark.Bounds().Dx()+fontSize*6, 120)
	stepY := maxInt(mark.Bounds().Dy()+fontSize*3, 80)
	for y := -stepY; y < dst.Bounds().Dy()+stepY; y += stepY {
		for x := -stepX; x < dst.Bounds().Dx()+stepX; x += stepX {
			draw.Draw(dst, image.Rect(x, y, x+mark.Bounds().Dx(), y+mark.Bounds().Dy()), mark, image.Point{}, draw.Over)
		}
	}
}

func renderWatermarkText(text string, fontSize int, alpha uint8) *image.RGBA {
	face, ok := newWatermarkFontFace(fontSize)
	if !ok {
		return renderBasicWatermarkText(text, fontSize, alpha)
	}
	defer face.Close()
	drawer := font.Drawer{Face: face}
	width := maxInt((drawer.MeasureString(text)>>6).Ceil()+fontSize, fontSize*4)
	height := maxInt((face.Metrics().Height>>6).Ceil()+fontSize/2, fontSize*2)
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	drawer.Dst = dst
	drawer.Src = &image.Uniform{C: watermarkTextColor(alpha)}
	drawer.Dot = fixed.P(fontSize/2, fontSize+fontSize/3)
	drawer.DrawString(text)
	return dst
}

func renderBasicWatermarkText(text string, fontSize int, alpha uint8) *image.RGBA {
	text = firstASCII(text, "youban preview")
	baseWidth := maxInt(len(text)*8+8, 64)
	baseHeight := 18
	base := image.NewRGBA(image.Rect(0, 0, baseWidth, baseHeight))
	drawBasicText(base, 4, 13, text, watermarkTextColor(alpha))
	width := maxInt(baseWidth*fontSize/13, 1)
	height := maxInt(baseHeight*fontSize/13, 1)
	scaled := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.NearestNeighbor.Scale(scaled, scaled.Bounds(), base, base.Bounds(), draw.Over, nil)
	return scaled
}

func watermarkTextColor(alpha uint8) color.RGBA {
	return color.RGBA{R: 0, G: 0, B: 0, A: alpha}
}

func rotateWatermark(src *image.RGBA, degrees float64) *image.RGBA {
	angle := degrees * math.Pi / 180
	sin := math.Sin(angle)
	cos := math.Cos(angle)
	sw := src.Bounds().Dx()
	sh := src.Bounds().Dy()
	dw := int(math.Abs(float64(sw)*cos)+math.Abs(float64(sh)*sin)) + 4
	dh := int(math.Abs(float64(sw)*sin)+math.Abs(float64(sh)*cos)) + 4
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	scx := float64(sw) / 2
	scy := float64(sh) / 2
	dcx := float64(dw) / 2
	dcy := float64(dh) / 2
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			rx := float64(x) - dcx
			ry := float64(y) - dcy
			sx := rx*cos + ry*sin + scx
			sy := -rx*sin + ry*cos + scy
			ix := int(math.Round(sx))
			iy := int(math.Round(sy))
			if ix >= 0 && ix < sw && iy >= 0 && iy < sh {
				dst.SetRGBA(x, y, src.RGBAAt(ix, iy))
			}
		}
	}
	return dst
}

func drawBasicText(dst *image.RGBA, x int, y int, text string, c color.Color) {
	d := &font.Drawer{Dst: dst, Src: &image.Uniform{C: c}, Face: basicfont.Face7x13, Dot: fixed.P(x, y)}
	d.DrawString(text)
}

func firstASCII(value string, fallback string) string {
	value = strings.TrimSpace(value)
	buf := strings.Builder{}
	for _, r := range value {
		if r >= 32 && r <= 126 {
			buf.WriteRune(r)
		}
	}
	if strings.TrimSpace(buf.String()) == "" {
		return fallback
	}
	return buf.String()
}

var (
	watermarkFont     *opentype.Font
	watermarkFontOnce sync.Once
)

func newWatermarkFontFace(fontSize int) (font.Face, bool) {
	watermarkFontOnce.Do(func() {
		watermarkFont = loadWatermarkFont()
	})
	if watermarkFont == nil {
		return nil, false
	}
	face, err := opentype.NewFace(watermarkFont, &opentype.FaceOptions{
		Size:    float64(fontSize),
		DPI:     72,
		Hinting: font.HintingFull,
	})
	return face, err == nil
}

func loadWatermarkFont() *opentype.Font {
	for _, path := range watermarkFontPaths() {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		collection, err := opentype.ParseCollection(data)
		if err == nil && collection.NumFonts() > 0 {
			if fontItem, fontErr := collection.Font(0); fontErr == nil {
				return fontItem
			}
		}
		if fontItem, err := opentype.Parse(data); err == nil {
			return fontItem
		}
	}
	return nil
}

func watermarkFontPaths() []string {
	return []string{
		"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
		"/System/Library/Fonts/PingFang.ttc",
		"/System/Library/Fonts/STHeiti Medium.ttc",
		"/System/Library/Fonts/Hiragino Sans GB.ttc",
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/opentype/noto/NotoSansCJKsc-Regular.otf",
		"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/truetype/noto/NotoSansSC-Regular.ttf",
		"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
		"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
		"/usr/share/fonts/truetype/arphic/uming.ttc",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	}
}
