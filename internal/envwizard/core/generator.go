package core

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

type Generator struct {
	Wizard *Wizard
}

func NewGenerator(w *Wizard) *Generator {
	return &Generator{Wizard: w}
}

func (g *Generator) ProduceEnvFile(maskSecrets bool) string {
	var builder strings.Builder

	categories := make(map[string][]*EnvVar)
	for _, v := range g.Wizard.Vars {
		catID := "uncategorized"
		if v.Category != nil {
			catID = v.Category.ID
		}
		categories[catID] = append(categories[catID], v)
	}

	sortedCats := make([]string, 0, len(categories))
	for id := range categories {
		sortedCats = append(sortedCats, id)
	}
	sort.Slice(sortedCats, func(i, j int) bool {
		ci := categoryOrder(categories[sortedCats[i]][0])
		cj := categoryOrder(categories[sortedCats[j]][0])
		return ci < cj
	})

	for _, catID := range sortedCats {
		vars := categories[catID]
		if len(vars) == 0 {
			continue
		}

		catName := catID
		if vars[0].Category != nil {
			catName = vars[0].Category.Name
		}
		fmt.Fprintf(&builder, "# %s\n", catName)

		for _, v := range vars {
			val, isSet := g.Wizard.Values[v.Name]
			if !isSet && g.Wizard.IsSkipped(v.Name) {
				continue
			}

			if !isSet {
				if v.Default != "" {
					val = v.Default
				} else {
					val = ""
				}
			}

			if maskSecrets && v.Secret && val != "" {
				val = strings.Repeat("*", len(val))
			}

			if v.Description != "" {
				fmt.Fprintf(&builder, "# %s\n", v.Description)
			}
			fmt.Fprintf(&builder, "%s=%s\n", v.Name, val)
		}
		builder.WriteString("\n")
	}

	return builder.String()
}

func (g *Generator) SaveToPath(path string, maskSecrets bool) error {
	content := g.ProduceEnvFile(maskSecrets)
	return os.WriteFile(path, []byte(content), 0600)
}

func categoryOrder(v *EnvVar) int {
	if v.Category != nil {
		return v.Category.Order
	}
	return 999
}
