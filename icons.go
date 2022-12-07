package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
)

const width, height = 288, 288

func parseHexColor(s string) (*color.RGBA, error) {
	c := &color.RGBA{
		A: 0xff,
	}
	_, err := fmt.Sscanf(s, "#%02x%02x%02x", &c.R, &c.G, &c.B)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func isEdge(x, y int) bool {
	if x == 0 || x == width-1 {
		return true
	}

	if y == 0 || y == height-1 {
		return true
	}

	return false
}

type myIcon struct {
	data *image.RGBA
}

func (m *myIcon) Name() string {
	return "Hi.png"
}

func (m *myIcon) Content() []byte {
	buf := new(bytes.Buffer)
	_ = png.Encode(buf, m.data)
	return buf.Bytes()
}

func buildImage(xin *xinStatus) *myIcon {
	i := &myIcon{}

	on, err := parseHexColor("#92CAFF")
	if err != nil {
		log.Println(err)
	}
	off, err := parseHexColor("#c1c1c1")
	if err != nil {
		log.Println(err)
	}
	border, err := parseHexColor("#000000")

	if err != nil {
		log.Println(err)
	}

	i.data = image.NewRGBA(image.Rect(0, 0, width, height))

	aliveCount := int(xin.aliveCount())
	utdCount := int(xin.uptodateCount())
	gridMark := 1
	if aliveCount > 0 {
		gridMark = int(height / aliveCount)
	}

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			if isEdge(x, y) {
				i.data.Set(x, y, border)
			} else {
				if aliveCount > 0 && y < gridMark*utdCount {
					i.data.Set(x, y, on)
				} else {
					i.data.Set(x, y, off)
				}
			}
		}
	}
	return i
}
