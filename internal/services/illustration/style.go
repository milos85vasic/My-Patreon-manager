package illustration

type StyleLoader struct {
	globalStyle string
}

func NewStyleLoader(globalStyle string) *StyleLoader {
	return &StyleLoader{globalStyle: globalStyle}
}

func (sl *StyleLoader) LoadStyle(repoOverride *string) string {
	if repoOverride != nil && *repoOverride != "" {
		return *repoOverride
	}
	return sl.globalStyle
}
