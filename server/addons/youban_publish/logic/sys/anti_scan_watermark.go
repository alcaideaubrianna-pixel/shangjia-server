package sys

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
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
	text := firstASCII(strings.Join(lines, "  "), "youban preview")
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
	baseWidth := maxInt(len(text)*8+8, 64)
	baseHeight := 18
	base := image.NewRGBA(image.Rect(0, 0, baseWidth, baseHeight))
	drawBasicText(base, 4, 13, text, color.RGBA{R: 24, G: 32, B: 42, A: alpha})
	width := maxInt(baseWidth*fontSize/13, 1)
	height := maxInt(baseHeight*fontSize/13, 1)
	scaled := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.NearestNeighbor.Scale(scaled, scaled.Bounds(), base, base.Bounds(), draw.Over, nil)
	return scaled
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
