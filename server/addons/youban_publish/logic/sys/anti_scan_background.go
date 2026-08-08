package sys

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	xdraw "golang.org/x/image/draw"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func patternedBackground(width int, height int, in *sysin.AntiScanPreviewInp) *image.RGBA {
	if in.BackgroundTextureImage != "" {
		if bg, ok := tiledCustomBackground(width, height, in.BackgroundTextureImage, in.StickerOpacity); ok {
			return bg
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	if in.BackgroundTexturePreset == "rabbit" || in.BackgroundTexturePreset == "" {
		drawRabbitBackground(dst)
		overlayTexture(dst, in.StickerOpacity/2)
		return dst
	}
	drawPresetBackground(dst, in.BackgroundTexturePreset, in.StickerOpacity)
	return dst
}

type antiScanTextureCacheItem struct {
	expiresAt time.Time
	data      []byte
}

var antiScanTextureCache sync.Map

func drawPresetBackground(dst *image.RGBA, preset string, opacity int) {
	b := dst.Bounds()
	c1 := color.RGBA{R: 238, G: 232, B: 218, A: 255}
	c2 := color.RGBA{R: 204, G: 214, B: 204, A: 255}
	c3 := color.RGBA{R: 178, G: 164, B: 146, A: 255}
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := c1
			if (x/42+y/42)%2 == 1 {
				c = c2
			}
			if preset == "heart" && (x+y)%74 < 8 {
				c = color.RGBA{R: 248, G: 190, B: 203, A: 255}
			}
			if preset == "dot" && (x%34-17)*(x%34-17)+(y%34-17)*(y%34-17) < 18 {
				c = color.RGBA{R: 92, G: 118, B: 122, A: 255}
			}
			if preset == "grid" && ((x+y)%96 < 4 || int(math.Abs(float64(x-y)))%120 < 3) {
				c = c3
			}
			dst.SetRGBA(x, y, c)
		}
	}
	overlayTexture(dst, opacity)
}

func tiledCustomBackground(width int, height int, imageUrl string, opacity int) (*image.RGBA, bool) {
	data, ok := readTextureImageBytes(imageUrl)
	if !ok {
		return nil, false
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	drawCoverImage(dst, img)
	overlayTexture(dst, opacity/3)
	return dst, true
}

func drawCoverImage(dst *image.RGBA, img image.Image) {
	db := dst.Bounds()
	sb := img.Bounds()
	if sb.Dx() <= 0 || sb.Dy() <= 0 {
		return
	}
	scale := math.Max(float64(db.Dx())/float64(sb.Dx()), float64(db.Dy())/float64(sb.Dy()))
	sw := int(float64(db.Dx()) / scale)
	sh := int(float64(db.Dy()) / scale)
	sx := sb.Min.X + maxInt(0, (sb.Dx()-sw)/2)
	sy := sb.Min.Y + maxInt(0, (sb.Dy()-sh)/2)
	srcRect := image.Rect(sx, sy, sx+minInt(sw, sb.Dx()), sy+minInt(sh, sb.Dy()))
	xdraw.ApproxBiLinear.Scale(dst, db, img, srcRect, draw.Src, nil)
}

func readTextureImageBytes(imageUrl string) ([]byte, bool) {
	imageUrl = strings.TrimSpace(imageUrl)
	if strings.HasPrefix(imageUrl, "data:image/") {
		idx := strings.Index(imageUrl, ",")
		if idx > 0 {
			data, err := base64.StdEncoding.DecodeString(imageUrl[idx+1:])
			return data, err == nil
		}
	}
	if !strings.HasPrefix(imageUrl, "http://") && !strings.HasPrefix(imageUrl, "https://") {
		return nil, false
	}
	if cached, ok := antiScanTextureCache.Load(imageUrl); ok {
		item := cached.(antiScanTextureCacheItem)
		if time.Now().Before(item.expiresAt) {
			return item.data, true
		}
		antiScanTextureCache.Delete(imageUrl)
	}
	request, err := http.NewRequest(http.MethodGet, imageUrl, nil)
	if err != nil {
		return nil, false
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(request)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil || len(data) == 0 {
		return nil, false
	}
	antiScanTextureCache.Store(imageUrl, antiScanTextureCacheItem{
		data:      data,
		expiresAt: time.Now().Add(15 * time.Minute),
	})
	return data, true
}

func drawRabbitBackground(dst *image.RGBA) {
	b := dst.Bounds()
	bg := &image.Uniform{C: color.RGBA{R: 250, G: 205, B: 222, A: 255}}
	draw.Draw(dst, b, bg, image.Point{}, draw.Src)
	for y := -20; y < b.Dy()+120; y += 150 {
		for x := -40; x < b.Dx()+160; x += 190 {
			offset := 0
			if (y/150)%2 != 0 {
				offset = 90
			}
			drawRabbitMotif(dst, x+offset, y+40)
			drawCarrotMotif(dst, x+offset+110, y+25)
			drawHeartMotif(dst, x+offset+70, y+108)
		}
	}
}

func drawRabbitMotif(dst *image.RGBA, cx int, cy int) {
	white := color.RGBA{R: 255, G: 255, B: 252, A: 230}
	pink := color.RGBA{R: 250, G: 156, B: 184, A: 165}
	dark := color.RGBA{R: 58, G: 42, B: 38, A: 220}
	fillEllipse(dst, cx-24, cy-18, 48, 44, white)
	fillEllipse(dst, cx-16, cy-52, 13, 42, white)
	fillEllipse(dst, cx+4, cy-52, 13, 42, white)
	fillEllipse(dst, cx-16, cy-2, 9, 6, pink)
	fillEllipse(dst, cx+9, cy-2, 9, 6, pink)
	fillEllipse(dst, cx-9, cy-7, 4, 5, dark)
	fillEllipse(dst, cx+9, cy-7, 4, 5, dark)
	fillEllipse(dst, cx-1, cy+4, 4, 3, dark)
}

func drawCarrotMotif(dst *image.RGBA, cx int, cy int) {
	orange := color.RGBA{R: 237, G: 142, B: 37, A: 225}
	green := color.RGBA{R: 52, G: 151, B: 72, A: 225}
	fillEllipse(dst, cx-14, cy, 22, 62, orange)
	fillEllipse(dst, cx-14, cy-18, 8, 28, green)
	fillEllipse(dst, cx-4, cy-24, 8, 32, green)
	fillEllipse(dst, cx+7, cy-16, 8, 25, green)
}

func drawHeartMotif(dst *image.RGBA, cx int, cy int) {
	pink := color.RGBA{R: 246, G: 174, B: 205, A: 135}
	fillEllipse(dst, cx-10, cy-8, 16, 16, pink)
	fillEllipse(dst, cx+1, cy-8, 16, 16, pink)
	fillEllipse(dst, cx-5, cy, 22, 18, pink)
}

func fillEllipse(dst *image.RGBA, x int, y int, width int, height int, c color.RGBA) {
	b := dst.Bounds()
	rx := float64(width) / 2
	ry := float64(height) / 2
	cx := float64(x) + rx
	cy := float64(y) + ry
	for py := y; py < y+height; py++ {
		if py < b.Min.Y || py >= b.Max.Y {
			continue
		}
		for px := x; px < x+width; px++ {
			if px < b.Min.X || px >= b.Max.X {
				continue
			}
			dx := (float64(px) - cx) / rx
			dy := (float64(py) - cy) / ry
			if dx*dx+dy*dy <= 1 {
				dst.SetRGBA(px, py, c)
			}
		}
	}
}
