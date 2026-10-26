package captcha

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// 图片渲染参数。
const (
	// charWidth 为 basicfont.Face7x13 的字符宽度。
	charWidth = 7
	// smallHeight 为小画布高度，与字体高度一致。
	smallHeight = 15
	// baseline 为字符绘制的基线位置，保证字形完整落在画布内。
	baseline = 12
	// maxScale 为放大倍数上限。
	maxScale = 4
)

// palette 为字符候选颜色。
var palette = []color.RGBA{
	{37, 99, 235, 255},
	{79, 70, 229, 255},
	{124, 58, 237, 255},
	{15, 118, 110, 255},
	{190, 24, 93, 255},
	{180, 83, 9, 255},
}

// Image 把验证码明文渲染为 PNG 图片。
func (m *Manager) Image(code string) ([]byte, error) {
	width, height := m.size()

	// 1. 在小画布上逐字符绘制（带纵向抖动）。
	smallWidth := len(code)*charWidth + 6
	small := image.NewRGBA(image.Rect(0, 0, smallWidth, smallHeight))
	for y := 0; y < smallHeight; y++ {
		for x := 0; x < smallWidth; x++ {
			small.Set(x, y, color.RGBA{246, 248, 252, 255})
		}
	}

	for index, char := range code {
		jitter := m.randInt(3) - 1
		drawer := &font.Drawer{
			Dst:  small,
			Src:  image.NewUniform(palette[m.randInt(len(palette))]),
			Face: basicfont.Face7x13,
			Dot:  fixed.P(3+index*charWidth, baseline+jitter),
		}
		drawer.DrawString(string(char))
	}

	// 2. 按目标尺寸计算放大倍数并等比缩放，避免裁切。
	scaleX := width / smallWidth
	scaleY := height / smallHeight
	scale := min(scaleX, scaleY)
	if scale > maxScale {
		scale = maxScale
	}
	if scale < 1 {
		scale = 1
	}

	big := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			big.Set(x, y, color.RGBA{246, 248, 252, 255})
		}
	}

	offsetX := (width - smallWidth*scale) / 2
	offsetY := (height - smallHeight*scale) / 2
	for y := 0; y < smallHeight*scale; y++ {
		for x := 0; x < smallWidth*scale; x++ {
			big.Set(offsetX+x, offsetY+y, small.At(x/scale, y/scale))
		}
	}

	// 3. 干扰线与噪点。
	m.drawNoise(big)

	var buf bytes.Buffer
	if err := png.Encode(&buf, big); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// drawNoise 在图片上叠加干扰线与噪点。
func (m *Manager) drawNoise(dst *image.RGBA) {
	bounds := dst.Bounds()
	for i := 0; i < 5; i++ {
		line := color.RGBA{
			uint8(120 + m.randInt(120)),
			uint8(120 + m.randInt(120)),
			uint8(120 + m.randInt(120)),
			160,
		}
		x0, y0 := m.randInt(bounds.Dx()), m.randInt(bounds.Dy())
		x1, y1 := m.randInt(bounds.Dx()), m.randInt(bounds.Dy())
		m.drawLine(dst, x0, y0, x1, y1, line)
	}
	for i := 0; i < bounds.Dx()*bounds.Dy()/90; i++ {
		dst.Set(m.randInt(bounds.Dx()), m.randInt(bounds.Dy()), color.RGBA{200, 205, 220, 255})
	}
}

// drawLine 使用 Bresenham 算法绘制直线。
func (m *Manager) drawLine(dst *image.RGBA, x0, y0, x1, y1 int, c color.Color) {
	dx := abs(x1 - x0)
	dy := abs(y1 - y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx - dy
	for {
		dst.Set(x0, y0, c)
		if x0 == x1 && y0 == y1 {
			return
		}
		double := 2 * err
		if double > -dy {
			err -= dy
			x0 += sx
		}
		if double < dx {
			err += dx
			y0 += sy
		}
	}
}

// size 返回图片宽高，未配置时使用 150x48。
func (m *Manager) size() (int, int) {
	width, height := m.Width, m.Height
	if width <= 0 {
		width = 150
	}
	if height <= 0 {
		height = 48
	}
	return width, height
}

// randInt 返回 [0, n) 的随机整数，内部串行化保证并发安全。
func (m *Manager) randInt(n int) int {
	if n <= 0 {
		return 0
	}
	m.randMu.Lock()
	defer m.randMu.Unlock()
	if m.rnd == nil {
		m.rnd = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return m.rnd.Intn(n)
}

// abs 返回绝对值。
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// min 返回较小值。
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
