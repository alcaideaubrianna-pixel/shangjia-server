package main

// Local-only experiment. It never touches Telegram or production storage.
// Usage:
//   FAPIHUB_API_KEY=... go run ./tools/fapihub_highres_local_test.go input.jpg output.png

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"time"

	_ "golang.org/x/image/webp"
	_ "image/gif"
)

const endpoint = "https://fapihub.com/v2/rembg/"

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: FAPIHUB_API_KEY=... go run ./tools/fapihub_highres_local.go input-image output-png")
		os.Exit(2)
	}
	key := os.Getenv("FAPIHUB_API_KEY")
	if key == "" {
		panic("FAPIHUB_API_KEY is required")
	}
	sourceBytes, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	source, _, err := image.Decode(bytes.NewReader(sourceBytes))
	if err != nil {
		panic(err)
	}
	low := resizeNearest(source, 1600)
	var lowJPEG bytes.Buffer
	if err := jpeg.Encode(&lowJPEG, low, &jpeg.Options{Quality: 88}); err != nil {
		panic(err)
	}
	started := time.Now()
	resultBytes, err := callFapiHub(key, lowJPEG.Bytes())
	if err != nil {
		panic(err)
	}
	result, _, err := image.Decode(bytes.NewReader(resultBytes))
	if err != nil {
		panic(err)
	}
	output := compositeOriginalWithMask(source, result, color.RGBA{R: 245, G: 245, B: 245, A: 255})
	out, err := os.Create(os.Args[2])
	if err != nil {
		panic(err)
	}
	defer out.Close()
	if err := png.Encode(out, output); err != nil {
		panic(err)
	}
	alpha, total := alphaStats(result)
	fmt.Printf("input=%s %dx%d low=%dx%d requestBytes=%d result=%dx%d transparentPixels=%d/%d output=%s duration=%s\n", filepath.Base(os.Args[1]), source.Bounds().Dx(), source.Bounds().Dy(), low.Bounds().Dx(), low.Bounds().Dy(), lowJPEG.Len(), result.Bounds().Dx(), result.Bounds().Dy(), alpha, total, os.Args[2], time.Since(started).Round(time.Millisecond))
}

func callFapiHub(key string, imageBytes []byte) ([]byte, error) {
	body := bytes.NewBuffer(nil)
	w := multipart.NewWriter(body)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="image"; filename="local-test.jpg"`}
	h["Content-Type"] = []string{"image/jpeg"}
	part, err := w.CreatePart(textprotoHeader(h))
	if err != nil {
		return nil, err
	}
	if _, err = part.Write(imageBytes); err != nil {
		return nil, err
	}
	if err = w.WriteField("model", "falcon"); err != nil {
		return nil, err
	}
	if err = w.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("ApiKey", key)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 30<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("FAPIHub HTTP %d: %s", resp.StatusCode, data)
	}
	return data, nil
}

// textprotoHeader avoids pulling application code into this local experiment.
func textprotoHeader(values map[string][]string) textproto.MIMEHeader {
	return textproto.MIMEHeader(values)
}

func resizeNearest(src image.Image, max int) image.Image {
	b := src.Bounds()
	scale := 1.0
	if b.Dx() > max || b.Dy() > max {
		if b.Dx() >= b.Dy() {
			scale = float64(max) / float64(b.Dx())
		} else {
			scale = float64(max) / float64(b.Dy())
		}
	}
	w, h := int(float64(b.Dx())*scale), int(float64(b.Dy())*scale)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sx := b.Min.X + x*b.Dx()/w
			sy := b.Min.Y + y*b.Dy()/h
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func compositeOriginalWithMask(original, cutout image.Image, bg color.RGBA) *image.RGBA {
	b := original.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := color.NRGBAModel.Convert(original.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			mx := x * cutout.Bounds().Dx() / b.Dx()
			my := y * cutout.Bounds().Dy() / b.Dy()
			_, _, _, a := cutout.At(cutout.Bounds().Min.X+mx, cutout.Bounds().Min.Y+my).RGBA()
			alpha := uint32(a >> 8)
			out.SetRGBA(x, y, color.RGBA{R: uint8((uint32(c.R)*alpha + uint32(bg.R)*(255-alpha)) / 255), G: uint8((uint32(c.G)*alpha + uint32(bg.G)*(255-alpha)) / 255), B: uint8((uint32(c.B)*alpha + uint32(bg.B)*(255-alpha)) / 255), A: 255})
		}
	}
	return out
}

func alphaStats(img image.Image) (int, int) {
	n, t := 0, 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			t++
			if a>>8 < 250 {
				n++
			}
		}
	}
	return n, t
}
