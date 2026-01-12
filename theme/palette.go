package theme

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type RGB [3]uint8

type Palette struct {
	Name   string
	Colors []RGB
}

func LoadGPL(path string) (*Palette, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	p := &Palette{}
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "Name:") {
			p.Name = strings.TrimSpace(strings.TrimPrefix(line, "Name:"))
			continue
		}

		// Skip headers and comments
		if line == "" || line[0] == '#' || strings.HasPrefix(line, "GIMP") || strings.HasPrefix(line, "Columns") {
			continue
		}

		// Parse RGB values (first 3 fields are R G B)
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			r, err1 := strconv.Atoi(fields[0])
			g, err2 := strconv.Atoi(fields[1])
			b, err3 := strconv.Atoi(fields[2])
			if err1 == nil && err2 == nil && err3 == nil {
				p.Colors = append(p.Colors, RGB{uint8(r), uint8(g), uint8(b)})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(p.Colors) == 0 {
		return nil, fmt.Errorf("no colors found in palette %s", path)
	}

	return p, nil
}

func MustLoadGPL(path string) *Palette {
	p, err := LoadGPL(path)
	if err != nil {
		panic(fmt.Sprintf("failed to load palette %s: %v", path, err))
	}
	return p
}

// Lookup returns interpolated color for normalized value 0-1
func (p *Palette) Lookup(norm float64) RGB {
	if norm <= 0 {
		return p.Colors[0]
	}
	if norm >= 1 {
		return p.Colors[len(p.Colors)-1]
	}

	// Find the two colors to interpolate between
	pos := norm * float64(len(p.Colors)-1)
	i := int(pos)
	frac := pos - float64(i)

	c0 := p.Colors[i]
	c1 := p.Colors[i+1]

	return RGB{
		lerp(c0[0], c1[0], frac),
		lerp(c0[1], c1[1], frac),
		lerp(c0[2], c1[2], frac),
	}
}

func lerp(a, b uint8, t float64) uint8 {
	return uint8(float64(a)*(1-t) + float64(b)*t)
}

// Index returns color at specific index (no interpolation)
func (p *Palette) Index(i int) RGB {
	if i < 0 {
		return p.Colors[0]
	}
	if i >= len(p.Colors) {
		return p.Colors[len(p.Colors)-1]
	}
	return p.Colors[i]
}
