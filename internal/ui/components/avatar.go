package components

import (
	"strings"

	"charm.land/lipgloss/v2"
)

const background = "#101317"

// Avatar draws an 8x8 pixel mascot in four terminal rows using half-blocks.
func Avatar(provider, customColor, fallback string) []string {
	ink := providerColor(provider, customColor)
	if fallback != "" {
		if preset, ok := presetSprite(fallback); ok {
			return renderSprite(preset, ink)
		}
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(ink))
		return []string{style.Render("  " + fallback), "", "", ""}
	}
	return renderSprite(sprite(provider), ink)
}

func renderSprite(pixels [8]string, ink string) []string {
	rows := make([]string, 4)
	for pair := range rows {
		var line strings.Builder
		for col := 0; col < 8; col++ {
			upper := pixelColor(pixels[pair*2][col], ink)
			lower := pixelColor(pixels[pair*2+1][col], ink)
			glyph := " "
			if pixels[pair*2][col] != '.' || pixels[pair*2+1][col] != '.' {
				glyph = "▀"
			}
			line.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(upper)).Background(lipgloss.Color(lower)).Render(glyph))
		}
		rows[pair] = line.String()
	}
	return rows
}

func presetSprite(id string) ([8]string, bool) {
	switch id {
	case "pixel1":
		return [8]string{"...##...", "...##...", "..####..", ".######.", "##e##e##", "##e##e##", ".######.", "..####.."}, true
	case "pixel2":
		return [8]string{"...##...", "..####..", ".######.", "########", "##e##e##", "##e##e##", ".##..##.", "..####.."}, true
	case "pixel3":
		return [8]string{"..###...", ".#####..", "########", "##e##e##", "##e##e##", "########", ".######.", "..####.."}, true
	case "pixel4":
		return [8]string{"........", "..####..", ".######.", "########", "##e##e##", "##e##e##", ".######.", "..####.."}, true
	case "pixel5":
		return [8]string{"...##...", ".######.", "########", "##e##e##", "##e##e##", "########", "########", ".######."}, true
	case "pixel6":
		return [8]string{"........", "........", ".######.", "########", "##e##e##", "########", ".######.", "........"}, true
	case "pixel7":
		return [8]string{"...##...", "...##...", "..####..", "..####..", ".##e##..", ".######.", "##...##.", "........"}, true
	case "pixel8":
		return [8]string{".######.", "########", "##....##", "##e##e##", "##e##e##", "##....##", "########", ".######."}, true
	case "pixel9":
		return [8]string{"..##....", ".####...", "######..", "##e##e##", "##e##e##", ".######.", "..####..", "...##..."}, true
	case "pixel10":
		return [8]string{"..####..", ".######.", "########", "##e##e##", "##e##e##", "########", ".######.", "..####.."}, true
	case "pixel11":
		return [8]string{"...##...", "..####..", ".######.", "########", "##e##e##", ".######.", "..####..", "...##..."}, true
	case "pixel12":
		return [8]string{".##..##.", "########", "##e##e##", "##e##e##", "########", ".######.", "..####..", "...##..."}, true
	case "pixel13":
		return [8]string{"...##...", "..####..", ".######.", "###e####", "##e##e##", "########", ".######.", "..####.."}, true
	case "pixel14":
		return [8]string{"..####..", ".######.", "########", "##e##e##", "########", "...##...", "..####..", "..####.."}, true
	case "pixel15":
		return [8]string{"....#...", "..#####.", ".#######", "########", "##e##e##", ".######.", "..####..", "...##..."}, true
	case "pixel16":
		return [8]string{"........", "########", ".######.", "##e##e##", "##e##e##", ".######.", "..####..", "...##..."}, true
	default:
		return [8]string{}, false
	}
}

func pixelColor(pixel byte, ink string) string {
	switch pixel {
	case '#':
		return ink
	case 'e':
		return "#F3F7FA"
	default:
		return background
	}
}

func providerColor(provider, custom string) string {
	if len(custom) == 7 && custom[0] == '#' {
		return custom
	}
	switch strings.ToLower(provider) {
	case "claude":
		return "#B34CFF"
	case "codex":
		return "#2596FF"
	case "grok":
		return "#05D68B"
	case "gemini":
		return "#FF8A18"
	default:
		return "#23D6A1"
	}
}

func sprite(provider string) [8]string {
	switch strings.ToLower(provider) {
	case "claude": // round assistant with small antenna
		return [8]string{"...##...", "...##...", "..####..", ".######.", "##e##e##", "##e##e##", ".######.", "..####.."}
	case "codex": // angular hood
		return [8]string{"...##...", "..####..", ".######.", "########", "##e##e##", "##e##e##", ".##..##.", "..####.."}
	case "grok": // cloud-shaped helper
		return [8]string{"..###...", ".#####..", "########", "##e##e##", "##e##e##", "########", ".######.", "..####.."}
	case "gemini": // twin peaks
		return [8]string{"...##...", "..####..", ".##..##.", "##....##", "##e##e##", "##e##e##", ".######.", "..####.."}
	default: // rounded generic agent
		return [8]string{"...##...", "..####..", ".######.", "##e##e##", "##e##e##", "########", ".######.", "..####.."}
	}
}
