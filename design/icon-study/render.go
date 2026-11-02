//go:build ignore

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/egoist/mygo/ui"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

func raster(data []byte, size int) image.Image {
	svg := ui.MustParseSVG(data)
	tt := ui.NewTester(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Image(c, svg).Size(float32(size), float32(size))
	}, size, size)
	return tt.Image()
}
func save(path string, img image.Image) {
	f, e := os.Create(path)
	if e != nil {
		panic(e)
	}
	defer f.Close()
	if e = png.Encode(f, img); e != nil {
		panic(e)
	}
}
func main() {
	dir := "design/icon-study"
	sheet := image.NewRGBA(image.Rect(0, 0, 1000, 800))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.RGBA{35, 35, 38, 255}), image.Point{}, draw.Src)
	for i := 1; i <= 20; i++ {
		data, e := os.ReadFile(filepath.Join(dir, "drafts", fmt.Sprintf("%02d.svg", i)))
		if e != nil {
			panic(e)
		}
		img := raster(data, 144)
		save(filepath.Join(dir, "drafts", fmt.Sprintf("%02d.png", i)), img)
		x := ((i-1)%5)*200 + 28
		y := ((i-1)/5)*200 + 12
		draw.Draw(sheet, image.Rect(x, y, x+144, y+144), img, image.Point{}, draw.Over)
		d := font.Drawer{Dst: sheet, Src: image.White, Face: basicfont.Face7x13, Dot: fixed.P(x+62, y+165)}
		d.DrawString(fmt.Sprintf("%02d", i))
	}
	save(filepath.Join(dir, "01-twenty-drafts.png"), sheet)
	if len(os.Args) > 1 {
		data, e := os.ReadFile(os.Args[1])
		if e != nil {
			panic(e)
		}
		for _, s := range []int{16, 24, 32, 48, 64, 128, 256, 512, 1024} {
			save(filepath.Join(dir, fmt.Sprintf("final-%d.png", s)), raster(data, s))
		}
		if len(os.Args) > 2 && os.Args[2] == "-install" {
			if e = os.WriteFile("resources/icon.svg", data, 0644); e != nil {
				panic(e)
			}
			save("resources/icon.png", raster(data, 1024))
			var frames [][]byte
			sizes := []int{16, 24, 32, 48, 64, 128, 256}
			for _, s := range sizes {
				var b bytes.Buffer
				if e := png.Encode(&b, raster(data, s)); e != nil {
					panic(e)
				}
				frames = append(frames, b.Bytes())
			}
			var ico bytes.Buffer
			binary.Write(&ico, binary.LittleEndian, uint16(0))
			binary.Write(&ico, binary.LittleEndian, uint16(1))
			binary.Write(&ico, binary.LittleEndian, uint16(len(frames)))
			offset := 6 + 16*len(frames)
			for i, f := range frames {
				ico.Write([]byte{byte(sizes[i] % 256), byte(sizes[i] % 256), 0, 0})
				binary.Write(&ico, binary.LittleEndian, uint16(1))
				binary.Write(&ico, binary.LittleEndian, uint16(32))
				binary.Write(&ico, binary.LittleEndian, uint32(len(f)))
				binary.Write(&ico, binary.LittleEndian, uint32(offset))
				offset += len(f)
			}
			for _, f := range frames {
				ico.Write(f)
			}
			if e := os.WriteFile("resources/icon.ico", ico.Bytes(), 0644); e != nil {
				panic(e)
			}
			tray, e := os.ReadFile(filepath.Join(dir, "tray-16.svg"))
			if e != nil {
				panic(e)
			}
			tray = []byte(strings.ReplaceAll(string(tray), "#f2f2f0", "#141414"))
			save("resources/tray.png", raster(tray, 32))
			for _, size := range []int{16, 20, 24} {
				data, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("tray-%d.svg", size)))
				if err != nil {
					panic(err)
				}
				data = []byte(strings.ReplaceAll(string(data), "#f2f2f0", "#141414"))
				save(fmt.Sprintf("resources/tray-%d.png", size), raster(data, size))
			}
		}
	}
	proof := image.NewRGBA(image.Rect(0, 0, 768, 400))
	draw.Draw(proof, proof.Bounds(), image.NewUniform(color.RGBA{35, 35, 38, 255}), image.Point{}, draw.Src)
	if data, e := os.ReadFile(filepath.Join(dir, "03-refined.svg")); e == nil {
		draw.Draw(proof, image.Rect(32, 32, 288, 288), raster(data, 256), image.Point{}, draw.Over)
	}
	for i, s := range []int{16, 20, 24} {
		data, e := os.ReadFile(filepath.Join(dir, fmt.Sprintf("tray-%d.svg", s)))
		if e != nil {
			continue
		}
		img := raster(data, s)
		save(filepath.Join(dir, fmt.Sprintf("tray-%d.png", s)), img)
		dark := []byte(strings.ReplaceAll(string(data), "#f2f2f0", "#141414"))
		save(filepath.Join(dir, fmt.Sprintf("tray-dark-%d.png", s)), raster(dark, s))
		x := 336 + i*128
		draw.Draw(proof, image.Rect(x, 64, x+s, 64+s), img, image.Point{}, draw.Over)
		// Nearest-neighbour enlargement reveals exact alpha coverage and spacing.
		zoom := image.NewRGBA(image.Rect(0, 0, s*4, s*4))
		for y := 0; y < s*4; y++ {
			for xx := 0; xx < s*4; xx++ {
				zoom.Set(xx, y, img.At(xx/4, y/4))
			}
		}
		draw.Draw(proof, image.Rect(x, 128, x+s*4, 128+s*4), zoom, image.Point{}, draw.Over)
		d := font.Drawer{Dst: proof, Src: image.White, Face: basicfont.Face7x13, Dot: fixed.P(x, 244)}
		d.DrawString(fmt.Sprintf("%dpx / 4x", s))
	}
	save(filepath.Join(dir, "04-refined-proof.png"), proof)
}
