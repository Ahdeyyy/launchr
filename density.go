package main

// Missing or unknown preferences retain the original comfortable layout.
func normalizedDensity(value string) string {
	switch value {
	case "compact", "spacious":
		return value
	default:
		return "comfortable"
	}
}

type densityMetrics struct {
	headerY, searchY, tabsY, inset, rowY, rowX, gap, textGap, icon, searchFont, spacing float32
}

// Preview the draft throughout Settings without changing saved preferences.
func (a *app) density() densityMetrics {
	if a.settingsOpen {
		return launcherDensity(a.densityEdit)
	}
	return launcherDensity(a.prefs.Density)
}

func (d densityMetrics) scale(size float32) float32 {
	return size * d.spacing / 4
}

func launcherDensity(value string) densityMetrics {
	switch normalizedDensity(value) {
	case "compact":
		return densityMetrics{8, 10, 6, 14, 6, 10, 8, 2, 28, 18, 3}
	case "spacious":
		return densityMetrics{16, 20, 14, 22, 16, 16, 16, 6, 36, 22, 5}
	default:
		return densityMetrics{12, 14, 10, 18, 10, 12, 12, 4, 32, 20, 4}
	}
}
