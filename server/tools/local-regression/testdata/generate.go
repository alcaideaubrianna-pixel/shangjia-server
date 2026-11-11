// 生成可被真实媒体处理流水线解码的本地 JPEG 夹具，不访问网络。
package main

import (
	"flag"
	"image"
	"image/color"
	"image/jpeg"
	"os"
)

func main() {
	output := flag.String("out", "image.jpg", "JPEG fixture path")
	flag.Parse()
	picture := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			picture.SetRGBA(x, y, color.RGBA{R: uint8(x * 4), G: uint8(y * 4), B: uint8((x + y) * 2), A: 255})
		}
	}
	file, err := os.Create(*output)
	if err != nil {
		panic(err)
	}
	if err := jpeg.Encode(file, picture, &jpeg.Options{Quality: 85}); err != nil {
		panic(err)
	}
	if err := file.Close(); err != nil {
		panic(err)
	}
}
