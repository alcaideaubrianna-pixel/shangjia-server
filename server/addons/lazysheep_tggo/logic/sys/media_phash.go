package sys

import (
	"bytes"
	"encoding/hex"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	"github.com/gogf/gf/v2/os/gfile"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

func mediaPHashFromBytes(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	return mediaPHashFromImage(img)
}

func mediaPHashFromLocalPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || !gfile.Exists(path) || gfile.IsDir(path) {
		return ""
	}
	return mediaPHashFromBytes(gfile.GetBytes(path))
}

func mediaPHashFromImage(src image.Image) string {
	if src == nil {
		return ""
	}
	dst := image.NewGray(image.Rect(0, 0, 8, 8))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	values := make([]uint8, 0, 64)
	total := 0
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			v := color.GrayModel.Convert(dst.GrayAt(x, y)).(color.Gray).Y
			values = append(values, v)
			total += int(v)
		}
	}
	avg := total / len(values)
	var bits uint64
	for _, v := range values {
		bits <<= 1
		if int(v) >= avg {
			bits |= 1
		}
	}
	var out [8]byte
	for i := 7; i >= 0; i-- {
		out[i] = byte(bits)
		bits >>= 8
	}
	return hex.EncodeToString(out[:])
}
