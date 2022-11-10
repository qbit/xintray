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
	if x == 0 || x == width {
		return true
	}

	if y == 0 || y == height {
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

	u2d, err := parseHexColor("#46d700")
	off, err := parseHexColor("#c1c1c1")

	if err != nil {
		log.Println(err)
	}

	i.data = image.NewRGBA(image.Rect(0, 0, width, height))
	border := &color.RGBA{
		R: 0x00,
		G: 0x00,
		B: 0x00,
		A: 0xff,
	}

	for y := 0; y < width; y++ {
		for x := 0; x < height; x++ {
			if isEdge(x, y) {
				i.data.Set(x, y, border)
			} else {
				if xin.uptodate() {
					i.data.Set(x, y, u2d)
				} else {
					i.data.Set(x, y, off)
				}
			}
		}
	}
	return i
}
