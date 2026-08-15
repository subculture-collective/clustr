package presentation

import (
	"fmt"
	"math"
)

// MacroPalette is a fixed color-deficiency-conscious set of categorical base
// roles. Shade, brightness, and redundant labels carry the remaining meaning.
func MacroPalette() []string {
	return []string{
		"#68DCFF", "#B6FF62", "#FF7096", "#FFC75F", "#B39DFF", "#5EE0B8",
		"#FF936B", "#8FB8FF", "#E4E86A", "#E18BEA", "#70D48B", "#D9A96E",
	}
}

func IsBridge(primary, secondary float64) bool {
	return secondary >= .30 && primary > 0 && secondary >= primary*.55
}

// ClusterShade keeps the macro hue categorical while encoding local spectral
// position as a bounded shade and confidence as bounded brightness. It never
// rotates hue, so it cannot imply a global ideological ordering.
func ClusterShade(base string, spectralPosition, confidence float64) string {
	var red, green, blue int
	if _, err := fmt.Sscanf(base, "#%02X%02X%02X", &red, &green, &blue); err != nil {
		return base
	}
	spectralPosition = math.Max(-1, math.Min(1, spectralPosition))
	confidence = math.Max(0, math.Min(1, confidence))
	channels := []float64{float64(red), float64(green), float64(blue)}
	// Low confidence is dimmer but remains readable; spectral position adds at
	// most 16% black/white so neighboring shades remain recognizably grouped.
	brightness := .72 + .28*confidence
	for index, channel := range channels {
		channel *= brightness
		if spectralPosition >= 0 {
			channel += (255 - channel) * spectralPosition * .16
		} else {
			channel *= 1 + spectralPosition*.16
		}
		channels[index] = math.Max(0, math.Min(255, channel))
	}
	return fmt.Sprintf("#%02X%02X%02X", int(math.Round(channels[0])), int(math.Round(channels[1])), int(math.Round(channels[2])))
}
