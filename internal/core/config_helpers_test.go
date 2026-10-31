package core

// loadConfig is a strict view of LoadEffectiveConfig: any settings-file
// problem is an error.
func loadConfig() (Config, error) {
	c, warning, err := LoadEffectiveConfig()
	if warning != nil {
		return c, warning
	}
	return c, err
}
