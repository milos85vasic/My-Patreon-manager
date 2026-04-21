package core

type ValidationType string

const (
	ValidationRequired ValidationType = "required"
	ValidationPort     ValidationType = "port"
	ValidationURL      ValidationType = "url"
	ValidationToken    ValidationType = "token"
	ValidationBoolean  ValidationType = "boolean"
	ValidationNumber   ValidationType = "number"
	ValidationCron     ValidationType = "cron"
	ValidationCustom   ValidationType = "custom"
)

type Category struct {
	ID          string
	Name        string
	Description string
	Order       int
}

type EnvVar struct {
	Name            string
	Description     string
	Category        *Category
	Required        bool
	Default         string
	Validation      ValidationType
	ValidationRule  string
	URL             string
	CanGenerate     bool
	Secret          bool
	Example         string
}
