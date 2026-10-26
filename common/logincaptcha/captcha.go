// Package logincaptcha renders the six-digit motor sign-in challenge.
package logincaptcha

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"math/big"
	"sync"

	"github.com/mojocn/base64Captcha"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

var parseFont = sync.OnceValues(func() (*opentype.Font, error) { return opentype.Parse(gomonobold.TTF) })

// Generate uses the same configured store as the existing login verifier.
// The PNG matches motor's light background and tilted digits without exposing
// the answer as selectable SVG text or returning it in the API envelope.
func Generate() (id, data string, err error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", "", err
	}
	answer := fmt.Sprintf("%06d", value.Int64())
	data, err = render(answer)
	if err != nil {
		return "", "", err
	}
	id = rand.Text()
	if err = base64Captcha.DefaultMemStore.Set(id, answer); err != nil {
		return "", "", err
	}
	return id, data, nil
}

func render(answer string) (string, error) {
	parsed, err := parseFont()
	if err != nil {
		return "", err
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: 27, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return "", err
	}
	defer face.Close()
	bounds := image.Rect(0, 0, 180, 52)
	canvas := image.NewRGBA(bounds)
	draw.Draw(canvas, bounds, image.NewUniform(color.RGBA{243, 246, 251, 255}), image.Point{}, draw.Src)
	// Five subdued lines retain contrast while separating glyphs from the backdrop.
	for range 5 {
		var points [4]float32
		for i := range points {
			limit := int64(180)
			if i%2 == 1 {
				limit = 52
			}
			n, err := rand.Int(rand.Reader, big.NewInt(limit))
			if err != nil {
				return "", err
			}
			points[i] = float32(n.Int64())
		}
		line := vector.NewRasterizer(180, 52)
		line.MoveTo(points[0], points[1])
		line.LineTo(points[2], points[3])
		line.LineTo(points[2]+1, points[3]+1)
		line.LineTo(points[0]+1, points[1]+1)
		line.ClosePath()
		line.Draw(canvas, bounds, image.NewUniform(color.NRGBA{147, 164, 189, 115}), image.Point{})
	}
	for i, digit := range answer {
		glyph := image.NewRGBA(bounds)
		x, y := float64(22+i*26), float64(39)
		drawer := font.Drawer{Dst: glyph, Src: image.NewUniform(color.RGBA{36, 50, 74, 255}), Face: face, Dot: fixed.P(int(x), int(y))}
		drawer.DrawString(string(digit))
		angle := 6.0
		if i%2 == 1 {
			angle = -7
		}
		sine, cosine := math.Sincos(angle * math.Pi / 180)
		transform := f64.Aff3{cosine, -sine, x - cosine*x + sine*y, sine, cosine, y - sine*x - cosine*y}
		xdraw.BiLinear.Transform(canvas, transform, glyph, bounds, draw.Over, nil)
	}
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(output.Bytes()), nil
}
